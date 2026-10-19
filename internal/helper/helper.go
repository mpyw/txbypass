// Package helper finds the helpers of a project that are invokers: they
// open a transaction and invoke a function they are given inside it, as
// s.withTx(ctx, fn) does.
//
// A helper is read from its body. It opens the transaction with an invoker
// or a begin, on a handle that one of its parameters holds, or a
// field of one. Every call of its function parameter runs inside that
// transaction, and is given it. The function parameter goes nowhere else:
// a helper that may run it outside the transaction, or keep it, is no
// helper. A helper of another package says what it is in a fact, which the
// analysis of its package exports.
package helper

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/mpyw/txbypass/internal/closer"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/region"
	"github.com/mpyw/txbypass/internal/txcall"
)

// Invokes says that a function is an invoker, and of what shape.
type Invokes struct {
	Shape txcall.InvokerShape
}

// AFact marks Invokes as an analysis fact.
func (*Invokes) AFact() {}

func (f *Invokes) String() string {
	ctx := ""
	if f.Shape.InContext {
		ctx = ", context"
	}
	return fmt.Sprintf("invokes[p%d%s, p%d(%d)%s]", f.Shape.Outer, helperFields(f.Shape.Fields), f.Shape.Callback, f.Shape.Tx, ctx)
}

// Begins says that a function returns a transaction it begins, and on
// what.
type Begins struct {
	Shape txcall.BeginShape
}

// AFact marks Begins as an analysis fact.
func (*Begins) AFact() {}

func (f *Begins) String() string {
	return "begins[p" + strconv.Itoa(f.Shape.Outer) + helperFields(f.Shape.Fields) + "]"
}

// helperFields spells fields selected one after another, as .db.
func helperFields(names []string) string {
	var b strings.Builder
	for _, name := range names {
		b.WriteString("." + name)
	}
	return b.String()
}

// Helpers finds the helpers, for one pass.
type Helpers struct {
	pass     *analysis.Pass
	prog     *program.Program
	paths    *handle.Paths
	closer   *closer.Closer
	declares func(directive.Kind, *types.Func) bool
	// holds reports whether a value of a type is a transaction.
	holds func(types.Type) bool
	// decls holds the declared functions of the package, by object.
	decls map[*types.Func]*ast.FuncDecl
	// invokers and begins hold the shape of each function of the package
	// already read, or nil when it is no such helper. A function being read
	// is known as no helper, so a helper that calls itself ends.
	invokers map[*types.Func]*txcall.InvokerShape
	begins   map[*types.Func]*txcall.BeginShape
	// txs holds the transactions of each function already listed, as both
	// readers list them for a function that may be either helper.
	txs map[*ssa.Function][]transaction
	// outers holds the handle that each declared function of the package
	// opens on, once read from its body. A function being read is held as
	// opening on no handle of its body, so a body that calls itself ends.
	outers map[*types.Func]*helperOuter
}

// helperOuter is the handle a declared function's body opens on: its
// parameter param, through fields. found is false when the body names no
// single handle.
type helperOuter struct {
	param  int
	fields []string
	nests  bool
	found  bool
}

// New finds the helpers of pass, whose SSA form is prog. declares reports
// whether a function is declared as an invoker or a begin, by a directive of
// the package or a flag. A declaration wins over the body.
func New(pass *analysis.Pass, prog *program.Program, paths *handle.Paths, c *closer.Closer, declares func(directive.Kind, *types.Func) bool, holds func(types.Type) bool) *Helpers {
	h := &Helpers{
		pass: pass, prog: prog, paths: paths, closer: c, declares: declares, holds: holds, decls: make(map[*types.Func]*ast.FuncDecl),
		invokers: make(map[*types.Func]*txcall.InvokerShape), begins: make(map[*types.Func]*txcall.BeginShape),
		txs: make(map[*ssa.Function][]transaction), outers: make(map[*types.Func]*helperOuter),
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Body != nil {
				h.decls[pass.TypesInfo.Defs[fd.Name].(*types.Func)] = fd
			}
		}
	}
	return h
}

