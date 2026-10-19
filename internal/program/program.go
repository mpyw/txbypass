// Package program builds a package's SSA form, with the debug information
// that ties each expression of the source to its value.
//
// The rules find what to report in the syntax, and ask the SSA form what a
// name holds: two names that hold one value are one handle, and a call on a
// copy of a transaction is a call on the transaction. buildssa builds no
// debug information, so this package builds its own program.
package program

import (
	"cmp"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"
)

// Program is one package's SSA form, and the ways to reach it from the
// syntax.
type Program struct {
	// init is the package initializer. It runs the initializers of the
	// package-level variables, and makes the literals in them.
	init *ssa.Function
	// funcs maps each function declaration and literal to its function.
	funcs map[ast.Node]*ssa.Function
	// refs maps each function's expressions to their debug references.
	refs map[*ssa.Function]map[ast.Expr]*ssa.DebugRef
}

// Build builds the SSA form of pass's package. noReturn reports the
// functions that never return, so that a call of one ends its block.
func Build(pass *analysis.Pass, noReturn func(*types.Func) bool) *Program {
	prog := ssa.NewProgram(pass.Fset, ssa.GlobalDebug)
	prog.SetNoReturn(noReturn)
	for _, p := range pass.Pkg.Imports() {
		prog.CreatePackage(p, nil, nil, true)
	}
	pkg := prog.CreatePackage(pass.Pkg, pass.Files, pass.TypesInfo, false)
	pkg.Build()
	p := &Program{init: pkg.Func("init"), funcs: make(map[ast.Node]*ssa.Function), refs: make(map[*ssa.Function]map[ast.Expr]*ssa.DebugRef)}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			if decl, ok := decl.(*ast.FuncDecl); ok {
				p.addFunc(prog.FuncValue(pass.TypesInfo.Defs[decl.Name].(*types.Func)))
			}
		}
	}
	for _, anon := range p.init.AnonFuncs {
		p.addFunc(anon)
	}
	return p
}

// Init is the package initializer, which holds the code of the
// package-level declarations.
func (p *Program) Init() *ssa.Function {
	return p.init
}

// addFunc records fn and the literals inside it.
func (p *Program) addFunc(fn *ssa.Function) {
	p.funcs[fn.Syntax()] = fn
	for _, anon := range fn.AnonFuncs {
		p.addFunc(anon)
	}
}

// Func is the function of a declaration or a literal, or of the body of a
// range over a function. It is nil for any other range statement.
func (p *Program) Func(n ast.Node) *ssa.Function {
	return p.funcs[n]
}

// Enclosing is the function that evaluates n, whose ancestors are stack:
// the innermost function literal around it, or loop body of a range over a
// function, or else outer. The builder makes such a loop body a function of
// its own, which the range statement names. Its range expression is
// evaluated outside it.
func (p *Program) Enclosing(outer *ssa.Function, stack []ast.Node, n ast.Node) *ssa.Function {
	child := n
	for _, a := range slices.Backward(stack) {
		switch a := a.(type) {
		case *ast.FuncLit:
			return p.Func(a)
		case *ast.RangeStmt:
			if fn := p.Func(a); fn != nil && child == a.Body {
				return fn
			}
		}
		child = a
	}
	return outer
}

// Funcs lists the package's declared functions and their literals, in the
// order of their positions, so that a pass reads them the same way each
// time.
func (p *Program) Funcs() []*ssa.Function {
	out := make([]*ssa.Function, 0, len(p.funcs))
	for _, fn := range p.funcs {
		out = append(out, fn)
	}
	slices.SortFunc(out, func(a, b *ssa.Function) int { return cmp.Compare(a.Pos(), b.Pos()) })
	return out
}

// Ref is the debug reference of e in fn: the value e evaluates to, or its
// address when e is a variable used through its address, and the
// instruction where it does. An expression that is not evaluated, such as
// a type, or the method in a call, has none.
func (p *Program) Ref(fn *ssa.Function, e ast.Expr) (*ssa.DebugRef, bool) {
	refs, ok := p.refs[fn]
	if !ok {
		refs = make(map[ast.Expr]*ssa.DebugRef)
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				if ref, ok := instr.(*ssa.DebugRef); ok {
					refs[ref.Expr] = ref
				}
			}
		}
		p.refs[fn] = refs
	}
	e = ast.Unparen(e)
	if ref, ok := refs[e]; ok {
		return ref, true
	}
	// A field whose address is taken, as the receiver of r.db.Begin() when
	// db is held by value, has its reference on the name after the dot.
	if sel, ok := e.(*ast.SelectorExpr); ok {
		ref, ok := refs[sel.Sel]
		return ref, ok && ref.IsAddr
	}
	return nil, false
}

