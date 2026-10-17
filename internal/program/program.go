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
	"go/types"
	"slices"

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
