package internal

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/region"
	"github.com/mpyw/txbypass/internal/rule"
	"github.com/mpyw/txbypass/internal/spell"
	"github.com/mpyw/txbypass/internal/txcall"
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
	// openedWith names the begin or the invoker that opened the transaction,
	// for messages.
	openedWith string
	// tx is the variable or parameter that holds the transaction, or the
	// context that carries it. It is nil when there is none to name.
	tx *ast.Ident
	// txs are the values of the transaction, which WithTx is called with.
	txs []handle.Path
	// unnamed is what a message suggests when tx is nil.
	unnamed string
	// builds reports whether the handle builds references for the
	// transaction, so that a use counts only when it runs. See
	// txcall.Invoker.Builds.
	builds bool
}

// scopeOuter is the handle o opens its transaction on, in fn. For a method,
// that is its receiver, through the embedded fields its promotion passes.
//
//declscope:shared // both rules find the handle or the context outside the scope through it
func (c *checker) scopeOuter(o txcall.Invoker, fn *ssa.Function) (handle.Path, bool) {
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

// scopeBindsBoth reports whether e is an argument of a call, or a field of
// a literal, that takes the transaction too. A constructor newStore(s.db,
// tx), or a literal &Store{db: s.db, tx: tx}, keeps the handle beside the
// transaction, which is no use of it.
func (c *checker) scopeBindsBoth(s txScope, fn *ssa.Function, e ast.Expr, stack []ast.Node) bool {
	var beside []ast.Expr
	switch parent := stack[len(stack)-1].(type) {
	case *ast.CallExpr:
		if slices.Contains(parent.Args, e) {
			beside = parent.Args
		}
	case *ast.KeyValueExpr:
		// A field of a literal, as &Store{db: s.db, tx: tx}.
		if lit, ok := stack[len(stack)-2].(*ast.CompositeLit); ok && parent.Value == e {
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					beside = append(beside, kv.Value)
				}
			}
		}
	case *ast.CompositeLit:
		beside = parent.Elts
	}
	return slices.ContainsFunc(beside, func(arg ast.Expr) bool {
		ref, ok := c.prog.Ref(fn, arg)
		return ok && slices.ContainsFunc(s.txs, c.paths.Of(ref.X).Within)
	})
}

// scopeChainBound reports whether a chain of calls on e, or on a call given
// e, is given the transaction, as bun's db.NewInsert().Conn(tx) and sqlc's
// New(pool).WithTx(tx) are. The query is built on the handle, and runs on
// the transaction.
func (c *checker) scopeChainBound(s txScope, fn *ssa.Function, e ast.Expr, stack []ast.Node) bool {
	// The scope's root is a block, which ends the walk up. The chain may
	// start at a call given the handle, as sqlc's New(pool).WithTx(tx).
	cur, i := e, len(stack)-1
	// The handle may be a field of a literal kept and then given the
	// transaction, as m := &Manager{DB: db} and m.CreateWithTx(tx, ...).
	if lit, ok := scopeLiteralOf(e, stack); ok {
		if ref, ok := c.prog.Ref(fn, lit); ok && c.scopeValueBound(s, ref.X) {
			return true
		}
	}
	if call, ok := stack[i].(*ast.CallExpr); ok && slices.Contains(call.Args, e) {
		// The call's result may be kept, as q := New(pool), and rebound
		// later, as q.WithTx(tx).
		if ref, ok := c.prog.Ref(fn, call); ok && c.scopeValueBound(s, ref.X) {
			return true
		}
		cur, i = call, i-1
	}
	for ; ; i-- {
		switch n := stack[i].(type) {
		case *ast.SelectorExpr:
			// cur is what n selects from: a handle is never the name after
			// the dot.
		case *ast.CallExpr:
			if n.Fun != cur {
				return false
			}
			if slices.ContainsFunc(n.Args, func(arg ast.Expr) bool {
				ref, ok := c.prog.Ref(fn, arg)
				return ok && slices.ContainsFunc(s.txs, c.paths.Of(ref.X).Within)
			}) {
				return true
			}
		default:
			return false
		}
		cur = stack[i].(ast.Expr)
	}
}

