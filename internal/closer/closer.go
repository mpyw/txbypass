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
// analysis does not follow, into a field, a map as a key or a value, a
// channel, even in a select, or a function literal that is given away, may be ended by code it does not read. That counts as
// an end too, so that nothing is reported after it. Inside a helper, only
// an end the helper writes counts: a library's own methods store their
// receiver in many places, and would otherwise seem to end it. A
// transaction given to a function value or an interface method, whose body
// is not known, is taken to stay open.
package closer

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

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
	pass  *analysis.Pass
	paths *handle.Paths
	// known holds, for each function and root that has been asked about,
	// whether the function ends the transaction rooted at that parameter or
	// free variable. Every answer starts false and only turns true, in a
	// least fixpoint over all of them.
	known map[closerKey]bool
	// order lists the keys of known in the order they were asked about, so
	// that the fixpoint reads them the same way each time.
	order []closerKey
	// settled is how many keys of order are final. A completed fixpoint
	// leaves its keys final: a key they read was pulled into it. A later
	// fixpoint reads only the keys after them.
	settled int
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

// New is a Closer for pass, which reads handles through paths.
func New(pass *analysis.Pass, paths *handle.Paths) *Closer {
	return &Closer{pass: pass, paths: paths, known: make(map[closerKey]bool)}
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
		return !local && c.within(instr.Val, txs)
	case *ssa.Send:
		return c.within(instr.X, txs)
	case *ssa.MapUpdate:
		return c.within(instr.Key, txs) || c.within(instr.Value, txs)
	case *ssa.Select:
		return slices.ContainsFunc(instr.States, func(st *ssa.SelectState) bool {
			return st.Dir == types.SendOnly && c.within(st.Send, txs)
		})
	case *ssa.MakeClosure:
		return c.closureGivenAway(instr, txs, false)
	}
	return false
}

