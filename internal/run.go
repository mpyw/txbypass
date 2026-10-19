// Package internal implements the txbypass rules.
//
// Each rule has its own file. They share one pass's state, in checker.go:
// the directives, the files that are generated, and the package's SSA
// form. They read the code inside a transaction through scope.go. Which
// calls open a transaction, what a name holds, what may end a transaction
// and where a begun one is open, live in the packages below this one.
package internal

import (
	"errors"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/mpyw/txbypass/internal/closer"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/helper"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/txcall"
)

// errRunWithoutCtrlflow is returned by Run when the pass carries no
// ctrlflow result, which means the analyzer was registered without its
// requirement.
var errRunWithoutCtrlflow = errors.New("txbypass: ctrlflow result missing")

// Run analyzes one package. The SSA form is built only for a package that
// opens a transaction, or has a function that takes one: the others have
// nothing to report and no fact to export.
func Run(pass *analysis.Pass, cfg Config) (any, error) {
	cfgs, ok := pass.ResultOf[ctrlflow.Analyzer].(*ctrlflow.CFGs)
	if !ok {
		return nil, errRunWithoutCtrlflow
	}
	c := newChecker(pass, cfg)
	for _, p := range c.directives.Problems() {
		c.reportf(p.Pos, p.Message)
	}
	c.runDeclarations()
	if runNeedsProgram(pass, c.declares) {
		c.prog = program.Build(pass, cfgs.NoReturn)
		c.paths = handle.NewPaths(runModule(pass.Module))
		c.closer = closer.New(pass, c.paths, func(fn *types.Func) bool { return c.declares(directive.Closes, fn) })
		c.closer.Export(c.prog)
		c.helpers = helper.New(pass, c.prog, c.paths, c.closer, c.declares)
		c.helpers.Export()
		for _, f := range pass.Files {
			// Nothing is reported in a generated file, so it is not
			// analyzed.
			if !c.inGenerated(f.Pos()) {
				c.runFile(f)
			}
		}
	}
	// Every function the pass reports on is seen in full, in a package's
	// test variant as in the ordinary one, so an ignore that silenced
	// nothing here silences nothing anywhere.
	for _, pos := range c.directives.Unused(cfg.On) {
		c.reportf(pos, "unused txbypass:ignore directive")
	}
	c.flush()
	return nil, nil
}

// runDeclarations checks each declaring directive against the signature it
// declares, and exports the fact of each that fits, so that a package that
// calls the function reads it. One that does not fit is reported, and
// declares nothing.
func (c *checker) runDeclarations() {
	for _, d := range c.directives.Declarations() {
		var fact analysis.Fact
		var problem string
		switch d.Kind {
		case directive.Invokes:
			var s txcall.InvokerShape
			s, problem = txcall.DeclaredInvoker(d.Obj)
			fact = &helper.Invokes{Shape: s}
		case directive.Begins:
			var s txcall.BeginShape
			s, problem = txcall.DeclaredBegin(d.Obj)
			fact = &helper.Begins{Shape: s}
		case directive.Closes:
			problem = txcall.DeclaredCloser(d.Obj)
			fact = &closer.Fact{Params: runAllParams(d.Obj)}
		}
		if problem != "" {
			c.reportf(d.Pos, "txbypass:"+string(d.Kind)+" "+problem)
			continue
		}
		if c.declared[d.Kind] == nil {
			c.declared[d.Kind] = make(map[*types.Func]bool)
		}
		c.declared[d.Kind][d.Obj] = true
		c.pass.ExportObjectFact(d.Obj, fact)
	}
}

// runAllParams lists every parameter of fn, the receiver of a method as 0.
func runAllParams(fn *types.Func) []int {
	n := fn.Signature().Params().Len()
	if fn.Signature().Recv() != nil {
		n++
	}
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// runModule is the path of module, or empty when there is none. go vet
// gives no module for a package outside any, as the standard library, and
// nogo gives none at all. The path is empty outside module mode too.
func runModule(module *analysis.Module) string {
	if module == nil {
		return ""
	}
	return module.Path
}

// runNeedsProgram reports whether the package calls an invoker or a begin,
// or has a function with a parameter that has a method that ends a
// transaction. declares tells a function that a flag declares.
func runNeedsProgram(pass *analysis.Pass, declares func(directive.Kind, *types.Func) bool) bool {
	found := false
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				_, isInvoker := txcall.InvokerOf(pass.TypesInfo, n)
				found = found || isInvoker || txcall.IsBegin(pass.TypesInfo, n) || helper.Calls(pass, n) || runCallsDeclared(pass, n, declares)
			case *ast.FuncDecl:
				found = found || runTakesTx(pass.TypesInfo.Defs[n.Name].(*types.Func).Signature())
			case *ast.FuncLit:
				found = found || runTakesTx(pass.TypesInfo.TypeOf(n).(*types.Signature))
			}
			return !found
		})
	}
	return found
}

// runCallsDeclared reports whether call calls a function declared as an
// invoker or a begin.
func runCallsDeclared(pass *analysis.Pass, call *ast.CallExpr, declares func(directive.Kind, *types.Func) bool) bool {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	return ok && (declares(directive.Invokes, fn) || declares(directive.Begins, fn))
}

// runTakesTx reports whether a function of signature sig takes a value that
// has a method that ends a transaction, as its receiver or a parameter.
func runTakesTx(sig *types.Signature) bool {
	if sig.Recv() != nil && txcall.Holds(sig.Recv().Type()) {
		return true
	}
	for p := range sig.Params().Variables() {
		if txcall.Holds(p.Type()) {
			return true
		}
	}
	return false
}

// runFile runs the rules over the declarations of f: its functions, and
// the package-level declarations, whose code the package initializer runs.
func (c *checker) runFile(f *ast.File) {
	for _, decl := range f.Decls {
		outer := c.prog.Init()
		if fd, ok := decl.(*ast.FuncDecl); ok {
			if fd.Body == nil {
				continue
			}
			outer = c.prog.Func(fd)
		}
		ast.PreorderStack(decl, nil, func(n ast.Node, stack []ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				c.checkOuterHandleBegins(outer, n.Body)
			case *ast.FuncLit:
				// A literal that is never made never runs, and neither does
				// anything inside it.
				if _, made := program.Creation(c.prog.Func(n)); !made {
					return false
				}
				c.checkOuterHandleBegins(c.prog.Func(n), n.Body)
			case *ast.RangeStmt:
				// The body of a range over a function is a function of its
				// own.
				if body := c.prog.Func(n); body != nil {
					c.checkOuterHandleBegins(body, n.Body)
				}
			case *ast.CallExpr:
				c.runCall(n, c.prog.Enclosing(outer, stack, n))
			}
			return true
		})
	}
}

// runCall runs the rule of the invoker that call is, in fn.
func (c *checker) runCall(call *ast.CallExpr, fn *ssa.Function) {
	o, ok := c.helpers.InvokerOf(call)
	switch {
	case !ok:
	case o.InContext:
		c.checkOuterContext(o, fn)
	default:
		c.checkOuterHandle(o, fn)
	}
}
