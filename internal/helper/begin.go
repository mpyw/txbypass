package helper

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/region"
	"github.com/mpyw/txbypass/internal/txcall"
)

// readBegin reads decl for a function that returns a transaction it
// begins, open, on a handle that a parameter holds. Its first result is the
// transaction, and its second, if any, an error. Every return gives the
// transaction, from begins on one handle, or nil.
//
//declscope:shared // helper.go reads a function's shape through it
func (h *Helpers) readBegin(decl *ast.FuncDecl) *txcall.BeginShape {
	fn := h.prog.Func(decl)
	res := fn.Signature.Results()
	if res.Len() == 0 || res.Len() > 2 || !h.decl.Holds(res.At(0).Type()) || res.Len() == 2 && !types.Identical(res.At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	txs := h.transactions(fn, decl)
	var shape *txcall.BeginShape
	for _, b := range fn.Blocks {
		ret, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		// A return after a deferred call recovers from a panic takes no
		// part, as a path through a panic takes none in a region.
		if !ok || b == fn.Recover {
			continue
		}
		// A named result that a deferred literal reads is returned through
		// a load of what the return stored.
		if region.Stored(ret.Results[0], beginNilConst) {
			// No transaction is returned. The caller has to read an error
			// beside it, or it reads a begin that did not happen.
			if len(ret.Results) < 2 || region.Stored(ret.Results[1], beginNilConst) {
				return nil
			}
			continue
		}
		if len(ret.Results) == 2 && beginErrRewritten(ret.Results[1]) {
			return nil
		}
		s, ok := h.beginReturned(fn, ret, txs)
		if !ok || shape != nil && !beginShapeEqual(*shape, s) {
			return nil
		}
		shape = &s
	}
	return shape
}

// beginNilConst reports whether v is the constant nil.
func beginNilConst(v ssa.Value) bool {
	c, ok := v.(*ssa.Const)
	return ok && c.IsNil()
}

// beginErrRewritten reports whether v, an error result, is a load of a
// variable that a function literal may set to another error, or whose
// address is handed on, as defer annotate(&err) or check(&err) does. A
// literal that sets it only where it is not nil, as one that wraps it,
// leaves a nil error nil, so a transaction returned beside it stays the
// begin's. A store of the function's own is followed by the region.
func beginErrRewritten(v ssa.Value) bool {
	load, ok := v.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	alloc, ok := load.X.(*ssa.Alloc)
	if !ok {
		return false
	}
	for _, ref := range *alloc.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp, *ssa.DebugRef:
		case *ssa.Store:
			if ref.Addr != alloc {
				return true
			}
		case *ssa.MakeClosure:
			lit := ref.Fn.(*ssa.Function)
			for i, b := range ref.Bindings {
				if b == alloc && beginErrSet(lit.FreeVars[i]) {
					return true
				}
			}
		default:
			return true
		}
	}
	return false
}

// beginErrSet reports whether a literal may set fv to an error that is
// not nil where it held nil: by a store not under fv != nil, or by handing
// fv on.
func beginErrSet(fv *ssa.FreeVar) bool {
	for _, ref := range *fv.Referrers() {
		switch ref := ref.(type) {
		case *ssa.UnOp, *ssa.DebugRef:
		case *ssa.Store:
			// A store of fv itself, rather than into it, hands it on.
			if ref.Addr != fv || !beginUnderNotNil(ref.Block(), fv) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// beginUnderNotNil reports whether b runs only where fv holds an error
// that is not nil: it is dominated by the true edge of fv != nil.
func beginUnderNotNil(b *ssa.BasicBlock, fv *ssa.FreeVar) bool {
	for c := b; c.Idom() != nil; c = c.Idom() {
		p := c.Idom()
		cond, ok := p.Instrs[len(p.Instrs)-1].(*ssa.If)
		if !ok || !p.Succs[0].Dominates(b) {
			continue
		}
		cmp, ok := cond.Cond.(*ssa.BinOp)
		if !ok || cmp.Op != token.NEQ {
			continue
		}
		x, y := cmp.X, cmp.Y
		if beginNilConst(x) {
			x, y = y, x
		}
		if load, ok := x.(*ssa.UnOp); ok && load.Op == token.MUL && load.X == fv && beginNilConst(y) {
			return true
		}
	}
	return false
}

// beginReturned is the shape of a begin in fn whose transaction ret returns,
// open.
func (h *Helpers) beginReturned(fn *ssa.Function, ret *ssa.Return, txs []helperTransaction) (txcall.BeginShape, bool) {
	v := h.paths.Of(ret.Results[0])
	for _, t := range txs {
		if t.region == nil || t.fn != fn || !t.region.Inside(ret) || !slices.ContainsFunc(t.txs, v.Equal) || h.beginDeferredOn(fn, t.txs) {
			continue
		}
		// A second result tells the caller whether the begin failed. Any
		// other error leaves the transaction open where the caller reads
		// none.
		if len(ret.Results) == 2 && !t.region.GivesErr(ret.Results[1]) {
			continue
		}
		if outer, fields, ok := helperParam(fn, t.outer); ok {
			return txcall.BeginShape{Outer: outer, Fields: fields, Nests: t.nests}, true
		}
	}
	return txcall.BeginShape{}, false
}

// beginDeferredOn reports whether a deferred call of fn is given a value of txs,
// as defer tx.Rollback() is, or a literal that holds one. It runs after the
// return, so it may end the transaction before the caller uses it. The
// region does not see it: a deferred call ends nothing where it is written.
func (h *Helpers) beginDeferredOn(fn *ssa.Function, txs []handle.Path) bool {
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			d, ok := instr.(*ssa.Defer)
			if !ok {
				continue
			}
			vals := append([]ssa.Value{d.Call.Value}, d.Call.Args...)
			if mc, ok := d.Call.Value.(*ssa.MakeClosure); ok {
				vals = append(vals, mc.Bindings...)
			}
			for _, v := range vals {
				p := h.paths.Of(v)
				if slices.ContainsFunc(txs, p.Within) {
					return true
				}
			}
		}
	}
	return false
}

// beginShapeEqual reports whether a and b, read from two returns, are one
// shape.
func beginShapeEqual(a, b txcall.BeginShape) bool {
	return a.Outer == b.Outer && slices.Equal(a.Fields, b.Fields) && a.Nests == b.Nests
}
