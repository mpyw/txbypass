package txhelper

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/txcall"
)

// readInvoker reads decl for a function parameter that it runs inside a
// transaction. The first such parameter is the callback.
//
//declscope:shared // helper.go reads a function's shape through it
func (h *Helpers) readInvoker(decl *ast.FuncDecl) *txcall.InvokerShape {
	fn := h.prog.Func(decl)
	var txs []helperTransaction
	for i, p := range fn.Params {
		if _, ok := p.Type().Underlying().(*types.Signature); !ok {
			continue
		}
		if txs == nil {
			txs = h.transactions(fn, decl)
		}
		if s, ok := h.invokerOfParam(fn, i, txs); ok {
			return &s
		}
	}
	return nil
}

// invokerOfParam reports the shape of fn when its parameter i is a callback that it
// runs only inside transactions of txs, opened on one handle that a
// parameter holds, and is given the transaction at one place.
func (h *Helpers) invokerOfParam(fn *ssa.Function, i int, txs []helperTransaction) (txcall.InvokerShape, bool) {
	cb := h.paths.Of(fn.Params[i])
	var shape *txcall.InvokerShape
	ok := true
	program.Walk(fn, func(instr ssa.Instruction) {
		if !ok {
			return
		}
		if call, isCall := instr.(ssa.CallInstruction); isCall && h.paths.Of(call.Common().Value).Equal(cb) {
			s, runs := h.invokerCallInside(fn, call, txs)
			ok = runs && (shape == nil || invokerShapesEqual(*shape, s))
			shape = &s
			return
		}
		ok = !h.invokerHandsOn(instr, cb)
	})
	if !ok || shape == nil {
		return txcall.InvokerShape{}, false
	}
	shape.Callback = i
	return *shape, true
}

// invokerCallInside reports the shape of a call of the callback, when it is a plain
// call inside one of txs, given the transaction.
func (h *Helpers) invokerCallInside(fn *ssa.Function, call ssa.CallInstruction, txs []helperTransaction) (txcall.InvokerShape, bool) {
	c, ok := call.(*ssa.Call)
	if !ok {
		// A go statement may outlive the transaction, and a deferred call
		// runs after the code that may end it.
		return txcall.InvokerShape{}, false
	}
	for _, t := range txs {
		if t.fn != c.Parent() || t.region != nil && !t.region.Inside(c) {
			continue
		}
		at := slices.IndexFunc(c.Call.Args, func(arg ssa.Value) bool {
			return slices.ContainsFunc(t.txs, h.paths.Of(arg).Equal)
		})
		if at < 0 {
			continue
		}
		if outer, fields, ok := helperParam(fn, t.outer); ok {
			return txcall.InvokerShape{Outer: outer, Fields: fields, Tx: at, Nests: t.nests}, true
		}
	}
	return txcall.InvokerShape{}, false
}

// invokerHandsOn reports whether instr hands the callback cb on, other than by
// calling it: as an argument, a stored value, or one edge of a phi. A store
// into a local variable moves nothing out of reach: the loads of it are the
// callback again. A debug reference does nothing, and neither does a
// comparison: a function is only compared with nil, as if fn == nil does.
func (h *Helpers) invokerHandsOn(instr ssa.Instruction, cb handle.Path) bool {
	switch instr := instr.(type) {
	case *ssa.DebugRef, *ssa.BinOp:
		return false
	case *ssa.Store:
		// Only a variable written once, as a whole, gives the callback at
		// every load. One written more than once is a value of its own, and
		// a use of it is not seen as the callback's.
		if alloc, local := instr.Addr.(*ssa.Alloc); local && h.paths.Of(alloc).Equal(cb) {
			return false
		}
	}
	for _, op := range instr.Operands(nil) {
		if *op == nil {
			continue
		}
		if _, isFunc := (*op).Type().Underlying().(*types.Signature); isFunc && h.paths.Of(*op).Equal(cb) {
			return true
		}
	}
	return false
}

// invokerShapesEqual reports whether a and b, read from two calls of one callback,
// are one shape. The callback is set once the calls agree.
func invokerShapesEqual(a, b txcall.InvokerShape) bool {
	return a.Outer == b.Outer && slices.Equal(a.Fields, b.Fields) && a.Tx == b.Tx && a.Nests == b.Nests
}
