package internal

import (
	"go/ast"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/rule"
)

// checkOuterHandle reports each use, inside the callback of a transaction,
// of the handle the transaction was opened on. A statement run through that
// handle runs outside the transaction: it is not rolled back with it, and it
// may wait on a lock the transaction holds.
//
//declscope:shared // run.go calls it for every call
func (c *checker) checkOuterHandle(call *ast.CallExpr) {
	info := c.pass.TypesInfo
	o, ok := opener.Of(info, call)
	if !ok {
		return
	}
	outer, ok := handle.Of(info, o.Outer)
	if !ok {
		return
	}
	msg := outer.Spell(c.pass.Pkg) + " bypasses the transaction it opened with " + o.Name + "; "
	if o.Tx == "" {
		msg += "use the callback's transaction instead"
	} else {
		msg += "use " + o.Tx + " instead"
	}
	ast.Inspect(o.Callback.Body, func(n ast.Node) bool {
		e, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		p, ok := handle.Of(info, e)
		if !ok || !p.Equal(outer) {
			return true
		}
		c.report(e.Pos(), rule.OuterHandle, msg)
		return false
	})
}
