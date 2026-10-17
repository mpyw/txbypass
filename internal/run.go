// Package internal implements the txbypass rules.
//
// Each rule has its own file. They share one pass's state, in checker.go:
// the directives, and the files that are generated. They read the code
// inside a transaction through scope.go. Which calls open a transaction,
// where a begun one is open, and which expressions name the same handle,
// live in the packages below this one.
package internal

import (
	"errors"
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/mpyw/txbypass/internal/opener"
)

// errRunWithoutInspector is returned by Run when the pass carries no
// inspector, which means the analyzer was registered without its
// requirement.
var errRunWithoutInspector = errors.New("txbypass: inspect result missing")

// Run analyzes one package.
func Run(pass *analysis.Pass, cfg Config) (any, error) {
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errRunWithoutInspector
	}
	c := newChecker(pass, cfg)
	for _, p := range c.directives.Problems() {
		c.reportf(p.Pos, p.Message)
	}
	for n := range insp.PreorderSeq((*ast.CallExpr)(nil), (*ast.FuncDecl)(nil), (*ast.FuncLit)(nil)) {
		// Nothing is reported in a generated file, so it is not analyzed.
		if c.inGenerated(n.Pos()) {
			continue
		}
		c.runNode(n)
	}
	// Every call the pass reports on is seen in full, in a package's test
	// variant as in the ordinary one, so an ignore that silenced nothing
	// here silences nothing anywhere.
	for _, pos := range c.directives.Unused(cfg.On) {
		c.reportf(pos, "unused txbypass:ignore directive")
	}
	c.flush()
	return nil, nil
}

// runNode runs the rules that n starts: a callback opener's, or those of the
// begins in a function's body.
func (c *checker) runNode(n ast.Node) {
	switch n := n.(type) {
	case *ast.CallExpr:
		o, ok := opener.Of(c.pass.TypesInfo, n)
		switch {
		case !ok:
		case o.InContext:
			c.checkOuterContext(o)
		default:
			c.checkOuterHandle(o)
		}
	case *ast.FuncDecl:
		if n.Body != nil {
			c.checkOuterHandleBegins(n.Body)
		}
	case *ast.FuncLit:
		c.checkOuterHandleBegins(n.Body)
	}
}
