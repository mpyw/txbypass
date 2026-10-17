package internal

import (
	"go/ast"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/rule"
)

// checkOuterContext reports each use, inside a callback whose context
// carries the transaction, of the context given to the opener. A statement
// run with that context runs outside the transaction, as with MongoDB's
// WithTransaction.
//
//declscope:shared // run.go calls it for every callback opener whose transaction rides in a context
func (c *checker) checkOuterContext(o opener.Opener) {
	outer, ok := handle.Of(c.pass.TypesInfo, o.Outer)
	if !ok {
		return
	}
	s := txScope{nodes: []ast.Node{o.Callback.Body}, opener: o.Name, tx: o.Tx, unnamed: "the callback's context"}
	c.reportScopeUses(s, outer, rule.OuterContext, s.scopeUse(), nil)
}