// scopeContextMethods are the methods of context.Context.
var scopeContextMethods = map[string]bool{"Deadline": true, "Done": true, "Err": true, "Value": true}

// scopeAsContext reports whether sel, selected from the outer context, uses
// it as a context: one of context.Context's own methods. A context of a web
// framework, as gin's *gin.Context, has methods and fields of its own, as
// c.JSON(...) or c.Request. They run no statement, and the inner context
// has none of them, so using it instead does not compile.
func scopeAsContext(info *types.Info, sel *ast.SelectorExpr) bool {
	selection := info.Selections[sel]
	return selection != nil && selection.Kind() == types.MethodVal && scopeContextMethods[sel.Sel.Name]
}

// scopeContextUse reports whether e, which holds the outer context, uses it
// as a context. A field of it, as c.Request, which field reports, or a
// method that the selector above e selects and context.Context lacks, as
// c.JSON, is no such use.
func scopeContextUse(info *types.Info, e ast.Expr, stack []ast.Node, field bool) bool {
	if field {
		return false
	}
	if call, ok := stack[len(stack)-1].(*ast.CallExpr); ok {
		return scopeContextArg(info, call, e)
	}
	if t, ok := scopeSlot(info, e, stack); ok {
		return scopeFitsContext(t)
	}
	sel, ok := stack[len(stack)-1].(*ast.SelectorExpr)
	return !ok || sel.X != e || scopeAsContext(info, sel)
}

// scopeSlot is the type of the place e is put in: a field or an element of
// a literal, keyed or not, a value sent on a channel, or a variable it is
// assigned to.
func scopeSlot(info *types.Info, e ast.Expr, stack []ast.Node) (types.Type, bool) {
	switch parent := stack[len(stack)-1].(type) {
	case *ast.KeyValueExpr:
		// A key of a map literal is no slot of a value.
		if parent.Value != e {
			return nil, false
		}
		if key, ok := parent.Key.(*ast.Ident); ok {
			if f, ok := info.Uses[key].(*types.Var); ok && f.IsField() {
				return f.Type(), true
			}
		}
		// The parent of a key and a value is a literal.
		return scopeElem(info.TypeOf(stack[len(stack)-2].(*ast.CompositeLit)), 0), true
	case *ast.CompositeLit:
		return scopeElem(info.TypeOf(parent), slices.Index(parent.Elts, e)), true
	case *ast.SendStmt:
		// A channel's operand is a channel, or a type parameter whose type
		// set holds channels of one element type.
		elem, ok := program.ChanElem(info.TypeOf(parent.Chan))
		return elem, ok && parent.Value == e
	case *ast.AssignStmt:
		// The blank identifier has no type: it takes e as itself, as does
		// an index on the left, as m[k] in m[k] = v.
		slot := e
		if i := slices.Index(parent.Rhs, e); i >= 0 {
			slot = parent.Lhs[i]
		}
		if t := info.TypeOf(slot); t != nil {
			return t, true
		}
		return info.TypeOf(e), true
	case *ast.ValueSpec:
		// What a declaration names is written, not read, so e is a value.
		return info.TypeOf(parent.Names[slices.Index(parent.Values, e)]), true
	}
	return nil, false
}

// scopeElem is the type of element i of a literal of type t: the field of
// a struct, or the element of a slice, an array or a map.
func scopeElem(t types.Type, i int) types.Type {
	// An element literal with its & left out, as {ctx} in []*Job{{ctx}},
	// has the pointer's type: it builds what the pointer points to.
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}
	// A literal of a type parameter, as S{ctx} with S ~[]context.Context,
	// builds its core type.
	switch core := program.Core(t).(type) {
	case *types.Struct:
		return core.Field(i).Type()
	case interface{ Elem() types.Type }:
		return core.Elem()
	}
	// Unreachable: a literal has a core type, which a type checker
	// requires of it.
	return t
}

