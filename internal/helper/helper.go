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
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/mpyw/txbypass/internal/closer"
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
	var b strings.Builder
	fmt.Fprintf(&b, "invokes[p%d", f.Shape.Outer)
	for _, name := range f.Shape.Fields {
		b.WriteString("." + name)
	}
	fmt.Fprintf(&b, ", p%d(%d)]", f.Shape.Callback, f.Shape.Tx)
	return b.String()
}

// Helpers finds the helpers, for one pass.
type Helpers struct {
	pass   *analysis.Pass
	prog   *program.Program
	paths  *handle.Paths
	closer *closer.Closer
	// decls holds the declared functions of the package, by object.
	decls map[*types.Func]*ast.FuncDecl
	// known holds the shape of each function of the package already read,
	// or nil when it is no helper. A function being read is known as no
	// helper, so a helper that calls itself ends.
	known map[*types.Func]*txcall.InvokerShape
}

// New finds the helpers of pass, whose SSA form is prog.
func New(pass *analysis.Pass, prog *program.Program, paths *handle.Paths, c *closer.Closer) *Helpers {
	h := &Helpers{pass: pass, prog: prog, paths: paths, closer: c, decls: make(map[*types.Func]*ast.FuncDecl), known: make(map[*types.Func]*txcall.InvokerShape)}
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
	s, ok := h.shape(fn)
	if !ok {
		return txcall.Invoker{}, false
	}
	return txcall.InvokerOfShape(h.pass.TypesInfo, call, fn, s)
}

// Calls reports whether call calls a helper of another package.
func Calls(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	return ok && pass.ImportObjectFact(fn.Origin(), new(Invokes))
}

// Export exports the fact of each helper of the package.
func (h *Helpers) Export() {
	for _, fn := range h.prog.Funcs() {
		obj, ok := fn.Object().(*types.Func)
		if !ok {
			continue
		}
		if s, ok := h.shape(obj); ok {
			h.pass.ExportObjectFact(obj, &Invokes{Shape: s})
		}
	}
}

// shape is the shape of fn, when it is a helper.
func (h *Helpers) shape(fn *types.Func) (txcall.InvokerShape, bool) {
	fn = fn.Origin()
	decl, local := h.decls[fn]
	if !local {
		var f Invokes
		if fn.Pkg() == h.pass.Pkg || !h.pass.ImportObjectFact(fn, &f) {
			return txcall.InvokerShape{}, false
		}
		return f.Shape, true
	}
	s, done := h.known[fn]
	if !done {
		h.known[fn] = nil
		s = h.read(decl)
		h.known[fn] = s
	}
	if s == nil {
		return txcall.InvokerShape{}, false
	}
	return *s, true
}

// read reads decl for a function parameter that it runs inside a
// transaction. The first such parameter is the callback.
func (h *Helpers) read(decl *ast.FuncDecl) *txcall.InvokerShape {
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
}

// transactions lists the transactions that fn, whose declaration is decl,
// opens. A transaction that opens a savepoint, or rides in a context, is
// left out.
func (h *Helpers) transactions(fn *ssa.Function, decl *ast.FuncDecl) []transaction {
	info := h.pass.TypesInfo
	txs := []transaction{}
	ast.PreorderStack(decl.Body, nil, func(n ast.Node, stack []ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		in := h.prog.Enclosing(fn, stack, n)
		if o, ok := h.InvokerOf(call); ok && !o.InContext && !o.Builds && o.Tx != nil {
			if t, ok := h.callbackTx(in, o); ok {
				txs = append(txs, t)
			}
			return true
		}
		if !txcall.IsBegin(info, call) {
			return true
		}
		begin, ok := program.Call(in, call)
		if !ok {
			return true
		}
		// The receiver of a method call is always evaluated as a value.
		sel := ast.Unparen(call.Fun).(*ast.SelectorExpr)
		ref, _ := h.prog.Ref(in, sel.X)
		r := region.Begun(begin, h.closer, h.paths)
		outer := h.paths.Of(ref.X).With(handle.Promoted(info.Selections[sel]))
		txs = append(txs, transaction{fn: in, region: r, txs: r.Txs(), outer: outer})
		return true
	})
	return txs
}

// callbackTx is the transaction that o opens, called in fn.
func (h *Helpers) callbackTx(fn *ssa.Function, o txcall.Invoker) (transaction, bool) {
	expr := o.Outer
	var promoted []*types.Var
	if o.Receiver {
		sel := o.Outer.(*ast.SelectorExpr)
		expr = sel.X
		promoted = handle.Promoted(h.pass.TypesInfo.Selections[sel])
	}
	ref, ok := h.prog.Ref(fn, expr)
	if !ok {
		return transaction{}, false
	}
	lit := h.prog.Func(o.Callback)
	obj := h.pass.TypesInfo.Defs[o.Tx]
	i := slices.IndexFunc(lit.Params, func(p *ssa.Parameter) bool { return p.Object() == obj })
	outer := h.paths.Of(ref.X).With(promoted).With(o.Fields)
	return transaction{fn: lit, txs: []handle.Path{h.paths.Of(lit.Params[i])}, outer: outer}, true
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
		p, ok := t.outer.Root().(*ssa.Parameter)
		if !ok || p.Parent() != fn {
			continue
		}
		s := txcall.InvokerShape{Outer: slices.Index(fn.Params, p), Tx: at}
		for _, f := range t.outer.Fields() {
			s.Fields = append(s.Fields, f.Name())
		}
		return s, true
	}
	return txcall.InvokerShape{}, false
}

// gives reports whether instr hands the callback cb on, other than by
// calling it: as an argument, a stored value, or one edge of a phi. A store
// into a local variable moves nothing out of reach: the loads of it are the
// callback again. A debug reference does nothing.
func (h *Helpers) gives(instr ssa.Instruction, cb handle.Path) bool {
	switch instr := instr.(type) {
	case *ssa.DebugRef:
		return false
	case *ssa.Store:
		if _, local := instr.Addr.(*ssa.Alloc); local {
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

// shapeEqual reports whether a and b are one shape.
func shapeEqual(a, b txcall.InvokerShape) bool {
	return a.Outer == b.Outer && slices.Equal(a.Fields, b.Fields) && a.Callback == b.Callback && a.Tx == b.Tx
}
