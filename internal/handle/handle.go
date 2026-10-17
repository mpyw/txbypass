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
	// unknown reports that base is a variable in memory written where this
	// analysis does not follow: by a function literal that captures it, or
	// through an address that goes elsewhere. Which handle it holds is not
	// known, so it is the same handle as nothing. It is still the same
	// storage as itself: see Stored.
	unknown bool
}

// Paths answers Of for one pass, and remembers what it reads. The SSA form
// does not change during a pass, so neither do the answers. A rule asks
// about every use in a scope, and a region about every instruction in it.
// A variable in memory is read through every use of it, so without the
// memory the time grew with the cube of a region's length.
type Paths struct {
	known  map[ssa.Value]Path
	stores map[*ssa.Alloc]allocStoreSet
	// module is the path of the module of the package read, or empty
	// outside module mode.
	module string
}

// NewPaths is a Paths that has read nothing yet, for a package of module,
// which is empty outside module mode.
func NewPaths(module string) *Paths {
	return &Paths{known: make(map[ssa.Value]Path), stores: make(map[*ssa.Alloc]allocStoreSet), module: module}
}

// Of is the handle v is.
func (ps *Paths) Of(v ssa.Value) Path {
	if p, ok := ps.known[v]; ok {
		return p
	}
	p := ps.of(v, &walk{on: make(map[ssa.Value]bool), phis: make(map[*ssa.Phi]Path)})
	ps.known[v] = p
	return p
}

// walk is what one Of has read. on holds the phis and variables still on
// the way, so that a loop of them ends. phis holds the phis already read: a
// phi met again through another path of the blocks gives the same handle,
// and reading it again would take time exponential in the branches.
type walk struct {
	on   map[ssa.Value]bool
	phis map[*ssa.Phi]Path
}

// of is Of, with what the walk has read. A variable stored a call on itself, as q = q.Where(c), leads
// back to itself.
func (ps *Paths) of(v ssa.Value, w *walk) Path {
	switch v := v.(type) {
	case *ssa.UnOp:
		// A load gives what its address holds. Any other operator gives a
		// value of its own.
		if v.Op == token.MUL {
			return ps.of(v.X, w)
		}
	case *ssa.FieldAddr:
		return ps.of(v.X, w).with(fieldOf(v.X.Type(), v.Field))
	case *ssa.Field:
		return ps.of(v.X, w).with(fieldOf(v.X.Type(), v.Field))
	case *ssa.ChangeType:
		return ps.of(v.X, w)
	case *ssa.MakeInterface:
		return ps.of(v.X, w)
	case *ssa.ChangeInterface:
		return ps.of(v.X, w)
	case *ssa.TypeAssert:
		// Only an assertion that panics when it fails gives the value
		// itself. The comma-ok form gives a tuple.
		if !v.CommaOk {
			return ps.of(v.X, w)
		}
	case *ssa.MakeClosure:
		// A method value, as db.Create, binds its receiver.
		if strings.HasPrefix(v.Fn.(*ssa.Function).Synthetic, "bound method wrapper") {
			return ps.of(v.Bindings[0], w)
		}
	case *ssa.FreeVar:
		if b, ok := binding(v); ok {
			return ps.of(b, w)
		}
	case *ssa.Alloc:
		// A variable written once as a whole is the value written. One
		// written more than once, or through a field, all in its own
		// function, is itself: a callback runs while it holds one value, and
		// a scope checks the writes inside it (see Address). One written
		// elsewhere is not known.
		whole, fields, ok := ps.allocStores(v)
		switch {
		case !ok:
			return Path{base: v, unknown: true}
		case len(whole) == 1 && !fields && !w.on[v]:
			w.on[v] = true
			defer delete(w.on, v)
			return ps.of(whole[0].Val, w)
		}
	case *ssa.Call:
		if recv, ok := returnedReceiver(v); ok {
			return ps.of(recv, w)
		}
	case *ssa.Phi:
		if p, ok := w.phis[v]; ok {
			return p
		}
		if w.on[v] {
			return Path{base: v}
		}
		w.on[v] = true
		p, ok := ps.samePhi(v, w)
		delete(w.on, v)
		if !ok {
			p = Path{base: v}
		}
		w.phis[v] = p
		return p
	}
	return Path{base: v}
}

// with extends p with f. A handle that is not known stays not known.
func (p Path) with(f *types.Var) Path {
	return Path{base: p.base, fields: append(slices.Clip(p.fields), f), unknown: p.unknown}
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
		return nil, false
	}
	return made.(*ssa.MakeClosure).Bindings[slices.Index(fn.FreeVars, v)], true
}

// allocStores lists the stores that write a variable in memory as a whole,
// and reports whether any store writes a field of it. ok is false when
// something this analysis does not follow may write it: a function literal
// that captures it, or its address, or a field's, going anywhere but to a
// load, a store to it, a debug reference, a field, a capture, or a call of a
// library, as libraryCall says. Such a call takes it as the receiver of a
// method of a value, as db.Begin() does on a db held by value.
func (ps *Paths) allocStores(a *ssa.Alloc) (whole []*ssa.Store, fields, ok bool) {
	set, read := ps.stores[a]
	if !read {
		set.module = ps.module
		set.ok = set.collect(a, false)
		for _, st := range append(slices.Clip(set.whole), set.fields...) {
			if st.Parent() != a.Parent() {
				set.ok = false
			}
		}
		set.visited = nil
		ps.stores[a] = set
	}
	if !set.ok {
		return nil, false, false
	}
	return set.whole, len(set.fields) > 0, true
}

