package internal

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/region"
	"github.com/mpyw/txbypass/internal/rule"
	"github.com/mpyw/txbypass/internal/spell"
)

// txScope is the code that runs inside one transaction: the body of a
// callback, or the part of a function where a begun transaction is open.
//
//declscope:shared // outer_handle.go and outer_context.go build one for each transaction
type txScope struct {
	// root is the syntax to read: the callback's body, or the body of the
	// function that holds the begin.
	root ast.Node
	// fn is root's function.
	fn *ssa.Function
	// region is where a begun transaction is open in fn. It is nil for a
	// callback, which is inside throughout.
	region *region.Region
	// opener names the call that opened the transaction, for messages.
	opener string
	// tx is the variable or parameter that holds the transaction, or the
	// context that carries it. It is nil when there is none to name.
	tx *ast.Ident
	// txs are the values of the transaction, which WithTx is called with.
	txs []handle.Path
	// unnamed is what a message suggests when tx is nil.
	unnamed string
	// builds reports whether the handle builds references for the
	// transaction, so that a use counts only when it runs. See
	// opener.Opener.Builds.
	builds bool
}

// scopeOuter is the handle o opens its transaction on, in fn. For a method,
// that is its receiver, through the embedded fields its promotion passes.
//
//declscope:shared // both rules find the handle or the context outside the scope through it
func (c *checker) scopeOuter(o opener.Opener, fn *ssa.Function) (handle.Path, bool) {
	expr := o.Outer
	var promoted []*types.Var
	if o.Receiver {
		sel := o.Outer.(*ast.SelectorExpr)
		expr = sel.X
		promoted = handle.Promoted(c.pass.TypesInfo.Selections[sel])
	}
	ref, ok := c.prog.Ref(fn, expr)
	if !ok {
		return handle.Path{}, false
	}
	return c.paths.Of(ref.X).With(promoted), true
}

// scopeVisit is called for each expression of a scope that has a value
// inside the transaction. It reports whether to read inside the expression.
type scopeVisit func(e ast.Expr, ref *ssa.DebugRef, stack []ast.Node) bool

// walkScope calls visit for each expression in s that is evaluated inside
// the transaction. A function literal in s is read as part of it. In a
// region, a deferred statement is not read: it runs when the function
// returns. What stands on the left of an assignment, or a variable its
// declaration names, is written, not used.
//
// A selector with no value of its own, as the method in s.Create(u), is
// given to promoted, when it is not nil, with the reference of what it
// selects from.
func (c *checker) walkScope(s txScope, visit, promoted scopeVisit) {
	var hooks map[*ast.FuncLit]bool
	if s.region == nil {
		hooks = c.scopeHooks(s.root)
	}
	ast.PreorderStack(s.root, nil, func(n ast.Node, stack []ast.Node) bool {
		switch n := n.(type) {
		case *ast.DeferStmt:
			// The function that holds the begin runs its deferred calls when
			// it returns. A literal runs its own when it returns, inside.
			if s.region != nil && !slices.ContainsFunc(stack, scopeIsLiteral) {
				return false
			}
		case *ast.FuncLit:
			if hooks[n] {
				return false
			}
		}
		e, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		// A literal that is never made never runs.
		if lit, ok := e.(*ast.FuncLit); ok {
			if _, made := program.Creation(c.prog.Func(lit)); !made {
				return false
			}
		}
		if scopeWritten(e, stack) {
			return false
		}
		fn := s.fn
		for _, a := range slices.Backward(stack) {
			if lit, ok := a.(*ast.FuncLit); ok {
				fn = c.prog.Func(lit)
				break
			}
		}
		// A handle held by value is used through its address. The address
		// is the same handle as the value it holds.
		if ref, ok := c.prog.Ref(fn, e); ok {
			return s.scopeInside(ref) && visit(e, ref, stack)
		}
		sel, ok := e.(*ast.SelectorExpr)
		if !ok || promoted == nil {
			return true
		}
		x, ok := c.prog.Ref(fn, sel.X)
		return !ok || !s.scopeInside(x) || promoted(sel, x, stack)
	})
}

// scopeIsLiteral reports whether n is a function literal.
func scopeIsLiteral(n ast.Node) bool {
	_, ok := n.(*ast.FuncLit)
	return ok
}

