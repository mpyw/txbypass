// Package region finds where a transaction from a begin is open.
//
// A transaction from database/sql's BeginTx, or any other begin, is open from
// the begin until it may have ended. Region reads the SSA form of the
// function that holds the begin. An instruction is inside when the
// transaction is open there on every path from the function's entry: the
// begin comes before it, and nothing that may end the transaction comes
// between. What may end it is closer's to say.
//
// The analysis asks "on every path", so a branch that might have committed
// leaves the code after it outside. The branch where the begin's error is
// set is outside too: the transaction did not begin there. A path that
// never comes back, through a panic or a call such as log.Fatal, takes no
// part.
package region

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/closer"
	"github.com/mpyw/txbypass/internal/handle"
)

// state is what is known of the transaction at a point.
type state int

const (
	// dead is a point no path that returns reaches. It joins any other
	// state as that state.
	dead state = iota
	open
	closed
)

// join is the state where two paths meet.
func join(a, b state) state {
	if a == dead {
		return b
	}
	if b == dead || a == b {
		return a
	}
	return closed
}

// Region is where one begun transaction is open.
type Region struct {
	begin ssa.Instruction
	// tx is the begin's value, and txAddr the variable in memory that holds
	// it, or nil.
	tx     ssa.Value
	txAddr *ssa.Alloc
	txs    []handle.Path
	err    ssa.Value
	closer *closer.Closer
	// entry holds the state at the entry of each block.
	entry map[*ssa.BasicBlock]state
	// ends remembers what closer said of each instruction. The fixpoint
	// steps through a block once for each time its entry changes.
	ends map[ssa.Instruction]bool
	// at holds the state before each instruction, once the fixpoint is
	// done. A rule asks about every use in the region.
	at map[ssa.Instruction]state
	// bodies holds the bodies of ranges over functions already read.
	bodies map[*ssa.Function]bool
}

// build computes the region of the transaction that begin returns, in begin's
// function. tx is the transaction's value. err is the begin's error, or
// nil. A transaction kept in a variable in memory is also every load of it,
// so its address is given as txAddr, or nil. Handles are read through
// paths.
func build(begin *ssa.Call, tx, err ssa.Value, txAddr *ssa.Alloc, c *closer.Closer, paths *handle.Paths) *Region {
	r := &Region{
		begin: begin, tx: tx, txAddr: txAddr, txs: []handle.Path{paths.Of(tx)}, err: err, closer: c,
		entry: make(map[*ssa.BasicBlock]state), ends: make(map[ssa.Instruction]bool), at: make(map[ssa.Instruction]state),
		bodies: make(map[*ssa.Function]bool),
	}
	if txAddr != nil {
		r.txs = append(r.txs, paths.Of(txAddr))
	}
	r.solve(begin.Parent(), closed)
	return r
}

// Begun computes the region of the transaction that begin returns. The
// transaction is the begin's first result, and its error the second. A
// transaction kept in a variable that a literal captures lives in memory, at
// the address it is stored to.
func Begun(begin *ssa.Call, c *closer.Closer, paths *handle.Paths) *Region {
	var tx, err ssa.Value = begin, nil
	for _, ref := range *begin.Referrers() {
		if ex, ok := ref.(*ssa.Extract); ok {
			switch ex.Index {
			case 0:
				tx = ex
			case 1:
				err = ex
			}
		}
	}
	var addr *ssa.Alloc
	for _, ref := range *tx.Referrers() {
		// A store to a field, after the variable's own, does not forget the
		// variable.
		if st, ok := ref.(*ssa.Store); ok {
			if a, ok := st.Addr.(*ssa.Alloc); ok {
				addr = a
			}
		}
	}
	return build(begin, tx, err, addr, c, paths)
}

// Txs are the values of the transaction: the begin's, and the variable in
// memory that holds it, if any.
func (r *Region) Txs() []handle.Path {
	return r.txs
}

