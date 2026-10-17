// Package closer tells which instructions may end a transaction.
//
// A transaction ends when a method named Commit or Rollback is called on
// it, or on a field of it, as pop's tx.TX.Commit(). It may also end inside
// a function it is given. A function of this package is read to see
// whether it ends the parameter. A function of another package says so in
// a fact, which the analysis of its package exports. A function literal
// that is called is read as a function whose free variables are what it
// binds.
//
// In the function that holds the begin, a transaction that goes where this
// analysis does not follow, into a field, a map, a channel, or a function
// literal that is given away, may be ended by code it does not read. That counts as
// an end too, so that nothing is reported after it. Inside a helper, only
// an end the helper writes counts: a library's own methods store their
// receiver in many places, and would otherwise seem to end it. A
// transaction given to a function value or an interface method, whose body
// is not known, is taken to stay open.
package closer

import (
	"fmt"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/program"
)

// Fact says which parameters a function ends, before it returns or by
// letting them go. The receiver of a method is parameter 0.
type Fact struct {
	Params []int
}

// AFact marks Fact as an analysis fact.
func (*Fact) AFact() {}

func (f *Fact) String() string { return fmt.Sprintf("closes%v", f.Params) }

// Closer answers whether instructions end a transaction, for one pass.
type Closer struct {
	pass *analysis.Pass
	// known holds, for each function and root that has been asked about,
	// whether the function ends the transaction rooted at that parameter or
	// free variable. Every answer starts false and only turns true, in a
	// least fixpoint over all of them.
	known map[closerKey]bool
	// order lists the keys of known in the order they were asked about, so
	// that the fixpoint reads them the same way each time.
	order []closerKey
	// solving reports that the fixpoint is being computed. A key asked about
	// then answers what is known so far, and is read in the same pass.
	solving bool
}

// closerKey is a function, and the parameter or free variable of it that
// holds a transaction.
type closerKey struct {
	fn   *ssa.Function
	root ssa.Value
}

// New is a Closer for pass.
func New(pass *analysis.Pass) *Closer {
	return &Closer{pass: pass, known: make(map[closerKey]bool)}
}

// Ends reports whether instr may end the transaction tx, which is any of
// the handles in txs. A deferred call does not end it where it is written:
// it runs when the function returns.
func (c *Closer) Ends(instr ssa.Instruction, txs []handle.Path) bool {
	if _, ok := instr.(*ssa.Defer); ok {
		return false
	}
	if call, ok := instr.(ssa.CallInstruction); ok {
		return c.callEnds(call.Common(), txs)
	}
	return c.escapes(instr, txs)
}

// escapes reports whether instr gives the transaction where this analysis
// does not follow it.
func (c *Closer) escapes(instr ssa.Instruction, txs []handle.Path) bool {
	switch instr := instr.(type) {
	case *ssa.Store:
		// A store into a variable of the function moves nothing out of
		// reach. Any other store gives the transaction away.
		_, local := instr.Addr.(*ssa.Alloc)
		return !local && within(instr.Val, txs)
	case *ssa.Send:
		return within(instr.X, txs)
	case *ssa.MapUpdate:
		return within(instr.Value, txs)
	case *ssa.MakeClosure:
		return c.closureGivenAway(instr, txs)
	}
	return false
}

// callEnds reports whether a call may end the transaction.
func (c *Closer) callEnds(call *ssa.CallCommon, txs []handle.Path) bool {
	if recv, name, ok := receiver(call); ok && opener.Ending(name) && !takesFunc(call.Signature()) && within(recv, txs) {
		return true
	}
	fn := call.StaticCallee()
	if fn == nil {
		return false
	}
	for i, arg := range call.Args {
		if within(arg, txs) && opener.Holds(arg.Type()) && c.paramEnds(fn, i) {
			return true
		}
	}
	if mc, ok := call.Value.(*ssa.MakeClosure); ok {
		for i, b := range mc.Bindings {
			if within(b, txs) && c.rootEnds(fn, fn.FreeVars[i]) {
				return true
			}
		}
	}
	return false
}

// takesFunc reports whether sig takes a function. pop's Rollback(fn) runs fn
// in a transaction of its own: it ends nothing it is called on.
func takesFunc(sig *types.Signature) bool {
	for p := range sig.Params().Variables() {
		if _, ok := p.Type().Underlying().(*types.Signature); ok {
			return true
		}
	}
	return false
}

