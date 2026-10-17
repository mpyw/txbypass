package internal

import (
	"go/ast"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/region"
	"github.com/mpyw/txbypass/internal/rule"
)

// checkOuterHandle reports each use, inside the callback of a transaction,
// of the handle the transaction was opened on. A statement run through that
// handle runs outside the transaction: it is not rolled back with it, and it
// may wait on a lock the transaction holds.
//
//declscope:shared // run.go calls it for every callback opener
func (c *checker) checkOuterHandle(o opener.Opener) {
	s := txScope{nodes: []ast.Node{o.Callback.Body}, opener: o.Name, tx: o.Tx, unnamed: "the callback's transaction", builds: o.Builds}
	c.reportOuterHandle(s, o.Outer)
}

// checkOuterHandleBegins reports each use of a handle while a transaction
// begun on it is open, in the function whose body is body. A begin inside a
// function literal in body belongs to the literal, which is checked on its
// own.
//
//declscope:shared // run.go calls it for every function
func (c *checker) checkOuterHandleBegins(body *ast.BlockStmt) {
	info := c.pass.TypesInfo
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		b, ok := opener.BeginOf(info, n)
		if !ok {
			return true
		}
		var err = info.ObjectOf(b.Err)
		if b.Err == nil {
			err = nil
		}
		nodes := region.Inside(info, body, n, info.ObjectOf(b.Tx), err)
		c.reportOuterHandle(txScope{nodes: nodes, opener: b.Name, tx: b.Tx}, b.Outer)
		return true
	})
}

// reportOuterHandle reports the uses in s of the handle outer names, and of
// each handle s rebinds to its transaction with WithTx.
func (c *checker) reportOuterHandle(s txScope, outer ast.Expr) {
	if p, ok := handle.Of(c.pass.TypesInfo, outer); ok {
		c.reportScopeUses(s, p, rule.OuterHandle, s.scopeUse(), nil)
	}
	c.reportScopeQueries(s)
}