// InvokerOf reports the invoker that call is: one of a library, or a
// helper.
func (h *Helpers) InvokerOf(call *ast.CallExpr) (txcall.Invoker, bool) {
	if o, ok := txcall.InvokerOf(h.pass.TypesInfo, call); ok {
		return o, true
	}
	fn, ok := typeutil.Callee(h.pass.TypesInfo, call).(*types.Func)
	if !ok {
		return txcall.Invoker{}, false
	}
	s, ok := h.invokerShape(fn)
	if !ok {
		return txcall.Invoker{}, false
	}
	return txcall.InvokerOfShape(h.pass.TypesInfo, call, fn, s)
}

// BeginOf reports the begin that n assigns: one of a library, or a
// helper.
func (h *Helpers) BeginOf(n ast.Node) (txcall.Begin, bool) {
	return txcall.BeginOf(h.pass.TypesInfo, n, h.beginShape)
}

// GivesTx reports whether fn is an invoker whose callback takes the
// transaction itself as its parameter i: one of a library, or a helper. A
// helper gives it only to the callback, its parameter arg. The receiver of
// a method is parameter 0.
func (h *Helpers) GivesTx(fn *types.Func, arg, i int) bool {
	if txcall.CallbackTx(fn, i) {
		return true
	}
	s, ok := h.invokerShape(fn)
	return ok && !s.InContext && s.Callback == arg && s.Tx == i
}

// GivesContext reports whether fn is an invoker whose callback takes the
// context that carries the transaction as its parameter i: one of a
// library, or a helper. A helper gives it only to the callback, its
// parameter arg.
func (h *Helpers) GivesContext(fn *types.Func, arg, i int) bool {
	if txcall.CallbackContext(fn, i) {
		return true
	}
	s, ok := h.invokerShape(fn)
	return ok && s.InContext && s.Callback == arg && s.Tx == i
}

// IsBegin reports whether fn is a begin: one of a library, or a helper.
func (h *Helpers) IsBegin(fn *types.Func) bool {
	if txcall.IsBeginFunc(fn) {
		return true
	}
	_, ok := h.beginShape(fn)
	return ok
}

// Calls reports whether call calls a helper of another package.
func Calls(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	return ok && fn.Pkg() != pass.Pkg && (pass.ImportObjectFact(fn.Origin(), new(Invokes)) || pass.ImportObjectFact(fn.Origin(), new(Begins)))
}

// Export exports the facts of each helper of the package.
func (h *Helpers) Export() {
	for _, fn := range h.prog.Funcs() {
		obj, ok := fn.Object().(*types.Func)
		if !ok {
			continue
		}
		if s, ok := h.invokerShape(obj); ok {
			h.pass.ExportObjectFact(obj, &Invokes{Shape: s})
		}
		if s, ok := h.beginShape(obj); ok {
			h.pass.ExportObjectFact(obj, &Begins{Shape: s})
		}
	}
}

// invokerShape is the shape of fn, when it is an invoker.
func (h *Helpers) invokerShape(fn *types.Func) (txcall.InvokerShape, bool) {
	// A declaration the signature does not fit declares nothing. A directive
	// that does not fit is reported. A flag's is not, and the body is read
	// as it would be without the flag.
	if h.declares(directive.KindInvokes, fn) {
		// The fact of another package was read from the body there, or
		// from a directive, and names the handle more closely than the
		// signature does.
		var f Invokes
		if fn.Pkg() != h.pass.Pkg && h.pass.ImportObjectFact(fn.Origin(), &f) {
			return f.Shape, true
		}
		if s, problem := txcall.DeclaredInvoker(fn.Origin()); problem == "" {
			if !s.InContext {
				s.Outer, s.Fields, s.Nests = h.declaredOuter(fn, s.Outer)
			}
			return s, true
		}
	}
	var f Invokes
	return helperShape(h, fn, h.invokers, &f, func() txcall.InvokerShape { return f.Shape }, h.readInvoker)
}