// receiver is the receiver of a method call, and the method's name.
func receiver(call *ssa.CallCommon) (ssa.Value, string, bool) {
	if call.IsInvoke() {
		return call.Value, call.Method.Name(), true
	}
	if fn := call.StaticCallee(); fn != nil && fn.Signature.Recv() != nil {
		return call.Args[0], fn.Name(), true
	}
	return nil, "", false
}

// paramEnds reports whether fn ends its parameter i. A function of this
// package is read, through its generic origin for an instance. One of
// another package says so in its fact.
func (c *Closer) paramEnds(fn *ssa.Function, i int) bool {
	if origin := fn.Origin(); origin != nil {
		fn = origin
	}
	if fn.Blocks != nil {
		return c.rootEnds(fn, fn.Params[i])
	}
	obj, ok := fn.Object().(*types.Func)
	var f Fact
	return ok && c.pass.ImportObjectFact(obj, &f) && slices.Contains(f.Params, i)
}

// rootEnds reports whether fn ends the transaction rooted at root, one of
// its parameters or free variables. Its deferred calls count: they run
// before it returns.
//
// Helpers may call each other, so the answers are a least fixpoint. Every
// key starts false. Each pass reads every key still false, and a pass that
// turns none true ends it. An answer never turns back, so there are at most
// as many passes as keys, and a ring of helpers that never end the
// transaction takes one pass, whichever is read first.
func (c *Closer) rootEnds(fn *ssa.Function, root ssa.Value) bool {
	key := closerKey{fn, root}
	if done, ok := c.known[key]; ok {
		return done
	}
	c.known[key] = false
	c.order = append(c.order, key)
	if !c.solving {
		c.solve()
	}
	return c.known[key]
}

// solve computes the fixpoint over every key known so far, and over the keys
// the reads ask about.
func (c *Closer) solve() {
	c.solving = true
	for changed := true; changed; {
		changed = false
		for i := 0; i < len(c.order); i++ {
			key := c.order[i]
			if !c.known[key] && c.rootEndsRead(key.fn, key.root) {
				c.known[key] = true
				changed = true
			}
		}
	}
	c.solving = false
}

// rootEndsRead reads fn for a call that ends the transaction rooted at root.
func (c *Closer) rootEndsRead(fn *ssa.Function, root ssa.Value) bool {
	txs := []handle.Path{handle.Of(root)}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if call, ok := instr.(ssa.CallInstruction); ok && c.callEnds(call.Common(), txs) {
				return true
			}
		}
	}
	return false
}

// closureGivenAway reports whether mc makes a literal that ends the
// transaction, and gives it to something other than a call of it or a
// defer. Then it may run anywhere. A closure that nothing uses never runs,
// so it ends nothing.
func (c *Closer) closureGivenAway(mc *ssa.MakeClosure, txs []handle.Path) bool {
	if !program.Used(mc) {
		return false
	}
	fn := mc.Fn.(*ssa.Function)
	ends := false
	for i, b := range mc.Bindings {
		if within(b, txs) && c.rootEnds(fn, fn.FreeVars[i]) {
			ends = true
		}
	}
	if !ends {
		return false
	}
	for _, ref := range *mc.Referrers() {
		switch ref := ref.(type) {
		case *ssa.DebugRef:
		case ssa.CallInstruction:
			if ref.Common().Value != mc {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// within reports whether v is stored where one of txs is, or in a field of
// one. A transaction kept in a variable assigned twice counts: a commit on
// a load of it may end the one this region began.
func within(v ssa.Value, txs []handle.Path) bool {
	p := handle.Of(v)
	return slices.ContainsFunc(txs, p.Stored)
}

// Export exports the fact of each function of the package that ends a
// parameter.
func (c *Closer) Export(prog *program.Program) {
	for _, fn := range prog.Funcs() {
		obj, ok := fn.Object().(*types.Func)
		if !ok {
			continue
		}
		var params []int
		for i, p := range fn.Params {
			if opener.Holds(p.Type()) && c.rootEnds(fn, p) {
				params = append(params, i)
			}
		}
		if len(params) > 0 {
			c.pass.ExportObjectFact(obj, &Fact{Params: params})
		}
	}
}