// scopeFitsContext reports whether a value of type t can be a plain
// context.Context: an interface with no method but the context's, as
// context.Context itself or any. A framework's context interface, as fiber
// v3's fiber.Ctx, has more, so the inner context does not fit there.
func scopeFitsContext(t types.Type) bool {
	iface, ok := t.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	for m := range iface.Methods() {
		if !scopeContextMethods[m.Name()] {
			return false
		}
	}
	return true
}

// scopeSessionEnds are the methods of MongoDB's session that end it or its
// transaction. They take a context only for its deadline: the session they
// are called on says what they end.
var scopeSessionEnds = map[string]bool{"CommitTransaction": true, "AbortTransaction": true, "EndSession": true}

// scopeContextArg reports whether giving e to call uses it as a context.
// A parameter of a concrete type, as a helper that takes *gin.Context,
// takes it as that type: the inner context has none of its methods. The
// methods of MongoDB's session that end it are given a context only for
// its deadline.
func scopeContextArg(info *types.Info, call *ast.CallExpr, e ast.Expr) bool {
	if fn, ok := typeutil.Callee(info, call).(*types.Func); ok && fn.Pkg() != nil && strings.HasPrefix(fn.Pkg().Path(), "go.mongodb.org/mongo-driver") && scopeSessionEnds[fn.Name()] {
		return false
	}
	// A conversion has no signature, and gives e as a context. A generic
	// function is read in its origin, where a type parameter constrained by
	// context.Context is an interface.
	// A method expression, as (*Repo).Save(r, ctx), takes its receiver
	// as its first argument, which its type spells as a parameter.
	sig, ok := info.TypeOf(call.Fun).Underlying().(*types.Signature)
	if fn, isFunc := typeutil.Callee(info, call).(*types.Func); isFunc && !scopeMethodExpr(info, call.Fun) {
		sig = fn.Origin().Signature()
	}
	return !ok || scopeFitsContext(scopeParam(sig, slices.Index(call.Args, e)))
}

// scopeMethodExpr reports whether fun is a method expression, as
// (*Repo).Save.
func scopeMethodExpr(info *types.Info, fun ast.Expr) bool {
	// A generic method takes its type arguments after it, as
	// (*Repo).Get[string].
	fun = ast.Unparen(fun)
	switch x := fun.(type) {
	case *ast.IndexExpr:
		fun = x.X
	case *ast.IndexListExpr:
		fun = x.X
	}
	sel, ok := ast.Unparen(fun).(*ast.SelectorExpr)
	return ok && info.Selections[sel] != nil && info.Selections[sel].Kind() == types.MethodExpr
}

// scopeParam is the type of the parameter that argument i of a call of sig
// is given to. The variadic parameter takes every argument after it.
func scopeParam(sig *types.Signature, i int) types.Type {
	params := sig.Params()
	if sig.Variadic() && i >= params.Len()-1 {
		return params.At(params.Len() - 1).Type().(*types.Slice).Elem()
	}
	return params.At(i).Type()
}

// scopeYieldsContext reports whether e, a method selector on the outer
// context, is called for a context.Context, as gin's c.Request.Context()
// gives the outer context back. A method call is read at its selector, so
// the call is e's parent.
func scopeYieldsContext(info *types.Info, e ast.Expr, stack []ast.Node) bool {
	call, ok := stack[len(stack)-1].(*ast.CallExpr)
	return ok && call.Fun == e && scopeIsContext(info.TypeOf(call))
}

// scopeIsContext reports whether t is context.Context.
func scopeIsContext(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "context" && named.Obj().Name() == "Context"
}

