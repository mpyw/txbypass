// Package txhelper finds the helpers of a project that are invokers: they
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
package txhelper

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/mpyw/txbypass/internal/declared"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/fact"
	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/region"
	"github.com/mpyw/txbypass/internal/txcall"
	"github.com/mpyw/txbypass/internal/txcloser"
)

// Helpers finds the helpers, for one pass.
type Helpers struct {
	pass *analysis.Pass
	// prog is the package's SSA form, and paths says which handle each
	// value of it is.
	//
	//declscope:shared // invoker.go and begin.go read helpers through them
	prog *program.Program
	//declscope:shared // invoker.go and begin.go read helpers through them
	paths  *handle.Paths
	closer *txcloser.Closer
	// decl tells what is declared, and which types are transactions.
	//
	//declscope:shared // begin.go asks it which types are transactions
	decl *declared.Set
	// decls holds the declared functions of the package, by object.
	decls map[*types.Func]*ast.FuncDecl
	// invokers and begins hold the shape of each function of the package
	// already read, or nil when it is no such helper. A function being read
	// is known as no helper, so a helper that calls itself ends.
	invokers map[*types.Func]*txcall.InvokerShape
	begins   map[*types.Func]*txcall.BeginShape
	// txs holds the transactions of each function already listed, as both
	// readers list them for a function that may be either helper.
	txs map[*ssa.Function][]helperTransaction
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

// NewHelpers finds the helpers of pass, whose SSA form is prog. decl tells the
// functions declared as invokers or begins, by a directive of the package
// or a flag, and the types whose values are transactions. A declaration
// wins over the body.
func NewHelpers(pass *analysis.Pass, prog *program.Program, paths *handle.Paths, c *txcloser.Closer, decl *declared.Set) *Helpers {
	h := &Helpers{
		pass: pass, prog: prog, paths: paths, closer: c, decl: decl, decls: make(map[*types.Func]*ast.FuncDecl),
		invokers: make(map[*types.Func]*txcall.InvokerShape), begins: make(map[*types.Func]*txcall.BeginShape),
		txs: make(map[*ssa.Function][]helperTransaction), outers: make(map[*types.Func]*helperOuter),
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
	if txcall.InvokerGivesTx(fn, i) {
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
	if txcall.InvokerGivesContext(fn, i) {
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

// CallsHelper reports whether call calls a helper of another package.
func CallsHelper(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	return ok && fn.Pkg() != pass.Pkg && (pass.ImportObjectFact(fn.Origin(), new(fact.Invokes)) || pass.ImportObjectFact(fn.Origin(), new(fact.Begins)))
}

// Export exports the facts of each helper of the package.
func (h *Helpers) Export() {
	for _, fn := range h.prog.Funcs() {
		obj, ok := fn.Object().(*types.Func)
		if !ok {
			continue
		}
		if s, ok := h.invokerShape(obj); ok {
			h.pass.ExportObjectFact(obj, &fact.Invokes{Shape: s})
		}
		if s, ok := h.beginShape(obj); ok {
			h.pass.ExportObjectFact(obj, &fact.Begins{Shape: s})
		}
	}
}

// invokerShape is the shape of fn, when it is an invoker.
func (h *Helpers) invokerShape(fn *types.Func) (txcall.InvokerShape, bool) {
	// A declaration the signature does not fit declares nothing. A directive
	// that does not fit is reported. A flag's is not, and the body is read
	// as it would be without the flag.
	if h.decl.Declares(directive.KindInvokes, fn) {
		// The fact of another package was read from the body there, or
		// from a directive, and names the handle more closely than the
		// signature does.
		var f fact.Invokes
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
	var f fact.Invokes
	return helperShape(h, fn, h.invokers, &f, func() txcall.InvokerShape { return f.Shape }, h.readInvoker)
}

// beginShape is the shape of fn, when it returns a transaction it begins.
func (h *Helpers) beginShape(fn *types.Func) (txcall.BeginShape, bool) {
	if h.decl.Declares(directive.KindBegins, fn) {
		var f fact.Begins
		if fn.Pkg() != h.pass.Pkg && h.pass.ImportObjectFact(fn.Origin(), &f) {
			return f.Shape, true
		}
		if s, problem := txcall.DeclaredBegin(fn.Origin()); problem == "" {
			s.Outer, s.Fields, s.Nests = h.declaredOuter(fn, s.Outer)
			return s, true
		}
	}
	var f fact.Begins
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
	var found *helperTransaction
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

// helperParam is the parameter of fn that outer is rooted at, and the
// fields selected from it, by name.
//
//declscope:shared // invoker.go and begin.go name the handle a helper opens on
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

// helperTransaction is one transaction that a helper opens: in a callback,
// or in a region of a begin.
//
//declscope:shared // invoker.go and begin.go read the transactions a helper opens
type helperTransaction struct {
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
//
//declscope:shared // invoker.go and begin.go read the transactions a helper opens
func (h *Helpers) transactions(fn *ssa.Function, decl *ast.FuncDecl) []helperTransaction {
	if txs, ok := h.txs[fn]; ok {
		return txs
	}
	txs := []helperTransaction{}
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
			txs = append(txs, helperTransaction{fn: lit, txs: []handle.Path{h.paths.Of(lit.Params[i])}, outer: outer, nests: o.Nests})
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
		txs = append(txs, helperTransaction{fn: in, region: r, txs: r.Txs(), outer: outer, nests: b.Nests})
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
