// Package handle tells whether two values are the same handle.
//
// A handle is a root value and the fields selected from it: db, r.db, or
// s.repo.db. The root is what the SSA form gives the value before any field
// is selected: a parameter, a package-level variable, the result of a call.
// Two values are one handle when they come from the same root through the
// same fields.
//
// The SSA form makes a copy the same value as what it copies, so d := db is
// one handle. A variable that a function literal captures lives in memory.
// When one store writes it, it is the value it holds. A method that returns
// the type of its receiver, as gorm's WithContext does, gives the same
// handle back. A struct that embeds a handle is not that handle: when s
// embeds *gorm.DB, s.Create(u) reaches s.DB, but s.validate(u) does not.
package handle

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/program"
)

// Path is a handle: a root value, and the fields selected from it.
type Path struct {
	base ssa.Value
	// fields are the fields selected from base, in order, including the
	// embedded fields a promoted field or method passes through.
	fields []*types.Var
}

// Of is the handle v is.
func Of(v ssa.Value) Path {
	return of(v, make(map[ssa.Value]bool))
}

// of is Of, with the phis already on the way, so that a loop of phis ends.
func of(v ssa.Value, seen map[ssa.Value]bool) Path {
	switch v := v.(type) {
	case *ssa.UnOp:
		// A load gives what its address holds. Any other operator gives a
		// value of its own.
		if v.Op == token.MUL {
			return of(v.X, seen)
		}
	case *ssa.FieldAddr:
		return of(v.X, seen).with(fieldOf(v.X.Type(), v.Field))
	case *ssa.Field:
		return of(v.X, seen).with(fieldOf(v.X.Type(), v.Field))
	case *ssa.ChangeType:
		return of(v.X, seen)
	case *ssa.MakeInterface:
		return of(v.X, seen)
	case *ssa.ChangeInterface:
		return of(v.X, seen)
	case *ssa.TypeAssert:
		// Only an assertion that panics when it fails gives the value
		// itself. The comma-ok form gives a tuple.
		if !v.CommaOk {
			return of(v.X, seen)
		}
	case *ssa.MakeClosure:
		// A method value, as db.Create, binds its receiver.
		if strings.HasPrefix(v.Fn.(*ssa.Function).Synthetic, "bound method wrapper") {
			return of(v.Bindings[0], seen)
		}
	case *ssa.FreeVar:
		if b, ok := binding(v); ok {
			return of(b, seen)
		}
	case *ssa.Alloc:
		if stored, ok := onlyStore(v); ok {
			return of(stored, seen)
		}
	case *ssa.Call:
		if recv, ok := returnedReceiver(v); ok {
			return of(recv, seen)
		}
	case *ssa.Phi:
		if seen[v] {
			return Path{base: v}
		}
		if p, ok := samePhi(v, seen); ok {
			return p
		}
	}
	return Path{base: v}
}

// with extends p with f.
func (p Path) with(f *types.Var) Path {
	return Path{base: p.base, fields: append(slices.Clip(p.fields), f)}
}

// With extends p with fields, as the embedded fields a promoted method
// passes through.
func (p Path) With(fields []*types.Var) Path {
	for _, f := range fields {
		p = p.with(f)
	}
	return p
}

// Promoted lists the embedded fields that a promoted method or field
// passes through, before the one it selects.
func Promoted(sel *types.Selection) []*types.Var {
	var fields []*types.Var
	t := sel.Recv()
	idx := sel.Index()
	for _, i := range idx[:len(idx)-1] {
		f := fieldOf(t, i)
		fields = append(fields, f)
		t = f.Type()
	}
	return fields
}

// fieldOf is field i of the struct that t is, or points to.
func fieldOf(t types.Type, i int) *types.Var {
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}
	return t.Underlying().(*types.Struct).Field(i)
}

// binding is the value that the closure making v's function binds v to. A
// literal with free variables is made by a closure, when it is made at all.
// A synthetic function, such as the wrapper of a bound method value, has no
// parent to read, and a literal that is never made binds nothing. Their
// free variables are roots of their own.
func binding(v *ssa.FreeVar) (ssa.Value, bool) {
	fn := v.Parent()
	if fn.Parent() == nil {
		return nil, false
	}
	made, ok := program.Creation(fn)
	if !ok {
		// Unreachable: only a literal that is made is read. walkScope skips
		// the others, and closer reads the literals that a live call or
		// closure names.
		return nil, false
	}
	return made.(*ssa.MakeClosure).Bindings[slices.Index(fn.FreeVars, v)], true
}

// onlyStore is the value a variable in memory holds, when one store writes
// it, in its own function or in a literal that captures it. Its address
// must go nowhere else, or something else may write it.
func onlyStore(a *ssa.Alloc) (ssa.Value, bool) {
	var stores []*ssa.Store
	if !collectStores(a, &stores) || len(stores) != 1 {
		return nil, false
	}
	return stores[0].Val, true
}

// collectStores adds the stores to the address v to stores. It reports
// false when the address is used for anything but a load, a store to it, a
// debug reference, or a capture.
func collectStores(v ssa.Value, stores *[]*ssa.Store) bool {
	for _, ref := range *v.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp:
		case *ssa.DebugRef:
		case *ssa.Store:
			if ref.Addr != v {
				return false
			}
			*stores = append(*stores, ref)
		case *ssa.MakeClosure:
			fv := ref.Fn.(*ssa.Function).FreeVars[slices.Index(ref.Bindings, v)]
			if !collectStores(fv, stores) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// returnedReceiver is the receiver of a method call that returns a value of
// the receiver's own type, as db.WithContext(ctx) or db.Session(cfg) do in
// gorm. Two kinds of method give another handle of that type: a begin, as
// gorm's db.Begin(), gives the transaction, and sqlc's q.WithTx(tx) gives
// queries bound to it.
func returnedReceiver(call *ssa.Call) (ssa.Value, bool) {
	c := call.Common()
	var recv ssa.Value
	var fn *types.Func
	switch {
	case c.IsInvoke():
		recv, fn = c.Value, c.Method
	case c.Signature().Recv() != nil:
		recv, fn = c.Args[0], c.StaticCallee().Object().(*types.Func)
	default:
		return nil, false
	}
	same := types.Identical(call.Type(), recv.Type())
	return recv, same && fn.Name() != "WithTx" && !opener.IsBeginFunc(fn)
}

// samePhi is the handle every edge of a phi gives, when they all give one.
// An edge that comes back to the phi itself, around a loop, gives nothing
// new.
func samePhi(v *ssa.Phi, seen map[ssa.Value]bool) (Path, bool) {
	seen[v] = true
	var p Path
	found := false
	for _, e := range v.Edges {
		q := of(e, seen)
		if q.base == v {
			continue
		}
		if found && !q.Equal(p) {
			return Path{}, false
		}
		p, found = q, true
	}
	return p, found
}

// Equal reports whether p and q are the same handle: the same root, and the
// same fields.
func (p Path) Equal(q Path) bool {
	return p.base == q.base && slices.Equal(p.fields, q.fields)
}

// Within reports whether p is q, or a field selected from q. A method of
// tx.TX is a method of the transaction tx, and reading db.Error reads the
// handle db. A value that embeds q is not within it: it may hold other
// things.
func (p Path) Within(q Path) bool {
	return p.base == q.base && len(p.fields) >= len(q.fields) && slices.Equal(p.fields[:len(q.fields)], q.fields)
}

// Depth is the number of fields p selects from its root.
func (p Path) Depth() int {
	return len(p.fields)
}