// scopeHooks finds the function literals that the callback root keeps for
// after it returns, as an after-commit hook is. A literal is kept when it
// is stored in a variable declared outside the callback, or in a field or
// an element of one: assigned, appended, as a field of a composite literal,
// converted to a named function type, or named in a local of the callback
// that nothing else uses. Such a literal runs once the callback has
// returned, outside the transaction, on purpose.
func (c *checker) scopeHooks(root ast.Node) map[*ast.FuncLit]bool {
	info := c.pass.TypesInfo
	locals := make(map[types.Object]*ast.FuncLit)
	uses := make(map[types.Object]int)
	var assigns []*ast.AssignStmt
	ast.Inspect(root, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			assigns = append(assigns, n)
			if lit, ok := n.Rhs[0].(*ast.FuncLit); ok && n.Tok == token.DEFINE && len(n.Lhs) == 1 {
				locals[info.Defs[n.Lhs[0].(*ast.Ident)]] = lit
			}
		case *ast.Ident:
			uses[info.Uses[n]]++
		}
		return true
	})
	hooks := make(map[*ast.FuncLit]bool)
	for _, assign := range assigns {
		for i, lhs := range assign.Lhs {
			obj := info.ObjectOf(scopeBase(lhs))
			if obj == nil || root.Pos() <= obj.Pos() && obj.Pos() < root.End() || i >= len(assign.Rhs) {
				continue
			}
			c.scopeHookLiterals(assign.Rhs[i], locals, uses, hooks)
		}
	}
	return hooks
}

// scopeHookLiterals adds to hooks the literals that e stores: e itself, the
// fields of a composite literal, the values append adds, the operand of a
// conversion or of &, and the literal of a local used only here. A literal
// given to any other call may run inside the callback, so it is not one.
func (c *checker) scopeHookLiterals(e ast.Expr, locals map[types.Object]*ast.FuncLit, uses map[types.Object]int, hooks map[*ast.FuncLit]bool) {
	info := c.pass.TypesInfo
	switch e := ast.Unparen(e).(type) {
	case *ast.FuncLit:
		hooks[e] = true
	case *ast.Ident:
		if lit, ok := locals[info.Uses[e]]; ok && uses[info.Uses[e]] == 1 {
			hooks[lit] = true
		}
	case *ast.UnaryExpr:
		c.scopeHookLiterals(e.X, locals, uses, hooks)
	case *ast.KeyValueExpr:
		c.scopeHookLiterals(e.Value, locals, uses, hooks)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			c.scopeHookLiterals(elt, locals, uses, hooks)
		}
	case *ast.CallExpr:
		if info.Types[e.Fun].IsType() {
			c.scopeHookLiterals(e.Args[0], locals, uses, hooks)
		}
		if b, ok := info.Uses[scopeBase(e.Fun)].(*types.Builtin); ok && b.Name() == "append" {
			for _, arg := range e.Args[1:] {
				c.scopeHookLiterals(arg, locals, uses, hooks)
			}
		}
	}
}

// scopeBase is the identifier an expression selects or indexes from, as h
// in h.hooks[0]. It is nil for anything else.
func scopeBase(e ast.Expr) *ast.Ident {
	for {
		switch x := ast.Unparen(e).(type) {
		case *ast.Ident:
			return x
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		default:
			return nil
		}
	}
}

// scopeWrites reports whether code that runs inside the transaction of s
// stores to the handle outer, or to a variable or a field that holds it.
// In a region, only the stores inside the region count, and the stores of
// the function literals it makes there.
func (c *checker) scopeWrites(s txScope, outer handle.Path) bool {
	writes := func(fn *ssa.Function) bool {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				st, ok := instr.(*ssa.Store)
				if ok && outer.Stored(c.paths.Address(st.Addr)) && (fn != s.fn || s.region == nil || s.region.Inside(st)) {
					return true
				}
			}
		}
		return false
	}
	var within func(fn *ssa.Function) bool
	within = func(fn *ssa.Function) bool {
		if writes(fn) {
			return true
		}
		for _, anon := range fn.AnonFuncs {
			made, ok := program.Creation(anon)
			if ok && (fn != s.fn || s.region == nil || s.region.Inside(made) && !scopeDeferred(made)) && within(anon) {
				return true
			}
		}
		return false
	}
	return within(s.fn)
}

// scopeDeferred reports whether the literal that made makes runs only when
// the function returns: made is a defer of it, or a closure that only a
// defer uses.
func scopeDeferred(made ssa.Instruction) bool {
	if _, ok := made.(*ssa.Defer); ok {
		return true
	}
	mc, ok := made.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, ref := range *mc.Referrers() {
		switch ref.(type) {
		case *ssa.Defer, *ssa.DebugRef:
		default:
			return false
		}
	}
	return true
}

// scopeInside reports whether ref is evaluated inside the transaction of s.
func (s txScope) scopeInside(ref *ssa.DebugRef) bool {
	return s.region == nil || s.region.Inside(scopeAnchor(s.fn, ref))
}