// beginShape is the shape of fn, when it returns a transaction it begins.
func (h *Helpers) beginShape(fn *types.Func) (txcall.BeginShape, bool) {
	if h.declares(directive.KindBegins, fn) {
		var f Begins
		if fn.Pkg() != h.pass.Pkg && h.pass.ImportObjectFact(fn.Origin(), &f) {
			return f.Shape, true
		}
		if s, problem := txcall.DeclaredBegin(fn.Origin()); problem == "" {
			s.Outer, s.Fields, s.Nests = h.declaredOuter(fn, s.Outer)
			return s, true
		}
	}
	var f Begins
	return helperShape(h, fn, h.begins, &f, func() txcall.BeginShape { return f.Shape }, h.readBegin)
}

// declaredOuter is the handle a declared fn opens on. A body of the
// package that opens every transaction on one handle, a parameter or a
// field of one, names it, as s.db for a method of s. Otherwise it is the
// parameter the signature gives, outer.
func (h *Helpers) declaredOuter(fn *types.Func, outer int) (int, []string, bool) {
	fn = fn.Origin()
	o, ok := h.outers[fn]
	if !ok {
		h.outers[fn] = &helperOuter{}
		o = h.readOuter(fn)
		h.outers[fn] = o
	}
	if !o.found {
		return outer, nil, false
	}
	return o.param, o.fields, o.nests
}

// readOuter reads the body of fn, when it is of the package, for the one
// handle that it opens every transaction on.
func (h *Helpers) readOuter(fn *types.Func) *helperOuter {
	decl, ok := h.decls[fn]
	if !ok {
		return &helperOuter{}
	}
	sfn := h.prog.Func(decl)
	var found *transaction
	// A savepoint only when every transaction opens one: a call on a
	// transaction then opens a savepoint, whichever path it takes.
	nests := true
	for _, t := range h.transactions(sfn, decl) {
		if found != nil && !found.outer.Equal(t.outer) {
			return &helperOuter{}
		}
		found = &t
		nests = nests && t.nests
	}
	if found == nil {
		return &helperOuter{}
	}
	i, fields, ok := helperParam(sfn, found.outer)
	if !ok {
		return &helperOuter{}
	}
	return &helperOuter{param: i, fields: fields, nests: nests, found: true}
}

// helperShape is the shape of fn: read from its body when it is of the
// package, once, or from fact when it is of another.
func helperShape[S any](h *Helpers, fn *types.Func, known map[*types.Func]*S, fact analysis.Fact, shape func() S, read func(*ast.FuncDecl) *S) (S, bool) {
	var zero S
	fn = fn.Origin()
	decl, local := h.decls[fn]
	if !local {
		if fn.Pkg() == h.pass.Pkg || !h.pass.ImportObjectFact(fn, fact) {
			return zero, false
		}
		return shape(), true
	}
	s, done := known[fn]
	if !done {
		known[fn] = nil
		s = read(decl)
		known[fn] = s
	}
	if s == nil {
		return zero, false
	}
	return *s, true
}

// readInvoker reads decl for a function parameter that it runs inside a
// transaction. The first such parameter is the callback.
func (h *Helpers) readInvoker(decl *ast.FuncDecl) *txcall.InvokerShape {
	fn := h.prog.Func(decl)
	var txs []transaction
	for i, p := range fn.Params {
		if _, ok := p.Type().Underlying().(*types.Signature); !ok {
			continue
		}
		if txs == nil {
			txs = h.transactions(fn, decl)
		}
		if s, ok := h.runs(fn, i, txs); ok {
			return &s
		}
	}
	return nil
}