// Call is the SSA call of call, in fn. A call whose results are assigned
// to several variables has no debug reference, so it is found by its
// position, which is the call's opening parenthesis. A call in code that
// no path reaches has none: the builder deletes its block.
func Call(fn *ssa.Function, call *ast.CallExpr) (*ssa.Call, bool) {
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if c, ok := instr.(*ssa.Call); ok && c.Pos() == call.Lparen {
				return c, true
			}
		}
	}
	return nil, false
}

// Creation is the instruction of fn's parent that makes fn: the closure
// that binds its free variables, or, for a literal with none, the
// instruction that names it. A literal in code that no path reaches, or
// assigned to the blank identifier, is made by none: it never runs. A
// closure that nothing uses but a debug reference does not count. A call
// that names a literal runs it, so it counts even when its result is not
// used.
func Creation(fn *ssa.Function) (ssa.Instruction, bool) {
	var rands []*ssa.Value
	for _, b := range fn.Parent().Blocks {
		for _, instr := range b.Instrs {
			if _, ok := instr.(*ssa.DebugRef); ok {
				continue
			}
			for _, r := range instr.Operands(rands[:0]) {
				if *r == fn && (runs(instr) || Used(instr)) {
					return instr, true
				}
			}
		}
	}
	return nil, false
}

// Core is the underlying type of t, or for a type parameter, the one
// underlying type of every type in its type set, as the SSA form reads a
// composite literal of it. A type parameter with no such type has none,
// and Core gives its constraint.
func Core(t types.Type) types.Type {
	tp, ok := types.Unalias(t).(*types.TypeParam)
	if !ok {
		return t.Underlying()
	}
	set := typeSet(tp.Constraint())
	if set.all || len(set.types) == 0 || slices.ContainsFunc(set.types, func(u types.Type) bool { return !types.Identical(u, set.types[0]) }) {
		return tp.Underlying()
	}
	return set.types[0]
}

// ChanElem is the element type of a channel of type t. For a type
// parameter, its type set may mix directions, as chan T | chan<- T, which
// a send allows when every channel has one element type.
func ChanElem(t types.Type) (types.Type, bool) {
	if ch, ok := Core(t).(*types.Chan); ok {
		return ch.Elem(), true
	}
	tp, ok := types.Unalias(t).(*types.TypeParam)
	if !ok {
		return nil, false
	}
	set := typeSet(tp.Constraint())
	var elem types.Type
	for _, u := range set.types {
		ch, ok := u.(*types.Chan)
		if !ok || elem != nil && !types.Identical(elem, ch.Elem()) {
			return nil, false
		}
		elem = ch.Elem()
	}
	return elem, elem != nil && !set.all
}

// underlyings is a set of underlying types, or every type.
type underlyings struct {
	all   bool
	types []types.Type
}

// typeSet is the set of underlying types t allows as a constraint. An
// interface allows what all its embedded types allow, and a union what
// any of its terms allows. Its methods narrow no underlying type.
func typeSet(t types.Type) underlyings {
	switch t := types.Unalias(t).(type) {
	case *types.Union:
		var set underlyings
		for term := range t.Terms() {
			s := typeSet(term.Type())
			set.all = set.all || s.all
			set.types = append(set.types, s.types...)
		}
		return set
	}
	iface, ok := t.Underlying().(*types.Interface)
	if !ok {
		return underlyings{types: []types.Type{t.Underlying()}}
	}
	set := underlyings{all: true}
	for e := range iface.EmbeddedTypes() {
		s := typeSet(e)
		switch {
		case s.all:
		case set.all:
			set = s
		default:
			set.types = slices.DeleteFunc(set.types, func(u types.Type) bool {
				return !slices.ContainsFunc(s.types, func(v types.Type) bool { return types.Identical(u, v) })
			})
		}
	}
	return set
}

