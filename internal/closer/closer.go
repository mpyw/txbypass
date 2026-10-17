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
// analysis does not follow, into a field, a channel, or a function literal
// that is given away, may be ended by code it does not read. That counts as
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
	"github.com/mpyw/txbypass/internal/program"
)

// methods are the names of the methods that end a transaction. go-pg's take
// a context as CommitContext and RollbackContext.
var methods = map[string]bool{"Commit": true, "Rollback": true, "CommitContext": true, "RollbackContext": true}

// Holds reports whether a value of type t has a method that ends a
// transaction, directly or through a pointer to it.
func Holds(t types.Type) bool {
	for _, mt := range []types.Type{t, types.NewPointer(t)} {
		ms := types.NewMethodSet(mt)
		for i := range ms.Len() {
			if methods[ms.At(i).Obj().Name()] {
				return true
			}
		}
	}
	return false
}

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
	// memo holds what is known of each function and root: whether the
	// function ends the transaction rooted at that parameter or free
	// variable. A pair still being read counts as false, which ends a
	// recursion.
	memo map[closerKey]bool
}

// closerKey is a function, and the parameter or free variable of it that
// holds a transaction.
type closerKey struct {
	fn   *ssa.Function
	root ssa.Value
}

// New is a Closer for pass.
func New(pass *analysis.Pass) *Closer {
	return &Closer{pass: pass, memo: make(map[closerKey]bool)}
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
	case *ssa.MakeClosure:
		return c.closureGivenAway(instr, txs)
	}
	return false
}

// callEnds reports whether a call may end the transaction.
func (c *Closer) callEnds(call *ssa.CallCommon, txs []handle.Path) bool {
	if recv, name, ok := receiver(call); ok && methods[name] && !takesFunc(call.Signature()) && within(recv, txs) {
		return true
	}
	fn := call.StaticCallee()
	if fn == nil {
		return false
	}
	for i, arg := range call.Args {
		if within(arg, txs) && Holds(arg.Type()) && c.paramEnds(fn, i) {
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
func (c *Closer) rootEnds(fn *ssa.Function, root ssa.Value) bool {
	key := closerKey{fn, root}
	if done, ok := c.memo[key]; ok {
		return done
	}
	c.memo[key] = false
	txs := []handle.Path{handle.Of(root)}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if call, ok := instr.(ssa.CallInstruction); ok && c.callEnds(call.Common(), txs) {
				c.memo[key] = true
				return true
			}
		}
	}
	return false
}

// closureGivenAway reports whether mc makes a literal that ends the
// transaction, and gives it to something other than a call of it or a
// defer. Then it may run anywhere.
func (c *Closer) closureGivenAway(mc *ssa.MakeClosure, txs []handle.Path) bool {
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
			if Holds(p.Type()) && c.rootEnds(fn, p) {
				params = append(params, i)
			}
		}
		if len(params) > 0 {
			c.pass.ExportObjectFact(obj, &Fact{Params: params})
		}
	}
}