// readBegin reads decl for a function that returns a transaction it
// begins, open, on a handle that a parameter holds. Its first result is the
// transaction, and its second, if any, an error. Every return gives the
// transaction, from begins on one handle, or nil.
func (h *Helpers) readBegin(decl *ast.FuncDecl) *txcall.BeginShape {
	fn := h.prog.Func(decl)
	res := fn.Signature.Results()
	if res.Len() == 0 || res.Len() > 2 || !h.holds(res.At(0).Type()) || res.Len() == 2 && !types.Identical(res.At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	txs := h.transactions(fn, decl)
	var shape *txcall.BeginShape
	for _, b := range fn.Blocks {
		ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		// A return after a deferred call recovers from a panic takes no
		// part, as a path through a panic takes none in a region.
		if !ok || b == fn.Recover {
			continue
		}
		// A named result that a deferred literal reads is returned through
		// a load of what the return stored.
		if region.Stored(ret.Results[0], helperNil) {
			// No transaction is returned. The caller has to read an error
			// beside it, or it reads a begin that did not happen.
			if len(ret.Results) < 2 || region.Stored(ret.Results[1], helperNil) {
				return nil
			}
			continue
		}
		if len(ret.Results) == 2 && helperRewritten(ret.Results[1]) {
			return nil
		}
		s, ok := h.returned(fn, ret, txs)
		if !ok || shape != nil && !beginShapeEqual(*shape, s) {
			return nil
		}
		shape = &s
	}
	return shape
}

// helperNil reports whether v is the constant nil.
func helperNil(v ssa.Value) bool {
	c, ok := v.(*ssa.Const)
	return ok && c.IsNil()
}

// helperRewritten reports whether v, an error result, is a load of a
// variable that a function literal may set to another error, or whose
// address is handed on, as defer annotate(&err) or check(&err) does. A
// literal that sets it only where it is not nil, as one that wraps it,
// leaves a nil error nil, so a transaction returned beside it stays the
// begin's. A store of the function's own is followed by the region.
func helperRewritten(v ssa.Value) bool {
	load, ok := v.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	alloc, ok := load.X.(*ssa.Alloc)
	if !ok {
		return false
	}
	for _, ref := range *alloc.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp, *ssa.DebugRef:
		case *ssa.Store:
			if ref.Addr != alloc {
				return true
			}
		case *ssa.MakeClosure:
			lit := ref.Fn.(*ssa.Function)
			for i, b := range ref.Bindings {
				if b == alloc && helperWrites(lit.FreeVars[i]) {
					return true
				}
			}
		default:
			return true
		}
	}
	return false
}

// helperWrites reports whether a literal may set fv to an error that is
// not nil where it held nil: by a store not under fv != nil, or by handing
// fv on.
func helperWrites(fv *ssa.FreeVar) bool {
	for _, ref := range *fv.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp, *ssa.DebugRef:
		case *ssa.Store:
			// A store of fv itself, rather than into it, hands it on.
			if ref.Addr != fv || !helperUnderNotNil(ref.Block(), fv) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// helperUnderNotNil reports whether b runs only where fv holds an error
// that is not nil: it is dominated by the true edge of fv != nil.
func helperUnderNotNil(b *ssa.BasicBlock, fv *ssa.FreeVar) bool {
	for c := b; c.Idom() != nil; c = c.Idom() {
		p := c.Idom()
		cond, ok := p.Instrs[len(p.Instrs)-1].(*ssa.If)
		if !ok || !p.Succs[0].Dominates(b) {
			continue
		}
		cmp, ok := cond.Cond.(*ssa.BinOp)
		if !ok || cmp.Op != token.NEQ {
			continue
		}
		x, y := cmp.X, cmp.Y
		if helperNil(x) {
			x, y = y, x
		}
		if load, ok := x.(*ssa.UnOp); ok && load.Op == token.MUL && load.X == fv && helperNil(y) {
			return true
		}
	}
	return false
}

// returned is the shape of a begin in fn whose transaction ret returns,
// open.
func (h *Helpers) returned(fn *ssa.Function, ret *ssa.Return, txs []transaction) (txcall.BeginShape, bool) {
	v := h.paths.Of(ret.Results[0])
	for _, t := range txs {
		if t.region == nil || t.fn != fn || !t.region.Inside(ret) || !slices.ContainsFunc(t.txs, v.Equal) || h.deferredOn(fn, t.txs) {
			continue
		}
		// A second result tells the caller whether the begin failed. Any
		// other error leaves the transaction open where the caller reads
		// none.
		if len(ret.Results) == 2 && !t.region.GivesErr(ret.Results[1]) {
			continue
		}
		if outer, fields, ok := helperParam(fn, t.outer); ok {
			return txcall.BeginShape{Outer: outer, Fields: fields, Nests: t.nests}, true
		}
	}
	return txcall.BeginShape{}, false
}

// deferredOn reports whether a deferred call of fn is given a value of txs,
// as defer tx.Rollback() is, or a literal that holds one. It runs after the
// return, so it may end the transaction before the caller uses it. The
// region does not see it: a deferred call ends nothing where it is written.
func (h *Helpers) deferredOn(fn *ssa.Function, txs []handle.Path) bool {
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			d, ok := instr.(*ssa.Defer)
			if !ok {
				continue
			}
			vals := append([]ssa.Value{d.Call.Value}, d.Call.Args...)
			if mc, ok := d.Call.Value.(*ssa.MakeClosure); ok {
				vals = append(vals, mc.Bindings...)
			}
			for _, v := range vals {
				p := h.paths.Of(v)
				if slices.ContainsFunc(txs, p.Within) {
					return true
				}
			}
		}
	}
	return false
}