// scopeGivenForLater reports whether e is given to a function of the
// package that uses it only in a function it gives to t.Cleanup, as a test
// helper that drops a table after the test. The use runs after the test,
// when the transaction has ended.
func scopeGivenForLater(fn *ssa.Function, e ast.Expr, stack []ast.Node) bool {
	call, ok := stack[len(stack)-1].(*ast.CallExpr)
	if !ok {
		return false
	}
	i := slices.Index(call.Args, e)
	ssaCall, ok := program.Call(fn, call)
	if i < 0 || !ok {
		return false
	}
	callee := ssaCall.Common().StaticCallee()
	if callee == nil {
		return false
	}
	// A generic helper is called through an instance, which is built from
	// its origin.
	if origin := callee.Origin(); origin != nil {
		callee = origin
	}
	// A method takes its receiver as parameter 0.
	if callee.Signature.Recv() != nil {
		i++
	}
	if callee.Blocks == nil || callee.Signature.Variadic() && i >= len(callee.Params)-1 {
		return false
	}
	return scopeOnlyLater(callee.Params[i])
}

// scopeOnlyLater reports whether every use of v is in a function given to
// t.Cleanup. A parameter that a literal captures is kept in a
// variable first.
func scopeOnlyLater(v ssa.Value) bool {
	used := false
	for _, ref := range *v.Referrers() {
		switch ref := ref.(type) {
		case *ssa.DebugRef:
		case *ssa.Store:
			if ref.Addr == v {
				// The variable that v is, written.
				continue
			}
			addr, ok := ref.Addr.(*ssa.Alloc)
			if !ok || !scopeOnlyLater(addr) {
				return false
			}
			used = true
		case *ssa.MakeClosure:
			// A defer in the helper runs when the helper returns, inside
			// the transaction. Only t.Cleanup runs after it.
			if !scopeCleanedUp(ref) {
				return false
			}
			used = true
		default:
			return false
		}
	}
	return used
}

// scopeCleanedUp reports whether mc is used only by t.Cleanup.
func scopeCleanedUp(mc *ssa.MakeClosure) bool {
	return !slices.ContainsFunc(*mc.Referrers(), func(ref ssa.Instruction) bool {
		if _, ok := ref.(*ssa.DebugRef); ok {
			return false
		}
		call, ok := ref.(*ssa.Call)
		return !ok || !program.Cleanup(call.Common())
	})
}

// scopeRunsBound reports whether e, a method of the handle, is called with
// an argument bound to the transaction, as Datastore's client.GetAll(ctx, q,
// &dst) is when q was built with q.Transaction(tx). The client runs the
// query in the transaction it carries.
func (c *checker) scopeRunsBound(s txScope, fn *ssa.Function, e ast.Expr, stack []ast.Node) bool {
	// A method of the handle with no value of its own is called, with type
	// arguments between, as s.Get[User](ctx, q), when it is generic.
	i := len(stack) - 1
	switch stack[i].(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		i--
	}
	call, ok := stack[i].(*ast.CallExpr)
	return ok && slices.ContainsFunc(call.Args, func(arg ast.Expr) bool {
		ref, ok := c.prog.Ref(fn, arg)
		return ok && c.scopeMadeBound(s, ref.X, make(scopeWalk))
	})
}

// scopeMadeBound reports whether v was built by a chain of methods one of
// which was given the transaction, as q.Ancestor(k).Transaction(tx). The
// method given it is a builder: it returns its receiver's type. A
// function given the transaction, as context.WithValue(ctx, key, tx),
// binds nothing it returns to it. A variable is bound when every value
// written to it is, and a phi when every edge is.
func (c *checker) scopeMadeBound(s txScope, v ssa.Value, w scopeWalk) bool {
	// A value still on the way comes back around a loop. It adds nothing to
	// the edges and stores it meets, which all must be bound. A value read
	// already gives the same answer.
	if bound, ok := w[v]; ok {
		return bound == nil || *bound
	}
	w[v] = nil
	bound := c.scopeMadeBoundRead(s, v, w)
	w[v] = &bound
	return bound
}

// scopeWalk holds, for each value scopeMadeBound met, its answer, or nil
// while it is still on the way.
type scopeWalk map[ssa.Value]*bool

