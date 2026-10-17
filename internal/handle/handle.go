// Package handle tells whether two expressions name the same handle.
//
// A handle is named by a variable and the fields selected from it: db, r.db,
// or s.repo.db. Two expressions name the same handle when they start from the
// same variable and select the same fields. A method that returns the type of
// its receiver, as gorm's WithContext does, gives the same handle back, so
// db.WithContext(ctx) names db. A value is the same handle as the fields it
// embeds: when s embeds *gorm.DB, s and s.DB are one handle.
//
// Only names are compared. A copy of the handle into another variable, as in
// d := db, names a different handle.
package handle

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
)

// Path is a handle: a variable, and the fields selected from it.
type Path struct {
	base types.Object
	// fields are the fields selected from base, in order, including the
	// embedded fields a promoted field or method passes through.
	fields []*types.Var
}

// Var is the handle v names by itself.
func Var(v types.Object) Path {
	return Path{base: v}
}

// Of reports the handle e names. It reports nothing for an expression that
// does not name one from a variable, such as the result of a call that
// returns something other than its receiver.
func Of(info *types.Info, e ast.Expr) (Path, bool) {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		v, ok := info.ObjectOf(e).(*types.Var)
		// A key of a composite literal resolves to its field, which names
		// no handle without the value it is selected from.
		if !ok || v.IsField() {
			return Path{}, false
		}
		return Path{base: v}, true
	case *ast.StarExpr:
		return Of(info, e.X)
	case *ast.CallExpr:
		return returnedReceiver(info, e)
	case *ast.SelectorExpr:
		return selected(info, e)
	}
	return Path{}, false
}

// selected reports the handle a selector names. A field extends the handle
// of the value it is selected from. A method value names the handle of its
// receiver, through the embedded fields its promotion passes. A qualified
// identifier names a package-level variable.
func selected(info *types.Info, e *ast.SelectorExpr) (Path, bool) {
	sel := info.Selections[e]
	if sel == nil {
		v, ok := info.Uses[e.Sel].(*types.Var)
		if !ok {
			return Path{}, false
		}
		return Path{base: v}, true
	}
	idx := sel.Index()
	switch sel.Kind() {
	case types.FieldVal:
	case types.MethodVal:
		idx = idx[:len(idx)-1]
	default:
		return Path{}, false
	}
	p, ok := Of(info, e.X)
	if !ok {
		return Path{}, false
	}
	return p.through(sel.Recv(), idx), true
}

// returnedReceiver reports the handle of the receiver of a method call that
// returns a value of the receiver's own type, as db.WithContext(ctx) or
// db.Session(cfg) do in gorm.
func returnedReceiver(info *types.Info, call *ast.CallExpr) (Path, bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return Path{}, false
	}
	s := info.Selections[sel]
	if s == nil || s.Kind() != types.MethodVal {
		return Path{}, false
	}
	if !types.Identical(info.TypeOf(call), info.TypeOf(sel.X)) {
		return Path{}, false
	}
	return Of(info, sel)
}

// through extends p with the fields at idx, read from t.
func (p Path) through(t types.Type, idx []int) Path {
	fields := append([]*types.Var(nil), p.fields...)
	for _, i := range idx {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		f := t.Underlying().(*types.Struct).Field(i)
		fields = append(fields, f)
		t = f.Type()
	}
	return Path{base: p.base, fields: fields}
}

// Equal reports whether p and q name the same handle. Embedded fields at the
// end of either are left out, since a value is the handle it embeds.
func (p Path) Equal(q Path) bool {
	pf, qf := p.named(), q.named()
	return p.base == q.base && slices.Equal(pf, qf)
}

// Within reports whether p is q, or a field selected from q, embedded or
// not. A method of tx.TX is a method of the transaction tx.
func (p Path) Within(q Path) bool {
	return p.base == q.base && len(p.fields) >= len(q.fields) && slices.Equal(p.fields[:len(q.fields)], q.fields)
}

// named is p's fields without the embedded fields at their end.
func (p Path) named() []*types.Var {
	n := len(p.fields)
	for n > 0 && p.fields[n-1].Embedded() {
		n--
	}
	return p.fields[:n]
}

// Spell spells the handle as source in package from would: the variable,
// then each field. A variable of another package is qualified by its
// package's name. Embedded fields at the end are left out, as Equal leaves
// them out.
func (p Path) Spell(from *types.Package) string {
	var b strings.Builder
	if pkg := p.base.Pkg(); pkg != from {
		b.WriteString(pkg.Name() + ".")
	}
	b.WriteString(p.base.Name())
	for _, f := range p.named() {
		b.WriteString("." + f.Name())
	}
	return b.String()
}
