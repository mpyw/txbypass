package internal

import (
	"go/ast"

	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/region"
	"github.com/mpyw/txbypass/internal/rule"
)

// checkOuterHandle reports each use, inside the callback of a transaction,
// of the handle the transaction was opened on. A statement run through that
// handle runs outside the transaction: it is not rolled back with it, and it
// may wait on a lock the transaction holds. fn is the function the opener
// is called in.
//
//declscope:shared // run.go calls it for every callback opener
func (c *checker) checkOuterHandle(o opener.Opener, fn *ssa.Function) {
	outer, ok := c.scopeOuter(o, fn)
	if !ok {
		return
	}
	lit := c.prog.Func(o.Callback)
	s := txScope{root: o.Callback.Body, fn: lit, opener: o.Name, tx: o.Tx, unnamed: "the callback's transaction", builds: o.Builds}
	if o.Tx != nil {
		s.txs = []handle.Path{handle.Of(outerHandleParam(lit, c.pass.TypesInfo.Defs[o.Tx]))}
	}
	c.reportScopeUses(s, outer, rule.OuterHandle, s.scopeUse(), nil)
	c.reportScopeQueries(s)
}

// outerHandleParam is the parameter of fn that obj declares.
func outerHandleParam(fn *ssa.Function, obj any) ssa.Value {
	for _, p := range fn.Params {
		if p.Object() == obj {
			return p
		}
	}
	// Unreachable: the transaction is a parameter of the callback, which
	// the builder made from the same declaration.
	panic("outer handle: no parameter for the transaction")
}

// checkOuterHandleBegins reports each use of a handle while a transaction
// begun on it is open, in the function fn whose body is body. A begin inside
// a function literal in body belongs to the literal, which is checked on
// its own.
//
//declscope:shared // run.go calls it for every function
func (c *checker) checkOuterHandleBegins(fn *ssa.Function, body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		b, ok := opener.BeginOf(c.pass.TypesInfo, n)
		if !ok {
			return true
		}
		c.checkOuterHandleBegin(fn, body, b)
		return true
	})
}

// checkOuterHandleBegin reports the uses of b's handle in its region.
func (c *checker) checkOuterHandleBegin(fn *ssa.Function, body *ast.BlockStmt, b opener.Begin) {
	call := program.Call(fn, b.Call)
	var tx, err ssa.Value = call, nil
	for _, ref := range *call.Referrers() {
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
		if st, ok := ref.(*ssa.Store); ok {
			addr, _ = st.Addr.(*ssa.Alloc)
		}
	}
	r := region.New(call, tx, err, addr, c.closer)
	s := txScope{root: body, fn: fn, region: r, opener: b.Name, tx: b.Tx, txs: []handle.Path{handle.Of(tx)}}
	if addr != nil {
		s.txs = append(s.txs, handle.Of(addr))
	}
	// The receiver of a method call is always evaluated as a value.
	outerRef, _ := c.prog.Ref(fn, b.Outer.X)
	outer := handle.Of(outerRef.X).With(handle.Promoted(c.pass.TypesInfo.Selections[b.Outer]))
	c.reportScopeUses(s, outer, rule.OuterHandle, s.scopeUse(), nil)
	c.reportScopeQueries(s)
}