// scopeMadeBoundRead is scopeMadeBound for a value met the first time.
func (c *checker) scopeMadeBoundRead(s txScope, v ssa.Value, w scopeWalk) bool {
	switch v := v.(type) {
	case *ssa.Call:
		common := v.Common()
		if common.Signature().Recv() == nil || len(common.Args) == 0 {
			return false
		}
		builder := types.Identical(v.Type(), common.Args[0].Type())
		if builder && slices.ContainsFunc(common.Args[1:], func(arg ssa.Value) bool {
			return slices.ContainsFunc(s.txs, c.paths.Of(arg).Within)
		}) {
			return true
		}
		// A method of a bound value gives a value bound too, as Datastore's
		// q.NewAggregationQuery() of a query bound with q.Transaction(tx).
		return c.scopeMadeBound(s, common.Args[0], w)
	case *ssa.UnOp:
		addr, ok := v.X.(*ssa.Alloc)
		if !ok {
			return false
		}
		var vals []ssa.Value
		for _, ref := range *addr.Referrers() {
			if st, ok := ref.(*ssa.Store); ok && st.Addr == addr {
				vals = append(vals, st.Val)
			}
		}
		return c.scopeAllBound(s, vals, w)
	case *ssa.Phi:
		return c.scopeAllBound(s, v.Edges, w)
	}
	return false
}

// scopeAllBound reports whether every value of vals is bound, as
// scopeMadeBound says, and one of them leads somewhere other than back.
func (c *checker) scopeAllBound(s txScope, vals []ssa.Value, w scopeWalk) bool {
	ahead := false
	for _, v := range vals {
		bound, met := w[v]
		back := met && bound == nil
		if !c.scopeMadeBound(s, v, w) {
			return false
		}
		ahead = ahead || !back
	}
	return ahead
}

