// Package spell spells a handle for a message, as the source names it.
//
// The SSA form says which values are one handle, but not what the reader
// wrote. A message names the expression where the use is: d, when the code
// used a copy d of db. It follows the names that select a handle: a
// variable, its fields, conversions, and the methods that give their
// receiver back, as gorm's WithContext does, and the embedded fields a
// promoted method passes. What it does not follow, such as a type
// assertion, it spells as written.
package spell

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
)

// Of spells the handle e reaches, as source in package from would: what e
// begins with, then its first depth fields. A use that reads a field of the
// handle, as db.Error, is spelled as the handle, db. Embedded fields are left
// out, as the source leaves them out: s.Create(u) reaches s.DB, and is
// spelled s. db.writer reaches db.dbbase.writer, and is spelled db.writer. A
// variable of another package is qualified by its package's name.
func Of(info *types.Info, e ast.Expr, from *types.Package, depth int) string {
	head, steps := path(info, e, from)
	steps = steps[:min(depth, len(steps))]
	var b strings.Builder
	b.WriteString(head)
	for i, s := range steps {
		if s.field.Embedded() && promoted(steps[i:], from) {
			continue
		}
		b.WriteString("." + s.field.Name())
	}
	return b.String()
}

// promoted reports whether the embedded field steps[0] can be left out: no
// named field follows it, or the next one is promoted through it. A field
// of the outer struct of the same name shadows it, as o.Inner.db does when
// o has a db of its own, so o.db would name another field.
func promoted(steps []step, from *types.Package) bool {
	i := slices.IndexFunc(steps, func(s step) bool { return !s.field.Embedded() })
	if i < 0 {
		return true
	}
	obj, _, _ := types.LookupFieldOrMethod(steps[0].in, true, from, steps[i].field.Name())
	v, ok := obj.(*types.Var)
	return ok && v.Origin() == steps[i].field.Origin()
}

// step is one field a path selects, and the type it is selected from.
type step struct {
	field *types.Var
	in    types.Type
}

// path is how e begins, spelled, and the fields it selects after that.
func path(info *types.Info, e ast.Expr, from *types.Package) (string, []step) {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		// The SSA form gives a handle only to an identifier of a
		// variable.
		return qualified(info.ObjectOf(e), from), nil
	case *ast.StarExpr:
		return path(info, e.X, from)
	case *ast.UnaryExpr:
		// Only &x reaches here: a handle is a pointer or an interface, and
		// *&x is x.
		return path(info, e.X, from)
	case *ast.CallExpr:
		// A conversion, or new(db), gives what it is given, or a pointer
		// to a copy of it.
		if info.Types[e.Fun].IsType() || info.Types[e.Fun].IsBuiltin() {
			return path(info, e.Args[0], from)
		}
		// Any other call that gives a handle is a method that returns its
		// receiver's type. Its selector adds the embedded fields its
		// promotion passes, as s.Create(u) reaches s.DB.
		fun := ast.Unparen(e.Fun)
		// A generic method called with type arguments, as r.Scoped[User]().
		switch x := fun.(type) {
		case *ast.IndexExpr:
			fun = x.X
		case *ast.IndexListExpr:
			fun = x.X
		}
		sel, ok := ast.Unparen(fun).(*ast.SelectorExpr)
		if !ok {
			// Unreachable: a call of a function, as New(db), makes a handle
			// of its own, and the scope reads no such call as a use.
			return types.ExprString(e), nil
		}
		return selected(info, sel, from)
	case *ast.SelectorExpr:
		return selected(info, e, from)
	}
	return types.ExprString(e), nil
}

// selected is the path a selector names. A field extends the path of the
// value it is selected from. A method value adds the embedded fields its
// promotion passes. A qualified identifier names a package-level variable.
func selected(info *types.Info, e *ast.SelectorExpr, from *types.Package) (string, []step) {
	sel := info.Selections[e]
	if sel == nil {
		return qualified(info.Uses[e.Sel], from), nil
	}
	head, fields := path(info, e.X, from)
	idx := sel.Index()
	if sel.Kind() == types.MethodVal {
		idx = idx[:len(idx)-1]
	}
	t := sel.Recv()
	for _, i := range idx {
		in := t
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		f := t.Underlying().(*types.Struct).Field(i)
		fields = append(fields, step{field: f, in: in})
		t = f.Type()
	}
	return head, fields
}

// qualified is obj's name, qualified by its package's name when the package
// is not from.
func qualified(obj types.Object, from *types.Package) string {
	if obj.Pkg() != from {
		return obj.Pkg().Name() + "." + obj.Name()
	}
	return obj.Name()
}