// GivesErr reports whether v, returned beside the transaction, is the
// begin's error, or nil. For a begin with no error result, as gorm's Begin,
// the begin's error is the transaction's Error field. A named result that a
// deferred literal reads lives in memory, so v may be a load of it.
func (r *Region) GivesErr(v ssa.Value) bool {
	if c, ok := v.(*ssa.Const); ok && c.IsNil() {
		return true
	}
	return r.holdsErr(v)
}

// solve computes the state before each instruction of fn, whose entry is in
// state start.
func (r *Region) solve(fn *ssa.Function, start state) {
	for _, b := range fn.Blocks[1:] {
		r.entry[b] = dead
	}
	r.entry[fn.Blocks[0]] = start
	for changed := true; changed; {
		changed = false
		for _, b := range fn.Blocks[1:] {
			in := dead
			for _, p := range b.Preds {
				in = join(in, r.edge(p, b))
			}
			if in != r.entry[b] {
				r.entry[b] = in
				changed = true
			}
		}
	}
	for _, b := range fn.Blocks {
		s := r.entry[b]
		for _, instr := range b.Instrs {
			r.at[instr] = s
			s = r.step(instr, s)
		}
	}
}

// InsideBody reports whether the transaction is open at instr, in body, the
// function the builder makes of the body of a range over a function. The
// body is entered where the transaction is open, on the first iteration.
// It is entered again where it returns true for the next one, so an end
// that goes on to the next iteration leaves the entry outside.
func (r *Region) InsideBody(body *ssa.Function, instr ssa.Instruction) bool {
	if !r.bodies[body] {
		r.bodies[body] = true
		r.solve(body, open)
		if r.endsIteration(body) {
			r.solve(body, closed)
		}
	}
	return r.at[instr] == open
}

// endsIteration reports whether body returns true, for the next iteration,
// where the transaction may have ended.
func (r *Region) endsIteration(body *ssa.Function) bool {
	for _, b := range body.Blocks {
		ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		if !ok {
			continue
		}
		next, ok := ret.Results[0].(*ssa.Const)
		if ok && constant.BoolVal(next.Value) && r.at[ret] == closed {
			return true
		}
	}
	return false
}

// edge is the state on the edge from p to b.
func (r *Region) edge(p, b *ssa.BasicBlock) state {
	s := r.entry[p]
	for _, instr := range p.Instrs {
		s = r.step(instr, s)
	}
	if s == open && r.failed(p, b) {
		return closed
	}
	return s
}

// step is the state after instr.
func (r *Region) step(instr ssa.Instruction, s state) state {
	switch {
	case instr == r.begin:
		return open
	case s == open && r.endsAt(instr):
		return closed
	case s == open && r.requiresFailure(instr):
		return closed
	}
	return s
}

// requireFailures are the functions of testify's require, and the methods of
// its Assertions, that return only when the error they are given is set.
var requireFailures = map[string]bool{
	"Error": true, "Errorf": true,
	"ErrorIs": true, "ErrorIsf": true,
	"ErrorAs": true, "ErrorAsf": true,
	"ErrorContains": true, "ErrorContainsf": true,
}

// requiresFailure reports whether instr is a call of testify's require that
// asserts the begin failed, as require.Error(t, err). The test goes on only
// when no transaction began. The error is argument 1 of the function, after
// t, and of the method, after the receiver.
func (r *Region) requiresFailure(instr ssa.Instruction) bool {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return false
	}
	fn := call.Common().StaticCallee()
	if fn == nil || fn.Pkg == nil || fn.Pkg.Pkg.Path() != "github.com/stretchr/testify/require" || !requireFailures[fn.Name()] {
		return false
	}
	args := call.Common().Args
	return len(args) > 1 && r.holdsErr(args[1])
}

// endsAt reports whether instr may end the transaction, as closer says.
func (r *Region) endsAt(instr ssa.Instruction) bool {
	ends, ok := r.ends[instr]
	if !ok {
		ends = r.closer.Ends(instr, r.txs)
		r.ends[instr] = ends
	}
	return ends
}