// Yield reports whether fn is the function the builder makes of the body
// of a range over a function.
func Yield(fn *ssa.Function) bool {
	return fn.Synthetic == "range-over-func yield"
}

// Cleanup reports whether call registers a function to run after the test,
// as t.Cleanup(f) does. The function runs as a deferred call of the test
// does, after the test returns.
func Cleanup(call *ssa.CallCommon) bool {
	var fn *types.Func
	if call.IsInvoke() {
		fn = call.Method
	} else if callee := call.StaticCallee(); callee != nil {
		fn, _ = callee.Object().(*types.Func)
	}
	return fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == "testing" && fn.Name() == "Cleanup"
}

// CleanupCall reports whether call, in the syntax, registers a function to
// run after the test, as Cleanup says.
func CleanupCall(info *types.Info, call *ast.CallExpr) bool {
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "testing" && fn.Name() == "Cleanup"
}

// runs reports whether instr is a call, a go or a defer, which runs what it
// names whatever becomes of its result.
func runs(instr ssa.Instruction) bool {
	_, ok := instr.(ssa.CallInstruction)
	return ok
}

// Used reports whether instr is used by anything but a debug reference. An
// instruction that is not a value, such as a call that names a literal, is
// a use in itself.
func Used(instr ssa.Instruction) bool {
	v, ok := instr.(ssa.Value)
	if !ok {
		return true
	}
	for _, ref := range *v.Referrers() {
		if _, ok := ref.(*ssa.DebugRef); !ok {
			return true
		}
	}
	return false
}

// Walk calls visit for each instruction of fn and of the literals inside
// it.
func Walk(fn *ssa.Function, visit func(ssa.Instruction)) {
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			visit(instr)
		}
	}
	for _, anon := range fn.AnonFuncs {
		Walk(anon, visit)
	}
}

// YieldGoesOn reports whether a path from block b of the body that mc makes
// goes on in the loop or after it. The body returns true
// for the next iteration. It returns false to leave the loop, with the exit
// it takes stored in its jump variable. The function that holds the loop
// branches on that exit: to a return for a return in the body, or past the
// loop for a break.
func YieldGoesOn(mc *ssa.MakeClosure, b *ssa.BasicBlock) bool {
	seen := map[*ssa.BasicBlock]bool{b: true}
	for work := []*ssa.BasicBlock{b}; len(work) > 0; {
		b, work = work[len(work)-1], work[:len(work)-1]
		ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		if ok && !yieldReturns(mc, b, ret) {
			return true
		}
		for _, s := range b.Succs {
			if !seen[s] {
				seen[s] = true
				work = append(work, s)
			}
		}
	}
	return false
}

// yieldReturns reports whether ret, at the end of block b of the body that
// mc makes, returns from the function that holds the loop.
func yieldReturns(mc *ssa.MakeClosure, b *ssa.BasicBlock, ret *ssa.Return) bool {
	if done, ok := ret.Results[0].(*ssa.Const); !ok || constant.BoolVal(done.Value) {
		return false
	}
	fn := mc.Fn.(*ssa.Function)
	// The last store is the exit. A body with no branch stores that it is
	// running first, in the same block.
	var st *ssa.Store
	for _, instr := range b.Instrs {
		if yieldExit(instr) {
			st = instr.(*ssa.Store)
		}
	}
	if st == nil {
		// Unreachable: the builder stores the exit in the jump variable
		// before each return of false.
		return false
	}
	return yieldExitReturns(mc.Parent(), mc.Bindings[slices.Index(fn.FreeVars, st.Addr.(*ssa.FreeVar))], st.Val.(*ssa.Const))
}

// yieldOnlyReturns reports whether b does nothing but return: it loads the
// results, runs the deferred calls, and returns. In the body of an outer
// loop, it stores the results and the exit for that loop's function first.
func yieldOnlyReturns(b *ssa.BasicBlock) bool {
	for _, instr := range b.Instrs[:len(b.Instrs)-1] {
		switch instr.(type) {
		case *ssa.UnOp, *ssa.Store, *ssa.RunDefers, *ssa.DebugRef:
		default:
			return false
		}
	}
	return true
}

// yieldExit reports whether instr stores a constant exit in the jump
// variable of the body of a range over a function.
func yieldExit(instr ssa.Instruction) bool {
	st, ok := instr.(*ssa.Store)
	if !ok {
		return false
	}
	fv, isFree := st.Addr.(*ssa.FreeVar)
	_, isConst := st.Val.(*ssa.Const)
	return isFree && isConst && strings.HasPrefix(fv.Name(), "jump$")
}

