package internal

import (
	"go/ast"
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
	return handle.Of(ref.X).With(promoted), true
}

// scopeVisit is called for each expression of a scope that has a value
// inside the transaction. It reports whether to read inside the expression.
type scopeVisit func(e ast.Expr, ref *ssa.DebugRef, stack []ast.Node) bool

// walkScope calls visit for each expression in s that is evaluated inside
// the transaction. A function literal in s is read as part of it. In a
// region, a deferred statement is not read: it runs when the function
// returns. A variable on the left of an assignment, or in its declaration,
// is written, not used.
func (c *checker) walkScope(s txScope, visit scopeVisit) {
	ast.PreorderStack(s.root, nil, func(n ast.Node, stack []ast.Node) bool {
		if _, ok := n.(*ast.DeferStmt); ok && s.region != nil {
			return false
		}
		e, ok := n.(ast.Expr)
		if !ok {
			return true
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
		ref, ok := c.prog.Ref(fn, e)
		if !ok || ref.IsAddr {
			return true
		}
		if s.region != nil && !s.region.Inside(scopeAnchor(s.fn, ref)) {
			return false
		}
		return visit(e, ref, stack)
	})
}

// scopeWritten reports whether e is a variable written where it stands: on
// the left of an assignment, or declared by one.
func scopeWritten(e ast.Expr, stack []ast.Node) bool {
	id, ok := e.(*ast.Ident)
	if !ok {
		return false
	}
	switch parent := stack[len(stack)-1].(type) {
	case *ast.AssignStmt:
		return slices.Contains(parent.Lhs, ast.Expr(id))
	case *ast.ValueSpec:
		return slices.Contains(parent.Names, id)
	}
	return false
}

// scopeAnchor is the instruction of fn where ref is evaluated. A reference
// in a function literal is evaluated where the literal is made.
func scopeAnchor(fn *ssa.Function, ref *ssa.DebugRef) ssa.Instruction {
	var anchor ssa.Instruction = ref
	for f := ref.Parent(); f != fn; f = f.Parent() {
		anchor = program.Creation(f)
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
	c.walkScope(s, func(e ast.Expr, ref *ssa.DebugRef, stack []ast.Node) bool {
		if scopeChainsOn(e, exempt) {
			return !exempt[e]
		}
		if !handle.Of(ref.X).Equal(outer) {
			return true
		}
		if !s.builds || scopeRuns(info, e, stack) {
			c.report(e.Pos(), r, spell.Of(info, e, c.pass.Pkg)+" bypasses the transaction opened with "+s.opener+"; use "+use+" instead")
		}
		return false
	})
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

// scopeTakesContext reports whether call passes a context.Context.
func scopeTakesContext(info *types.Info, call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if t, ok := info.TypeOf(arg).(*types.Named); ok && t.Obj().Pkg() != nil && t.Obj().Pkg().Path() == "context" && t.Obj().Name() == "Context" {
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
	})
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
	arg, ok := c.prog.Ref(fn, call.Args[0])
	if !ok || !slices.ContainsFunc(s.txs, handle.Of(arg.X).Equal) {
		return scopeQueries{}, false
	}
	recv, ok := c.prog.Ref(fn, sel.X)
	info := c.pass.TypesInfo
	if !ok || !types.Identical(info.TypeOf(call), info.TypeOf(sel.X)) {
		return scopeQueries{}, false
	}
	return scopeQueries{outer: handle.Of(recv.X), use: types.ExprString(call)}, true
}
