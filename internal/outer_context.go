package internal

import (
	"golang.org/x/tools/go/ssa"

	"github.com/mpyw/txbypass/internal/opener"
	"github.com/mpyw/txbypass/internal/rule"
)

// checkOuterContext reports each use, inside a callback whose context
// carries the transaction, of the context given to the opener. A statement
// run with that context runs outside the transaction, as with MongoDB's
// WithTransaction. fn is the function the opener is called in.
//
//declscope:shared // run.go calls it for every callback opener whose transaction rides in a context
func (c *checker) checkOuterContext(o opener.Opener, fn *ssa.Function) {
	outer, ok := c.scopeOuter(o, fn)
	if !ok {
		return
	}
	s := txScope{root: o.Callback.Body, fn: c.prog.Func(o.Callback), opener: o.Name, tx: o.Tx, unnamed: "the callback's context"}
	c.reportScopeUses(s, outer, rule.OuterContext, s.scopeUse(), nil)
}