// yieldExitReturns reports whether fn, which holds the loop, returns when
// the body leaves it by exit, which the body stored in jump. When fn is
// itself the body of an outer loop, it returns false for a break too, so
// its return is read as the body's own.
func yieldExitReturns(fn *ssa.Function, jump ssa.Value, exit *ssa.Const) bool {
	for _, b := range fn.Blocks {
		cond, ok := b.Instrs[len(b.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		cmp, ok := cond.Cond.(*ssa.BinOp)
		if !ok || cmp.Op != token.EQL {
			continue
		}
		load, ok := cmp.X.(*ssa.UnOp)
		k, isConst := cmp.Y.(*ssa.Const)
		if ok && load.X == jump && isConst && constant.Compare(k.Value, token.EQL, exit.Value) {
			then := b.Succs[0]
			ret, returns := then.Instrs[len(then.Instrs)-1].(*ssa.Return)
			// A return in the body comes back through a block that only
			// loads the results the body stored, and returns them. A labeled
			// break or a goto out of the body jumps to its target, which may
			// end in a return of its own after code of its own. The optimizer
			// may merge such a target into the block the builder made.
			returns = returns && yieldOnlyReturns(then)
			if !returns || !Yield(fn) {
				return returns
			}
			made, _ := Creation(fn)
			outer, ok := made.(*ssa.MakeClosure)
			return ok && yieldReturns(outer, then, ret)
		}
	}
	return false
}

// LastStored reports whether every path to instruction at of b last stores
// a value that v accepts to addr. A variable stored on one path only has a
// path that reaches the entry with no store, so it holds no such value
// there. A block met again around a loop leaves the answer to the other
// paths.
func LastStored(b *ssa.BasicBlock, at int, addr ssa.Value, v func(ssa.Value) bool, seen map[*ssa.BasicBlock]bool) bool {
	for i := at - 1; i >= 0; i-- {
		if st, ok := b.Instrs[i].(*ssa.Store); ok && st.Addr == addr {
			return v(st.Val)
		}
	}
	// A path that reaches the entry stored nothing to addr, so addr does not
	// hold the begin's error there.
	if len(b.Preds) == 0 {
		return false
	}
	if seen[b] {
		return true
	}
	seen[b] = true
	for _, p := range b.Preds {
		if !LastStored(p, len(p.Instrs), addr, v, seen) {
			return false
		}
	}
	return true
}

// IsNil reports whether v is the constant nil.
func IsNil(v ssa.Value) bool {
	c, ok := v.(*ssa.Const)
	return ok && c.IsNil()
}

// UnderNotNil reports whether b runs only where the variable at addr holds
// a value that is not nil: it is dominated by the true edge of a test that
// a load of addr is not nil.
func UnderNotNil(b *ssa.BasicBlock, addr ssa.Value) bool {
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
		if IsNil(x) {
			x, y = y, x
		}
		if load, ok := x.(*ssa.UnOp); ok && load.Op == token.MUL && load.X == addr && IsNil(y) {
			return true
		}
	}
	return false
}

// Deferred reports whether the literal that made makes runs only when
// the function returns: made is a defer of it or a t.Cleanup of it, or a
// closure that only those use.
func Deferred(made ssa.Instruction) bool {
	if defers(made) {
		return true
	}
	mc, ok := made.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, ref := range *mc.Referrers() {
		if _, ok := ref.(*ssa.DebugRef); !ok && !defers(ref) {
			return false
		}
	}
	return true
}

// defers reports whether instr is a defer, or a t.Cleanup.
func defers(instr ssa.Instruction) bool {
	switch instr := instr.(type) {
	case *ssa.Defer:
		return true
	case *ssa.Call:
		return Cleanup(instr.Common())
	}
	return false
}

// Param is the parameter of fn that obj declares.
func Param(fn *ssa.Function, obj types.Object) ssa.Value {
	for _, p := range fn.Params {
		if p.Object() == obj {
			return p
		}
	}
	// Unreachable: each caller asks for a parameter of the declaration
	// that the builder made fn from.
	panic("program: no parameter of fn for the object")
}
