package handle

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// TestFreeVarOfNeverMadeLiteral reads the free variable of a literal that is
// never made: assigned to the blank identifier, it is bound by nothing. The
// rules skip such a literal, so they never ask, but the answer must still be
// safe: the free variable is a root of its own.
func TestFreeVarOfNeverMadeLiteral(t *testing.T) {
	const src = `package p

func f(x *int) {
	_ = func() { _ = x }
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssautil.BuildPackage(&types.Config{Importer: importer.Default()}, fset,
		types.NewPackage("p", "p"), []*ast.File{file}, ssa.GlobalDebug)
	if err != nil {
		t.Fatal(err)
	}
	f := pkg.Func("f")
	fv := f.AnonFuncs[0].FreeVars[0]
	ps := NewPaths("")
	if !ps.Of(fv).Equal(ps.Of(fv)) {
		t.Error("the free variable is not the same handle as itself")
	}
	if ps.Of(fv).Equal(ps.Of(f.Params[0])) {
		t.Error("the free variable of a literal that is never made is bound to the parameter")
	}
}