// failed reports whether the edge from p to b is taken when the begin's
// error is set, so the transaction did not begin there. That is the true
// edge of "err != nil", or the false edge of "err == nil".
func (r *Region) failed(p, b *ssa.BasicBlock) bool {
	cond, ok := p.Instrs[len(p.Instrs)-1].(*ssa.If)
	if !ok {
		return false
	}
	cmp, ok := cond.Cond.(*ssa.BinOp)
	if !ok || !r.isErr(cmp.X, cmp.Y) && !r.isErr(cmp.Y, cmp.X) {
		return false
	}
	return cmp.Op == token.NEQ && b == p.Succs[0] || cmp.Op == token.EQL && b == p.Succs[1]
}

// isErr reports whether v is the begin's error, compared with nil. A
// variable in memory that holds the error, as a named result a deferred
// literal reads, is it when every path to the load last stored the begin's
// error there. A begin with no error result, as gorm's Begin, reports
// failure in the transaction's Error field: tx.Error is its error.
func (r *Region) isErr(v, other ssa.Value) bool {
	c, ok := other.(*ssa.Const)
	return ok && c.IsNil() && r.holdsErr(v)
}

// holdsErr reports whether v is the begin's error, or a load of a variable
// in memory that last stored it on every path.
func (r *Region) holdsErr(v ssa.Value) bool {
	return r.loaded(v, r.isBeginErr, make(map[*ssa.UnOp]bool))
}

// loaded reports whether v is a value that is accepts, or a load of a
// variable in memory that last stored such a value on every path. A value
// stored may itself be such a load: a return of named results, before
// deferred calls, loads each result and stores it back. A load met again
// around a loop leaves the answer to the other paths.
func (r *Region) loaded(v ssa.Value, is func(ssa.Value) bool, seen map[*ssa.UnOp]bool) bool {
	if is(v) {
		return true
	}
	load, ok := v.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	if seen[load] {
		return true
	}
	seen[load] = true
	return lastStored(load.Block(), slices.Index(load.Block().Instrs, ssa.Instruction(load)), load.X, func(x ssa.Value) bool { return r.loaded(x, is, seen) }, make(map[*ssa.BasicBlock]bool))
}

// isBeginErr reports whether v is the begin's error itself: its error
// result, or for a begin with none, its Error field.
func (r *Region) isBeginErr(v ssa.Value) bool {
	if r.err == nil {
		return r.isErrorField(v)
	}
	return v == r.err
}

// isErrorField reports whether v reads the field Error of the transaction
// itself: of the begin's value, or of a load of the variable that holds it.
// tx.Create(u).Error is the error of another statement, not the begin's.
func (r *Region) isErrorField(v ssa.Value) bool {
	if load, ok := v.(*ssa.UnOp); ok && load.Op == token.MUL {
		v = load.X
	}
	f, ok := v.(*ssa.FieldAddr)
	if !ok {
		return false
	}
	field := f.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Struct).Field(f.Field)
	load, loaded := f.X.(*ssa.UnOp)
	direct := f.X == r.tx || loaded && load.Op == token.MUL && r.txAddr != nil && load.X == r.txAddr
	return field.Name() == "Error" && direct
}

// lastStored reports whether every path to instruction at of b last stores
// a value that v accepts to addr. A variable stored on one path only has a path that reaches the
// entry with no store, so it is not the begin's error.
// A block met again around a loop leaves the answer to the other paths.
func lastStored(b *ssa.BasicBlock, at int, addr ssa.Value, v func(ssa.Value) bool, seen map[*ssa.BasicBlock]bool) bool {
	for i := at - 1; i >= 0; i-- {
		if st, ok := b.Instrs[i].(*ssa.Store); ok && st.Addr == addr {
			return v(st.Val)
		}
	}
	// A path that reaches the entry stored nothing to addr, so addr does not
	// hold the begin's error there.
	if len(b.Preds) == 0 {
		return false
	}
	if seen[b] {
		return true
	}
	seen[b] = true
	for _, p := range b.Preds {
		if !lastStored(p, len(p.Instrs), addr, v, seen) {
			return false
		}
	}
	return true
}

// Inside reports whether the transaction is open at instr, on every path.
func (r *Region) Inside(instr ssa.Instruction) bool {
	return r.at[instr] == open
}
