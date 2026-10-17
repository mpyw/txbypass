package program

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// TestCore reads the core type of each type parameter of f.
func TestCore(t *testing.T) {
	const src = `package p

type Row struct{ ID int }

type Ints []int

func f[
	Plain Row,
	One ~struct{ ID int },
	Same Ints | []int,
	Mixed ~[]int | ~[]string,
	Any any,
	Embedded interface{ comparable; ~[]int },
]() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("p", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tparams := pkg.Scope().Lookup("f").Type().(*types.Signature).TypeParams()
	core := func(i int) types.Type { return Core(tparams.At(i)) }
	if _, ok := core(1).(*types.Struct); !ok {
		t.Errorf("Core(One) = %v, want a struct", core(1))
	}
	if s, ok := core(2).(*types.Slice); !ok || !types.Identical(s.Elem(), types.Typ[types.Int]) {
		t.Errorf("Core(Same) = %v, want []int", core(2))
	}
	for _, i := range []int{3, 4} {
		if _, ok := core(i).(*types.Interface); !ok {
			t.Errorf("Core(%s) = %v, want its constraint", tparams.At(i), core(i))
		}
	}
	if s, ok := core(5).(*types.Slice); !ok || !types.Identical(s.Elem(), types.Typ[types.Int]) {
		t.Errorf("Core(Embedded) = %v, want []int", core(5))
	}
	row := pkg.Scope().Lookup("Row").Type()
	if _, ok := Core(row).(*types.Struct); !ok {
		t.Errorf("Core(Row) = %v, want its struct", Core(row))
	}
	if _, ok := core(0).(*types.Struct); !ok {
		t.Errorf("Core(Plain) = %v, want Row's struct", core(0))
	}
}