// allocStoreSet is what allocStores finds.
type allocStoreSet struct {
	whole  []*ssa.Store
	fields []*ssa.Store
	// ok reports that the address goes only where allocStores allows.
	ok bool
	// module is Paths.module.
	module string
	// visited holds the variables whose loads are already walked. A pointer
	// copied back and forth between two variables leads back to one.
	visited map[allocStep]bool
}

// allocStep is a variable that holds the address, as collectLoads walks it.
type allocStep struct {
	v     ssa.Value
	field bool
}

// visit reports whether step is new, and marks it walked. Each address is
// reached once: a load, a field, or a free variable has one way in.
func (set *allocStoreSet) visit(step allocStep) bool {
	if set.visited == nil {
		set.visited = make(map[allocStep]bool)
	}
	if set.visited[step] {
		return false
	}
	set.visited[step] = true
	return true
}

// collect adds the stores to the address v to the set, as whole stores or,
// when field is set, as stores to a field. It reports whether the address
// goes only where allocStores allows.
func (set *allocStoreSet) collect(v ssa.Value, field bool) bool {
	for _, ref := range *v.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp, *ssa.DebugRef:
		case ssa.CallInstruction:
			if !libraryCall(ref.Common(), v, set.module) {
				return false
			}
		case *ssa.FieldAddr:
			if !set.collect(ref, true) {
				return false
			}
		case *ssa.Store:
			switch {
			case ref.Addr != v:
				// The address kept in a variable of the function, as the
				// pointer r := &Repo{db: db} a literal captures, is still
				// this address wherever the variable is loaded.
				local, ok := ref.Addr.(*ssa.Alloc)
				if !ok || !set.collectLoads(local, field) {
					return false
				}
			case field:
				set.fields = append(set.fields, ref)
			default:
				set.whole = append(set.whole, ref)
			}
		case *ssa.MakeClosure:
			fv := ref.Fn.(*ssa.Function).FreeVars[slices.Index(ref.Bindings, v)]
			if !set.collect(fv, field) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// libraryCall reports whether call, which takes the address v, is of a
// library: a function of another module, as db.Begin() on a db held by
// value is. A function of the module, as a repository's Bind(tx), may store
// another handle there, in the package or in another. So may a function
// value. A library's own method does not rebind a field of the caller's
// struct. Outside module mode, module is empty, and another package is a
// library.
func libraryCall(call *ssa.CallCommon, v ssa.Value, module string) bool {
	fn := call.StaticCallee()
	if fn == nil || fn.Pkg == nil {
		return false
	}
	path := fn.Pkg.Pkg.Path()
	if module != "" {
		return path != module && !strings.HasPrefix(path, module+"/")
	}
	return fn.Pkg != v.Parent().Pkg
}

// collectLoads collects, as collect does, through every load of the
// variable in memory v that holds the address. Another store to v changes
// what v holds, not the address. A literal that captures v loads it
// through its free variable.
func (set *allocStoreSet) collectLoads(v ssa.Value, field bool) bool {
	if !set.visit(allocStep{v: v, field: field}) {
		return true
	}
	for _, ref := range *v.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp:
			if !set.collect(ref, field) {
				return false
			}
		case *ssa.Store, *ssa.DebugRef:
		case *ssa.MakeClosure:
			fv := ref.Fn.(*ssa.Function).FreeVars[slices.Index(ref.Bindings, v)]
			if !set.collectLoads(fv, field) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Address is the storage that a store to addr writes: a variable, or a field
// of a handle.
func (ps *Paths) Address(addr ssa.Value) Path {
	switch a := addr.(type) {
	case *ssa.Alloc:
		return Path{base: a}
	case *ssa.FreeVar:
		if b, ok := binding(a); ok {
			return ps.Address(b)
		}
	}
	return ps.Of(addr)
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
	return recv, same && fn.Name() != "WithTx" && !opener.IsBeginFunc(fn) && !binds(fn, recv.Type())
}

// binds reports whether fn takes a value of its receiver's type, or one that
// can end a transaction. Such a method binds a new value to what it is
// given, as repo.Using(tx) does, whatever its name.
func binds(fn *types.Func, recv types.Type) bool {
	for p := range fn.Signature().Params().Variables() {
		if types.Identical(p.Type(), recv) || opener.Holds(p.Type()) {
			return true
		}
	}
	return false
}

// samePhi is the handle every edge of a phi gives, when they all give one.
// An edge that comes back to the phi itself, around a loop, gives nothing
// new.
func (ps *Paths) samePhi(v *ssa.Phi, w *walk) (Path, bool) {
	var p Path
	found := false
	for _, e := range v.Edges {
		q := ps.of(e, w)
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
// same fields. A handle that is not known is the same as nothing.
func (p Path) Equal(q Path) bool {
	return !p.unknown && !q.unknown && p.base == q.base && slices.Equal(p.fields, q.fields)
}

// Within reports whether p is q, or a field selected from q. Reading
// db.Error reads the handle db. A value that embeds q is not within it: it
// may hold other things. A handle that is not known is within nothing.
func (p Path) Within(q Path) bool {
	return !p.unknown && !q.unknown && p.Stored(q)
}

// Stored reports whether p is stored where q is, or in a field of it, known
// or not. A transaction kept in a variable assigned twice is still that
// variable: a commit through a load of it ends what it holds. A method of
// tx.TX is a method of the transaction tx.
func (p Path) Stored(q Path) bool {
	return p.base == q.base && len(p.fields) >= len(q.fields) && slices.Equal(p.fields[:len(q.fields)], q.fields)
}

// Depth is the number of fields p selects from its root.
func (p Path) Depth() int {
	return len(p.fields)
}