// scopeWritten reports whether e is written where it stands: on the left
// of an assignment, or a variable its declaration names. An index on the
// left is not: m[k] = v reads m and k, and only the element is written.
func scopeWritten(e ast.Expr, stack []ast.Node) bool {
	if _, ok := e.(*ast.IndexExpr); ok {
		return false
	}
	switch parent := stack[len(stack)-1].(type) {
	case *ast.AssignStmt:
		return slices.Contains(parent.Lhs, e)
	case *ast.ValueSpec:
		id, ok := e.(*ast.Ident)
		return ok && slices.Contains(parent.Names, id)
	}
	return false
}

// scopeAnchor is the instruction of fn where ref is evaluated. A reference
// in a function literal is evaluated where the literal is made. walkScope
// reads no literal that is never made, so each one on the way is made.
func scopeAnchor(fn *ssa.Function, ref *ssa.DebugRef) ssa.Instruction {
	var anchor ssa.Instruction = ref
	for f := ref.Parent(); f != fn; f = f.Parent() {
		anchor, _ = program.Creation(f)
	}
	return anchor
}

// reportScopeUses reports, as rule r, each expression in s that is the
// handle outer. use is what the message tells the reader to use instead. An
// expression in exempt is not read, and neither is a chain built on one, as
// in q.WithTx(tx).CreateUser(ctx).
//
//declscope:shared // the rules report what their scopes find through it
func (c *checker) reportScopeUses(s txScope, outer handle.Path, r rule.Name, use string, exempt map[ast.Node]bool) {
	info := c.pass.TypesInfo
	// A scope that writes the handle, or what holds it, may run on another
	// value after the write: s.db = tx makes s.db the transaction. Which
	// handle it holds is then not known.
	if c.scopeWrites(s, outer) {
		return
	}
	visit := func(e ast.Expr, ref *ssa.DebugRef, stack []ast.Node) bool {
		if scopeChainsOn(e, exempt) {
			return !exempt[e]
		}
		if !c.paths.Of(ref.X).Within(outer) {
			return true
		}
		c.reportScopeUse(s, e, stack, outer, r, use)
		return false
	}
	// A method promoted from an embedded handle selects it with no
	// expression of its own: s.Create(u) passes s.DB in the SSA form, but
	// its debug reference is on s.
	promoted := func(e ast.Expr, x *ssa.DebugRef, stack []ast.Node) bool {
		path := handle.Promoted(info.Selections[e.(*ast.SelectorExpr)])
		if scopeChainsOn(e, exempt) || !c.paths.Of(x.X).With(path).Within(outer) {
			return true
		}
		c.reportScopeUse(s, e, stack, outer, r, use)
		return false
	}
	c.walkScope(s, visit, promoted)
}

// reportScopeUse reports the use e of the handle outer, unless it builds a
// reference that runs nothing.
func (c *checker) reportScopeUse(s txScope, e ast.Expr, stack []ast.Node, outer handle.Path, r rule.Name, use string) {
	info := c.pass.TypesInfo
	if s.builds && !scopeRuns(info, e, stack) {
		return
	}
	c.report(e.Pos(), r, spell.Of(info, e, c.pass.Pkg, outer.Depth())+" bypasses the transaction opened with "+s.opener+"; use "+use+" instead")
}

// scopeChainsOn reports whether e is an expression in exempt, or a chain of
// selectors and calls built on one.
func scopeChainsOn(e ast.Expr, exempt map[ast.Node]bool) bool {
	for {
		if exempt[e] {
			return true
		}
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.SelectorExpr:
			e = x.X
		case *ast.CallExpr:
			e = x.Fun
		default:
			return false
		}
	}
}

// scopeRuns reports whether the handle e is runs something where it is
// used, for a handle that builds references. stack holds e's ancestors. A
// chain of method calls on e runs when one of its calls takes a context, as
// client.Doc(p).Set(ctx, v) does. A chain of builders alone, as in
// client.Doc(p), only makes a reference. The handle on its own, given to a
// function, runs whatever the function does with it.
func scopeRuns(info *types.Info, e ast.Expr, stack []ast.Node) bool {
	chained := false
walk:
	for _, n := range slices.Backward(stack) {
		switch parent := n.(type) {
		case *ast.SelectorExpr:
			// e is what the selector selects from: a handle is never the
			// name after the dot.
			e = parent
		case *ast.CallExpr:
			if parent.Fun != e {
				break walk
			}
			if scopeTakesContext(info, parent) {
				return true
			}
			chained = true
			e = parent
		default:
			break walk
		}
	}
	return !chained
}