// helperParam is the parameter of fn that outer is rooted at, and the
// fields selected from it, by name.
func helperParam(fn *ssa.Function, outer handle.Path) (int, []string, bool) {
	p, ok := outer.Root().(*ssa.Parameter)
	if !ok || p.Parent() != fn {
		return 0, nil, false
	}
	var fields []string
	for _, f := range outer.Fields() {
		fields = append(fields, f.Name())
	}
	return slices.Index(fn.Params, p), fields, true
}

// transaction is one transaction that a helper opens: in a callback, or in
// a region of a begin.
type transaction struct {
	// fn is where the transaction can be used: the callback, or the
	// function that holds the begin.
	fn *ssa.Function
	// region is where a begun transaction is open in fn. It is nil for a
	// callback, which is inside throughout.
	region *region.Region
	// txs are the values of the transaction.
	txs []handle.Path
	// outer is the handle the transaction is opened on.
	outer handle.Path
	// nests reports whether the transaction is a savepoint when outer is
	// a transaction, as Invoker.Nests does.
	nests bool
}

// transactions lists the transactions that fn, whose declaration is decl,
// opens. A transaction that opens a savepoint, or rides in a context, is
// left out.
func (h *Helpers) transactions(fn *ssa.Function, decl *ast.FuncDecl) []transaction {
	if txs, ok := h.txs[fn]; ok {
		return txs
	}
	txs := []transaction{}
	ast.PreorderStack(decl.Body, nil, func(n ast.Node, stack []ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		in := h.prog.Enclosing(fn, stack, n)
		if o, ok := h.InvokerOf(call); ok && !o.InContext && !o.Builds && o.Tx != nil {
			outer, ok := h.Outer(in, o.Outer, o.Receiver, o.Fields)
			if !ok {
				return true
			}
			lit := h.prog.Func(o.Callback)
			obj := h.pass.TypesInfo.Defs[o.Tx]
			i := slices.IndexFunc(lit.Params, func(p *ssa.Parameter) bool { return p.Object() == obj })
			txs = append(txs, transaction{fn: lit, txs: []handle.Path{h.paths.Of(lit.Params[i])}, outer: outer, nests: o.Nests})
			return true
		}
		b, ok := txcall.BeginCall(h.pass.TypesInfo, call, h.beginShape)
		if !ok {
			return true
		}
		begin, ok := program.Call(in, call)
		if !ok {
			return true
		}
		outer, ok := h.Outer(in, b.Outer, b.Receiver, b.Fields)
		if !ok {
			return true
		}
		r := region.Begun(begin, h.closer, h.paths)
		txs = append(txs, transaction{fn: in, region: r, txs: r.Txs(), outer: outer, nests: b.Nests})
		return true
	})
	h.txs[fn] = txs
	return txs
}

