package funcname

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		flag    string
		want    []string
		wantErr bool
	}{
		{flag: "", want: nil},
		{flag: "example.com/tele.Send", want: []string{"example.com/tele.Send"}},
		{flag: " a.F , (b.T).M ", want: []string{"(b.T).M", "a.F"}},
		{flag: "(*example.com/tele.Client).Capture", want: []string{"(example.com/tele.Client).Capture"}},
		{flag: "garbage((", wantErr: true},
		{flag: "a.F,noDot", wantErr: true},
		{flag: "(a.T)", wantErr: true},
		{flag: "(a.T).", wantErr: true},
		{flag: "a.F()", wantErr: true},
		{flag: "(m.L[T]).Report", want: []string{"(m.L).Report"}},
		{flag: "(*m.L[K, V]).Report", want: []string{"(m.L).Report"}},
		{flag: "(m.L[K,V]).Report,(m.L).Other", want: []string{"(m.L).Other", "(m.L).Report"}},
		{flag: "(m.L[T).Report", wantErr: true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.flag)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error = %v, want error %v", tt.flag, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, tt.want) {
			t.Errorf("Parse(%q) = %v, want %v", tt.flag, keys, tt.want)
		}
	}
}

func TestParseEach(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		want    []string
		wantErr bool
	}{
		{name: "none", names: nil, want: nil},
		{name: "one per item", names: []string{"a.F", " (*b.T).M "}, want: []string{"(b.T).M", "a.F"}},
		{name: "a comma inside type arguments", names: []string{"(m.L[K, V]).Report"}, want: []string{"(m.L).Report"}},
		{name: "two names in one item", names: []string{"a.F,b.G"}, wantErr: true},
		{name: "a misspelled name", names: []string{"garbage(("}, wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseEach(tt.names)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: error = %v, want error %v", tt.name, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, keys, tt.want)
		}
	}
}

// TestKeyGeneric names a method of a generic type whatever names its
// receiver gives the type parameters, so that a flag spelled from the type
// declaration matches.
func TestKeyGeneric(t *testing.T) {
	const src = `package m
type L[K comparable, V any] struct{}
func (l *L[A, B]) Report() {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "m.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("m", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(pkg.Scope().Lookup("L").Type()), true, pkg, "Report")
	names, err := Parse("(*m.L[K, V]).Report")
	if err != nil {
		t.Fatal(err)
	}
	if key := Key(obj.(*types.Func)); !names[key] {
		t.Errorf("Key = %q, not in %v", key, names)
	}
}