// scopeTakesContext reports whether call takes a context.Context: whether
// a parameter of the function it calls has that type. What the caller
// passes there, as a framework's request context, does not change what the
// call does.
//
// call is a call in a chain of methods on the handle, so what it calls is a
// method, and has a signature.
func scopeTakesContext(info *types.Info, call *ast.CallExpr) bool {
	sig := info.TypeOf(call.Fun).(*types.Signature)
	for p := range sig.Params().Variables() {
		if t, ok := types.Unalias(p.Type()).(*types.Named); ok && t.Obj().Pkg() != nil && t.Obj().Pkg().Path() == "context" && t.Obj().Name() == "Context" {
			return true
		}
	}
	return false
}

// scopeUse is what a message in s suggests in place of the outer handle.
//
//declscope:shared // the rules phrase their messages with it
func (s txScope) scopeUse() string {
	if s.tx == nil {
		return s.unnamed
	}
	return s.tx.Name
}

// scopeQueries is a handle that a WithTx call in a scope rebinds to the
// transaction, as sqlc's q.WithTx(tx) does.
type scopeQueries struct {
	// outer is the handle WithTx is called on.
	outer handle.Path
	// use is what a message suggests: the variable the result is assigned
	// to, or the call itself.
	use string
}

// reportScopeQueries reports the uses in s of each handle that s rebinds to
// its transaction with WithTx. Code that calls q.WithTx(tx) means to run on
// the transaction, so every other use of q there bypasses it. The method
// must return the type of its receiver, as sqlc's generated one does.
//
//declscope:shared // outer_handle.go reads both kinds of scope through it
func (c *checker) reportScopeQueries(s txScope) {
	var found []scopeQueries
	exempt := make(map[ast.Node]bool)
	c.walkScope(s, func(e ast.Expr, ref *ssa.DebugRef, stack []ast.Node) bool {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return true
		}
		q, ok := c.scopeWithTx(s, call, ref.Parent())
		if !ok {
			return true
		}
		if assign, ok := stack[len(stack)-1].(*ast.AssignStmt); ok && len(assign.Lhs) == 1 {
			if id, ok := assign.Lhs[0].(*ast.Ident); ok && id.Name != "_" {
				q.use = id.Name
			}
		}
		found = append(found, q)
		exempt[call] = true
		return true
	}, nil)
	for _, q := range found {
		c.reportScopeUses(s, q.outer, rule.OuterHandle, q.use, exempt)
	}
}

// scopeWithTx reports the handle call rebinds, when call is a call of WithTx
// with the transaction of s, on a handle, returning the handle's own type.
// fn is the function call is in.
func (c *checker) scopeWithTx(s txScope, call *ast.CallExpr, fn *ssa.Function) (scopeQueries, bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WithTx" || len(call.Args) != 1 {
		return scopeQueries{}, false
	}
	// The argument may be the transaction or a field of it: sqlc's WithTx
	// takes a *sql.Tx, which sqlx's and bun's transactions embed, as tx.Tx.
	arg, ok := c.prog.Ref(fn, call.Args[0])
	if !ok || !slices.ContainsFunc(s.txs, c.paths.Of(arg.X).Within) {
		return scopeQueries{}, false
	}
	// The method's own receiver decides, not what it is called on: WithTx
	// may be promoted from an embedded *Queries, as in sqlc's Store.
	info := c.pass.TypesInfo
	selection := info.Selections[sel]
	if selection == nil || selection.Kind() != types.MethodVal {
		// A function named WithTx, as pkg.WithTx(tx), rebinds nothing.
		return scopeQueries{}, false
	}
	recvType := selection.Obj().(*types.Func).Signature().Recv().Type()
	recv, ok := c.prog.Ref(fn, sel.X)
	if !ok || !types.Identical(info.TypeOf(call), recvType) || !scopeHoldsOnly(recvType, info.TypeOf(call.Args[0])) {
		return scopeQueries{}, false
	}
	return scopeQueries{outer: c.paths.Of(recv.X).With(handle.Promoted(selection)), use: types.ExprString(call)}, true
}

// scopeHoldsOnly reports whether every field of the struct that t is, or
// points to, can hold a transaction of type tx, as sqlc's Queries holds only
// its DBTX. A struct that holds more, as a logger or metrics, is not only a
// database: a use of its other members is no use of the transaction.
func scopeHoldsOnly(t, tx types.Type) bool {
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for f := range st.Fields() {
		if !types.AssignableTo(tx, f.Type()) {
			return false
		}
	}
	return true
}
