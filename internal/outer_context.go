package internal

import (
	"go/types"

	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/rule"
	"github.com/mpyw/txbypass/internal/txcall"
)

// checkOuterContext reports each use, inside a callback whose context
// carries the transaction, of the context given to the invoker. A statement
// run with that context runs outside the transaction, as with MongoDB's
// WithTransaction. fn is the function the invoker is called in.
//
//declscope:shared // run.go calls it for every invoker whose transaction rides in a context
func (c *checker) checkOuterContext(o txcall.Invoker, fn *ssa.Function) {
	outer, ok := c.helpers.Outer(fn, o.Outer, o.Receiver, o.Fields)
	if !ok {
		return
	}
	// The context of MongoDB's WithSession carries the session. A
	// transaction on it, as sess.WithTransaction(sc, ...), runs with sc too:
	// a statement on sc runs inside it.
	if p, ok := outer.Root().(*ssa.Parameter); ok && outer.Depth() == 0 && scopeGiven(p, func(fn *types.Func, _, i int) bool { return txcall.CallbackSession(fn, i) }) {
		return
	}
	// An invoker that nests, as go-transaction-manager's Do or go-rel's
	// Transaction, given the context of another one's callback, joins that
	// transaction or opens a savepoint in it. A statement with the outer
	// context runs inside it.
	if p, ok := outer.Root().(*ssa.Parameter); ok && outer.Depth() == 0 && o.Nests && scopeGiven(p, func(fn *types.Func, _, i int) bool { return txcall.CallbackContext(fn, i) }) {
		return
	}
	s := txScope{root: o.Callback.Body, fn: c.prog.Func(o.Callback), openedWith: o.Name, tx: o.Tx, unnamed: "the callback's context"}
	c.reportScopeUses(s, outer, rule.OuterContext, s.scopeUse(), nil)
}