// Outer is the handle that expr gives, in fn, as an invoker or a begin
// gives its handle: the receiver of a method, through the embedded fields
// its promotion passes, then fields.
func (h *Helpers) Outer(fn *ssa.Function, expr ast.Expr, receiver bool, fields []*types.Var) (handle.Path, bool) {
	var promoted []*types.Var
	if receiver {
		sel := expr.(*ast.SelectorExpr)
		expr = sel.X
		promoted = handle.Promoted(h.pass.TypesInfo.Selections[sel])
	}
	ref, ok := h.prog.Ref(fn, expr)
	if !ok {
		return handle.Path{}, false
	}
	return h.paths.Of(ref.X).With(promoted).With(fields), true
}

// runs reports the shape of fn when its parameter i is a callback that it
// runs only inside transactions of txs, opened on one handle that a
// parameter holds, and is given the transaction at one place.
func (h *Helpers) runs(fn *ssa.Function, i int, txs []transaction) (txcall.InvokerShape, bool) {
	cb := h.paths.Of(fn.Params[i])
	var shape *txcall.InvokerShape
	ok := true
	program.Walk(fn, func(instr ssa.Instruction) {
		if !ok {
			return
		}
		if call, isCall := instr.(ssa.CallInstruction); isCall && h.paths.Of(call.Common().Value).Equal(cb) {
			s, runs := h.runInside(fn, call, txs)
			ok = runs && (shape == nil || shapeEqual(*shape, s))
			shape = &s
			return
		}
		ok = !h.gives(instr, cb)
	})
	if !ok || shape == nil {
		return txcall.InvokerShape{}, false
	}
	shape.Callback = i
	return *shape, true
}

// runInside reports the shape of a call of the callback, when it is a plain
// call inside one of txs, given the transaction.
func (h *Helpers) runInside(fn *ssa.Function, call ssa.CallInstruction, txs []transaction) (txcall.InvokerShape, bool) {
	c, ok := call.(*ssa.Call)
	if !ok {
		// A go statement may outlive the transaction, and a deferred call
		// runs after the code that may end it.
		return txcall.InvokerShape{}, false
	}
	for _, t := range txs {
		if t.fn != c.Parent() || t.region != nil && !t.region.Inside(c) {
			continue
		}
		at := slices.IndexFunc(c.Call.Args, func(arg ssa.Value) bool {
			return slices.ContainsFunc(t.txs, h.paths.Of(arg).Equal)
		})
		if at < 0 {
			continue
		}
		if outer, fields, ok := helperParam(fn, t.outer); ok {
			return txcall.InvokerShape{Outer: outer, Fields: fields, Tx: at, Nests: t.nests}, true
		}
	}
	return txcall.InvokerShape{}, false
}

// gives reports whether instr hands the callback cb on, other than by
// calling it: as an argument, a stored value, or one edge of a phi. A store
// into a local variable moves nothing out of reach: the loads of it are the
// callback again. A debug reference does nothing, and neither does a
// comparison: a function is only compared with nil, as if fn == nil does.
func (h *Helpers) gives(instr ssa.Instruction, cb handle.Path) bool {
	switch instr := instr.(type) {
	case *ssa.DebugRef, *ssa.BinOp:
		return false
	case *ssa.Store:
		// Only a variable written once, as a whole, gives the callback at
		// every load. One written more than once is a value of its own, and
		// a use of it is not seen as the callback's.
		if alloc, local := instr.Addr.(*ssa.Alloc); local && h.paths.Of(alloc).Equal(cb) {
			return false
		}
	}
	for _, op := range instr.Operands(nil) {
		if *op == nil {
			continue
		}
		if _, isFunc := (*op).Type().Underlying().(*types.Signature); isFunc && h.paths.Of(*op).Equal(cb) {
			return true
		}
	}
	return false
}

// beginShapeEqual reports whether a and b, read from two returns, are one
// shape.
func beginShapeEqual(a, b txcall.BeginShape) bool {
	return a.Outer == b.Outer && slices.Equal(a.Fields, b.Fields) && a.Nests == b.Nests
}

// shapeEqual reports whether a and b, read from two calls of one callback,
// are one shape. The callback is set once the calls agree.
func shapeEqual(a, b txcall.InvokerShape) bool {
	return a.Outer == b.Outer && slices.Equal(a.Fields, b.Fields) && a.Tx == b.Tx && a.Nests == b.Nests
}