// callEnds reports whether a call may end the transaction.
func (c *Closer) callEnds(call *ssa.CallCommon, txs []handle.Path) bool {
	if recv, name, ok := receiver(call); ok && opener.Ending(name) && !takesFunc(call.Signature()) && c.within(recv, txs) {
		return true
	}
	fn := call.StaticCallee()
	if fn == nil {
		return false
	}
	for i, arg := range call.Args {
		if c.within(arg, txs) && opener.Holds(arg.Type()) && c.paramEnds(fn, i) {
			return true
		}
	}
	if mc, ok := call.Value.(*ssa.MakeClosure); ok {
		for i, b := range mc.Bindings {
			if c.within(b, txs) && c.rootEnds(fn, fn.FreeVars[i]) {
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

// solve computes the fixpoint over the keys not yet settled, and over the
// keys the reads ask about.
func (c *Closer) solve() {
	c.solving = true
	for changed := true; changed; {
		changed = false
		for i := c.settled; i < len(c.order); i++ {
			key := c.order[i]
			if !c.known[key] && c.rootEndsRead(key.fn, key.root) {
				c.known[key] = true
				changed = true
			}
		}
	}
	c.settled = len(c.order)
	c.solving = false
}

// rootEndsRead reads fn for a call that ends the transaction rooted at root,
// or a literal that ends it and is given away.
func (c *Closer) rootEndsRead(fn *ssa.Function, root ssa.Value) bool {
	txs := []handle.Path{c.paths.Of(root)}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if call, ok := instr.(ssa.CallInstruction); ok && c.callEnds(call.Common(), txs) {
				return true
			}
			// A literal that ends the transaction, given to a call such as
			// a retry, ends it as one the begin's function gives away.
			if mc, ok := instr.(*ssa.MakeClosure); ok && c.closureGivenAway(mc, txs, true) {
				return true
			}
		}
	}
	return false
}

// closureGivenAway reports whether mc makes a literal that ends the
// transaction, and gives it to something other than a call of it or a
// defer. In a helper, every end in the body of a range over a function
// counts: a path that returns from the helper has ended the parameter by
// then. Then it may run anywhere. A closure that nothing uses never runs,
// so it ends nothing.
func (c *Closer) closureGivenAway(mc *ssa.MakeClosure, txs []handle.Path, helper bool) bool {
	if !program.Used(mc) {
		return false
	}
	fn := mc.Fn.(*ssa.Function)
	ends := false
	for i, b := range mc.Bindings {
		if !c.within(b, txs) {
			continue
		}
		if fn.Synthetic == yieldSynthetic && !helper {
			ends = ends || c.yieldEnds(mc, fn.FreeVars[i])
		} else {
			ends = ends || c.rootEnds(fn, fn.FreeVars[i])
		}
	}
	if !ends {
		return false
	}
	for _, ref := range *mc.Referrers() {
		switch ref := ref.(type) {
		case *ssa.DebugRef:
		case ssa.CallInstruction:
			// t.Cleanup runs it after the test returns, as a defer does.
			if ref.Common().Value != mc && !program.Cleanup(ref.Common()) {
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
func (c *Closer) within(v ssa.Value, txs []handle.Path) bool {
	p := c.paths.Of(v)
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

// yieldSynthetic is how the builder marks the function it makes of the body
// of a range over a function.
const yieldSynthetic = "range-over-func yield"

// yieldEnds reports whether the body of a range over a function, which mc
// makes, ends the transaction rooted at root and then goes on: to the next
// iteration, or out of the loop by a break. A path that returns from the
// function that holds the loop takes no part, as in a loop over a slice:
// if err != nil { tx.Rollback(); return err } leaves the transaction open
// after the loop.
func (c *Closer) yieldEnds(mc *ssa.MakeClosure, root ssa.Value) bool {
	fn := mc.Fn.(*ssa.Function)
	txs := []handle.Path{c.paths.Of(root)}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if c.yieldStepEnds(instr, txs) && yieldGoesOn(mc, b) {
				return true
			}
		}
	}
	return false
}

// yieldStepEnds reports whether instr, in the body of a range over a
// function, ends the transaction: a call that ends it, or the body of an
// inner loop that does.
func (c *Closer) yieldStepEnds(instr ssa.Instruction, txs []handle.Path) bool {
	switch instr := instr.(type) {
	case ssa.CallInstruction:
		return c.callEnds(instr.Common(), txs)
	case *ssa.MakeClosure:
		return c.closureGivenAway(instr, txs, false)
	}
	return false
}

// yieldGoesOn reports whether a path from block b of the body that mc makes
// goes on in the loop or after it. The body returns true
// for the next iteration. It returns false to leave the loop, with the exit
// it takes stored in its jump variable. The function that holds the loop
// branches on that exit: to a return for a return in the body, or past the
// loop for a break.
func yieldGoesOn(mc *ssa.MakeClosure, b *ssa.BasicBlock) bool {
	seen := map[*ssa.BasicBlock]bool{b: true}
	for work := []*ssa.BasicBlock{b}; len(work) > 0; {
		b, work = work[len(work)-1], work[:len(work)-1]
		ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		if ok && !yieldReturns(mc, b, ret) {
			return true
		}
		for _, s := range b.Succs {
			if !seen[s] {
				seen[s] = true
				work = append(work, s)
			}
		}
	}
	return false
}

// yieldReturns reports whether ret, at the end of block b of the body that
// mc makes, returns from the function that holds the loop.
func yieldReturns(mc *ssa.MakeClosure, b *ssa.BasicBlock, ret *ssa.Return) bool {
	if done, ok := ret.Results[0].(*ssa.Const); !ok || constant.BoolVal(done.Value) {
		return false
	}
	fn := mc.Fn.(*ssa.Function)
	// The last store is the exit. A body with no branch stores that it is
	// running first, in the same block.
	var st *ssa.Store
	for _, instr := range b.Instrs {
		if yieldExit(instr) {
			st = instr.(*ssa.Store)
		}
	}
	if st == nil {
		// Unreachable: the builder stores the exit in the jump variable
		// before each return of false.
		return false
	}
	return yieldExitReturns(mc.Parent(), mc.Bindings[slices.Index(fn.FreeVars, st.Addr.(*ssa.FreeVar))], st.Val.(*ssa.Const))
}

// yieldExit reports whether instr stores a constant exit in the jump
// variable of the body of a range over a function.
func yieldExit(instr ssa.Instruction) bool {
	st, ok := instr.(*ssa.Store)
	if !ok {
		return false
	}
	fv, isFree := st.Addr.(*ssa.FreeVar)
	_, isConst := st.Val.(*ssa.Const)
	return isFree && isConst && strings.HasPrefix(fv.Name(), "jump$")
}

// yieldExitReturns reports whether fn, which holds the loop, returns when
// the body leaves it by exit, which the body stored in jump. When fn is
// itself the body of an outer loop, it returns false for a break too, so
// its return is read as the body's own.
func yieldExitReturns(fn *ssa.Function, jump ssa.Value, exit *ssa.Const) bool {
	for _, b := range fn.Blocks {
		cond, ok := b.Instrs[len(b.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		cmp, ok := cond.Cond.(*ssa.BinOp)
		if !ok || cmp.Op != token.EQL {
			continue
		}
		load, ok := cmp.X.(*ssa.UnOp)
		k, isConst := cmp.Y.(*ssa.Const)
		if ok && load.X == jump && isConst && constant.Compare(k.Value, token.EQL, exit.Value) {
			then := b.Succs[0]
			ret, returns := then.Instrs[len(then.Instrs)-1].(*ssa.Return)
			// A return in the body comes back through the block the
			// builder makes for it. A labeled break or a goto out of the body
			// jumps to its target, which may end in a return of its own.
			returns = returns && then.Comment == "rangefunc.resume.match"
			if !returns || fn.Synthetic != yieldSynthetic {
				return returns
			}
			made, _ := program.Creation(fn)
			outer, ok := made.(*ssa.MakeClosure)
			return ok && yieldReturns(outer, then, ret)
		}
	}
	return false
}
