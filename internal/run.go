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

	"github.com/mpyw/txbypass/internal/closer"
	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/program"
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
	if runNeedsProgram(pass) {
		c.prog = program.Build(pass, cfgs.NoReturn)
		c.paths = handle.NewPaths(runModule(pass.Module))
		c.closer = closer.New(pass, c.paths)
		c.closer.Export(c.prog)
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

// runModule is the path of module, or empty when there is none. go vet
// gives no module for a package outside any, as the standard library, and
// nogo gives none at all. The path is empty outside module mode too.
func runModule(module *analysis.Module) string {
	if module == nil {
		return ""
	}
	return module.Path
}

// runNeedsProgram reports whether the package calls an opener or a begin,
// or has a function with a parameter that has a method that ends a
// transaction.
func runNeedsProgram(pass *analysis.Pass) bool {
	found := false
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				_, isOpener := opener.Of(pass.TypesInfo, n)
				found = found || isOpener || opener.IsBegin(pass.TypesInfo, n)
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

// runTakesTx reports whether a function of signature sig takes a value that
// has a method that ends a transaction, as its receiver or a parameter.
func runTakesTx(sig *types.Signature) bool {
	if sig.Recv() != nil && opener.Holds(sig.Recv().Type()) {
		return true
	}
	for p := range sig.Params().Variables() {
		if opener.Holds(p.Type()) {
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

// runCall runs the rule of the callback opener that call is, in fn.
func (c *checker) runCall(call *ast.CallExpr, fn *ssa.Function) {
	o, ok := opener.Of(c.pass.TypesInfo, call)
	switch {
	case !ok:
	case o.InContext:
		c.checkOuterContext(o, fn)
	default:
		c.checkOuterHandle(o, fn)
	}
}