// scopeLiteralOf is the literal that e is a field or an element of, with the
// & taken of it when there is one.
func scopeLiteralOf(e ast.Expr, stack []ast.Node) (ast.Expr, bool) {
	i := len(stack) - 1
	if kv, ok := stack[i].(*ast.KeyValueExpr); ok && kv.Value == e {
		i--
	}
	lit, ok := stack[i].(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	if addr, ok := stack[i-1].(*ast.UnaryExpr); ok && addr.Op == token.AND {
		return addr, true
	}
	return lit, true
}

// scopeValueBound reports whether every use of v, or of a variable it is
// kept in, is a call given the transaction, as q in q.WithTx(tx), and one
// is. Any other use runs on what v was built from: m.Exists(name) on
// m := &Manager{DB: db} uses db, whatever m.CreateWithTx(tx, name) does.
func (c *checker) scopeValueBound(s txScope, v ssa.Value) bool {
	bound, only := c.scopeOnlyBound(s, v, make(map[ssa.Value]bool))
	return bound && only
}

// scopeOnlyBound is scopeValueBound, with the variables already read. It
// reports whether a use of v is a call given the transaction, and whether
// every use is one, or builds v.
func (c *checker) scopeOnlyBound(s txScope, v ssa.Value, seen map[ssa.Value]bool) (bound, only bool) {
	for _, ref := range *v.Referrers() {
		b, o := false, false
		switch ref := ref.(type) {
		case *ssa.DebugRef:
			o = true
		case ssa.CallInstruction:
			args := ref.Common().Args
			b = len(args) > 0 && args[0] == v && slices.ContainsFunc(args[1:], func(arg ssa.Value) bool {
				return slices.ContainsFunc(s.txs, c.paths.Of(arg).Within)
			})
			o = b
		case *ssa.FieldAddr:
			// A field written as v is built, as &Manager{DB: db} does.
			o = !slices.ContainsFunc(*ref.Referrers(), func(r ssa.Instruction) bool {
				st, ok := r.(*ssa.Store)
				return !ok || st.Addr != ref
			})
		case *ssa.Store:
			if ref.Val == v {
				b, o = c.scopeVarBound(s, ref.Addr, seen)
			}
		}
		if !o {
			return false, false
		}
		bound = bound || b
	}
	return bound, true
}

// scopeVarBound is scopeOnlyBound for a variable that v is kept in, read
// through its loads, and through the free variable of a literal that
// captures it. A pointer copied back and forth between two variables leads
// back to one.
func (c *checker) scopeVarBound(s txScope, addr ssa.Value, seen map[ssa.Value]bool) (bound, only bool) {
	if seen[addr] {
		return false, true
	}
	seen[addr] = true
	for _, use := range *addr.Referrers() {
		b, o := false, false
		switch use := use.(type) {
		case *ssa.UnOp:
			b, o = c.scopeOnlyBound(s, use, seen)
		case *ssa.MakeClosure:
			fv := use.Fn.(*ssa.Function).FreeVars[slices.Index(use.Bindings, addr)]
			b, o = c.scopeVarBound(s, fv, seen)
		case *ssa.Store, *ssa.DebugRef:
			o = true
		}
		if !o {
			return false, false
		}
		bound = bound || b
	}
	return bound, true
}

// scopeMetadataMethods are the methods of a pool that read or set the pool
// itself, by package, type and name. A transaction has none of them, so
// "use the transaction" is no fix, and they run no statement.
var scopeMetadataMethods = map[[3]string]bool{
	{"database/sql", "DB", "Stats"}:                       true,
	{"database/sql", "DB", "Driver"}:                      true,
	{"database/sql", "DB", "SetMaxOpenConns"}:             true,
	{"database/sql", "DB", "SetMaxIdleConns"}:             true,
	{"database/sql", "DB", "SetConnMaxLifetime"}:          true,
	{"database/sql", "DB", "SetConnMaxIdleTime"}:          true,
	{"github.com/jackc/pgx/v5/pgxpool", "Pool", "Stat"}:   true,
	{"github.com/jackc/pgx/v5/pgxpool", "Pool", "Config"}: true,
	{"github.com/jackc/pgx/v4/pgxpool", "Pool", "Stat"}:   true,
	{"github.com/jackc/pgx/v4/pgxpool", "Pool", "Config"}: true,
	{"github.com/uptrace/bun", "DB", "DBStats"}:           true,
}

// scopeMetadata reports whether e selects one of scopeMetadataMethods. A
// method call is read at its selector, which has no value of its own, so
// the handle under it is not read again.
func scopeMetadata(info *types.Info, e ast.Expr) bool {
	selection := scopeMethod(info, e)
	if selection == nil {
		return false
	}
	fn := selection.Obj().(*types.Func)
	recv := types.Unalias(fn.Signature().Recv().Type())
	if ptr, ok := recv.(*types.Pointer); ok {
		recv = ptr.Elem()
	}
	// A method of the universe, as error's Error, belongs to no package.
	named, ok := types.Unalias(recv).(*types.Named)
	return ok && fn.Pkg() != nil && scopeMetadataMethods[[3]string{fn.Pkg().Path(), named.Obj().Name(), fn.Name()}]
}

// scopeInert reports whether e only reads a value of the handle that runs
// nothing: a field of a basic type or an error, as gorm's
// db.Create(x).Error or RowsAffected, or a variable that holds one. The
// statement that set it was read where it ran. Reading it again is no
// statement. A handle whose only use is such a read, as res in res.Error,
// is read there too.
func scopeInert(info *types.Info, e ast.Expr, stack []ast.Node) bool {
	if scopeInertType(info.TypeOf(e)) {
		return true
	}
	// A scope's root is a block, so an expression has a parent.
	sel, ok := stack[len(stack)-1].(*ast.SelectorExpr)
	return ok && sel.X == e && info.Selections[sel] != nil && info.Selections[sel].Kind() == types.FieldVal && scopeInertType(info.TypeOf(sel))
}

// scopeInertType reports whether t is a basic type or error.
func scopeInertType(t types.Type) bool {
	if _, ok := t.Underlying().(*types.Basic); ok {
		return true
	}
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

// scopeMethod is the selection of the method that e selects, or nil.
func scopeMethod(info *types.Info, e ast.Expr) *types.Selection {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || info.Selections[sel] == nil || info.Selections[sel].Kind() != types.MethodVal {
		return nil
	}
	return info.Selections[sel]
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
	// later holds what runs after the scope: the function and the
	// arguments of a deferred call, and the function given to t.Cleanup. A
	// call among them runs where it stands, so it is read.
	later := make(map[ast.Node]bool)
	ast.PreorderStack(s.root, nil, func(n ast.Node, stack []ast.Node) bool {
		if later[n] {
			return false
		}
		// The name after a dot is no expression of its own: the selector is
		// read whole.
		if sel, ok := scopeParent(stack).(*ast.SelectorExpr); ok && sel.Sel == n {
			return false
		}
		switch n := n.(type) {
		case *ast.DeferStmt:
			// The function that holds the begin runs its deferred calls when
			// it returns. A literal runs its own when it returns, inside.
			if s.region != nil && !slices.ContainsFunc(stack, scopeIsLiteral) {
				scopeLater(later, n.Call.Fun)
				for _, arg := range n.Call.Args {
					scopeLater(later, arg)
				}
				return true
			}
		case *ast.CallExpr:
			// t.Cleanup runs its function after the test returns, when the
			// transaction has ended.
			if program.CleanupCall(c.pass.TypesInfo, n) {
				for _, arg := range n.Args {
					scopeLater(later, arg)
				}
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
		fn := c.prog.Enclosing(s.fn, stack, e)
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

// scopeParent is the innermost node of stack, or nil.
func scopeParent(stack []ast.Node) ast.Node {
	if len(stack) == 0 {
		return nil
	}
	return stack[len(stack)-1]
}

// scopeLater adds e to later, unless it is a call, which runs where it
// stands.
func scopeLater(later map[ast.Node]bool, e ast.Expr) {
	if _, ok := ast.Unparen(e).(*ast.CallExpr); !ok {
		later[e] = true
	}
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
// rebinds the handle outer, as scopeRebinds says.
// In a region, only the stores inside the region count, and the stores of
// the function literals it makes there.
func (c *checker) scopeWrites(s txScope, outer handle.Path) bool {
	writes := func(fn *ssa.Function) bool {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				st, ok := instr.(*ssa.Store)
				if ok && c.scopeRebinds(s, outer, st) && (fn != s.fn || s.region == nil || s.region.Inside(st)) {
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

// scopeRebinds reports whether st stores to the handle outer, or to a
// variable or a field that holds it. A store into a field of outer of a
// value taken from the transaction counts too. gorm's own
// base.Statement.ConnPool = tx.Statement.ConnPool makes base run its
// statements on the transaction.
func (c *checker) scopeRebinds(s txScope, outer handle.Path, st *ssa.Store) bool {
	addr := c.paths.Address(st.Addr)
	if outer.Stored(addr) {
		return true
	}
	return addr.Within(outer) && slices.ContainsFunc(s.txs, c.paths.Of(st.Val).Within)
}

// scopeDeferred reports whether the literal that made makes runs only when
// the function returns: made is a defer of it or a t.Cleanup of it, or a
// closure that only those use.
func scopeDeferred(made ssa.Instruction) bool {
	if scopeDefers(made) {
		return true
	}
	mc, ok := made.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, ref := range *mc.Referrers() {
		if _, ok := ref.(*ssa.DebugRef); !ok && !scopeDefers(ref) {
			return false
		}
	}
	return true
}

// scopeDefers reports whether instr is a defer, or a t.Cleanup.
func scopeDefers(instr ssa.Instruction) bool {
	switch instr := instr.(type) {
	case *ssa.Defer:
		return true
	case *ssa.Call:
		return program.Cleanup(instr.Common())
	}
	return false
}

// scopeInside reports whether ref is evaluated inside the transaction of s.
func (s txScope) scopeInside(ref *ssa.DebugRef) bool {
	if s.region == nil {
		return true
	}
	// The body of a range over a function runs in the loop, where its own
	// ends are read. Any other literal runs where it is made.
	var anchor ssa.Instruction = ref
	for f := ref.Parent(); f != s.fn; f = f.Parent() {
		if program.Yield(f) && !s.region.InsideBody(f, anchor) {
			return false
		}
		anchor, _ = program.Creation(f)
	}
	return s.region.Inside(anchor)
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
		if !c.paths.Of(ref.X).Within(outer) || scopeInert(info, e, stack) {
			return true
		}
		// The call that makes the handle, as New(pool) for q, is no use of
		// it. What it is given is read.
		if _, ok := e.(*ast.CallExpr); ok && ref.X == outer.Root() {
			return true
		}
		if r == rule.OuterContext && !scopeContextUse(info, e, stack, c.paths.Of(ref.X).Depth() > outer.Depth()) {
			return false
		}
		if c.scopeBindsBoth(s, ref.Parent(), e, stack) || c.scopeChainBound(s, ref.Parent(), e, stack) || scopeGivenForLater(ref.Parent(), e, stack) {
			return false
		}
		if !scopeMetadata(info, e) {
			c.reportScopeUse(s, e, stack, outer, r, use)
		}
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
		sel := e.(*ast.SelectorExpr)
		if scopeInertType(info.TypeOf(sel.X)) {
			// A method of an error the handle gave, as err.Error().
			return true
		}
		if c.scopeChainBound(s, x.Parent(), e, stack) || c.scopeRunsBound(s, x.Parent(), e, stack) || r == rule.OuterContext && !scopeAsContext(info, sel) && !scopeYieldsContext(info, e, stack) {
			return false
		}
		if !scopeMetadata(info, e) {
			c.reportScopeUse(s, e, stack, outer, r, use)
		}
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
	c.report(e.Pos(), r, spell.Of(info, e, c.pass.Pkg, outer.Depth())+" bypasses the transaction opened with "+s.openedWith+"; use "+use+" instead")
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
// its DBTX, or is a statement prepared on one. With emit_prepared_queries,
// sqlc's Queries holds a *sql.Stmt per query, and runs it on the
// transaction after WithTx. A struct that holds more, as a logger or
// metrics, is not only a database: a use of its other members is no use of
// the transaction.
func scopeHoldsOnly(t, tx types.Type) bool {
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for f := range st.Fields() {
		if !types.AssignableTo(tx, f.Type()) && !scopePrepared(f.Type()) {
			return false
		}
	}
	return true
}

// scopePrepared reports whether t is database/sql's prepared statement,
// *sql.Stmt.
func scopePrepared(t types.Type) bool {
	ptr, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return false
	}
	n, ok := types.Unalias(ptr.Elem()).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "database/sql" && n.Obj().Name() == "Stmt"
}

// scopeGiven reports whether p is the parameter of a callback that an
// invoker runs, which takes reports of the invoker and p's index.
//
//declscope:shared // both rules ask whether a value is what an invoker gives its callback
func scopeGiven(p *ssa.Parameter, takes func(*types.Func, int) bool) bool {
	lit := p.Parent()
	if lit.Parent() == nil {
		// A declared function is no callback this analysis reads.
		return false
	}
	// A literal never made has no creation, and its body is not read.
	made, _ := program.Creation(lit)
	i := slices.Index(lit.Params, p)
	var calls []ssa.CallInstruction
	switch made := made.(type) {
	case ssa.CallInstruction:
		calls = append(calls, made)
	case *ssa.MakeClosure:
		for _, ref := range *made.Referrers() {
			if call, ok := ref.(ssa.CallInstruction); ok {
				calls = append(calls, call)
			}
		}
	}
	return slices.ContainsFunc(calls, func(call ssa.CallInstruction) bool {
		fn, ok := scopeCallee(call.Common())
		return ok && takes(fn, i)
	})
}

// scopeCallee is the function, or the interface method, that call calls.
// go-rel's Repository and go-transaction-manager's trm.Manager are
// interfaces.
//
//declscope:shared // outer_handle.go asks what a begin calls through it
func scopeCallee(call *ssa.CallCommon) (*types.Func, bool) {
	if call.IsInvoke() {
		return call.Method, true
	}
	if fn := call.StaticCallee(); fn != nil {
		obj, ok := fn.Object().(*types.Func)
		return obj, ok
	}
	return nil, false
}
