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
	"go/token"

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
	begin  ssa.Instruction
	txs    []handle.Path
	err    ssa.Value
	closer *closer.Closer
	// entry holds the state at the entry of each block.
	entry map[*ssa.BasicBlock]state
}

// New computes the region of the transaction that begin returns, in begin's
// function. tx is the transaction's value. err is the begin's error, or
// nil. A transaction kept in a variable in memory is also every load of it,
// so its address is given as txAddr, or nil.
func New(begin *ssa.Call, tx, err ssa.Value, txAddr *ssa.Alloc, c *closer.Closer) *Region {
	r := &Region{begin: begin, txs: []handle.Path{handle.Of(tx)}, err: err, closer: c, entry: make(map[*ssa.BasicBlock]state)}
	if txAddr != nil {
		r.txs = append(r.txs, handle.Of(txAddr))
	}
	fn := begin.Parent()
	for _, b := range fn.Blocks[1:] {
		r.entry[b] = dead
	}
	r.entry[fn.Blocks[0]] = closed
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
	return r
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
	case s == open && r.closer.Ends(instr, r.txs):
		return closed
	}
	return s
}

// failed reports whether the edge from p to b is taken when the begin's
// error is set, so the transaction did not begin there. That is the true
// edge of "err != nil", or the false edge of "err == nil".
func (r *Region) failed(p, b *ssa.BasicBlock) bool {
	if r.err == nil {
		return false
	}
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
// variable in memory that holds the error is it when loaded right after the
// begin, in the begin's block.
func (r *Region) isErr(v, other ssa.Value) bool {
	if c, ok := other.(*ssa.Const); !ok || !c.IsNil() {
		return false
	}
	if v == r.err {
		return true
	}
	load, ok := v.(*ssa.UnOp)
	if !ok || load.Op != token.MUL || load.Block() != r.begin.Block() {
		return false
	}
	return storedBefore(load, r.err)
}

// storedBefore reports whether the last store to load's address before load,
// in its block, writes v.
func storedBefore(load *ssa.UnOp, v ssa.Value) bool {
	var last ssa.Value
	for _, instr := range load.Block().Instrs {
		if instr == load {
			break
		}
		if st, ok := instr.(*ssa.Store); ok && st.Addr == load.X {
			last = st.Val
		}
	}
	return last == v
}

// Inside reports whether the transaction is open at instr, on every path.
func (r *Region) Inside(instr ssa.Instruction) bool {
	s := r.entry[instr.Block()]
	for _, i := range instr.Block().Instrs {
		if i == instr {
			break
		}
		s = r.step(i, s)
	}
	return s == open
}
