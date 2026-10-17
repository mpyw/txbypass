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
	if !ok || o.Nests && outerHandleSavepoint(outer) {
		return
	}
	lit := c.prog.Func(o.Callback)
	s := txScope{root: o.Callback.Body, fn: lit, opener: o.Name, tx: o.Tx, unnamed: "the callback's transaction", builds: o.Builds}
	if o.Tx != nil {
		s.txs = []handle.Path{c.paths.Of(outerHandleParam(lit, c.pass.TypesInfo.Defs[o.Tx]))}
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
// its own. So does one in the body of a range over a function.
//
//declscope:shared // run.go calls it for every function
func (c *checker) checkOuterHandleBegins(fn *ssa.Function, body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.RangeStmt:
			if c.prog.Func(n) != nil {
				return false
			}
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
	call, ok := program.Call(fn, b.Call)
	if !ok {
		// The begin is in code that no path reaches. It never runs.
		return
	}
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
	r := region.New(call, tx, err, addr, c.closer, c.paths)
	s := txScope{root: body, fn: fn, region: r, opener: b.Name, tx: b.Tx, txs: []handle.Path{c.paths.Of(tx)}}
	if addr != nil {
		s.txs = append(s.txs, c.paths.Of(addr))
	}
	// The receiver of a method call is always evaluated as a value.
	outerRef, _ := c.prog.Ref(fn, b.Outer.X)
	outer := c.paths.Of(outerRef.X).With(handle.Promoted(c.pass.TypesInfo.Selections[b.Outer]))
	if b.Nests && outerHandleSavepoint(outer) {
		return
	}
	c.reportScopeUses(s, outer, rule.OuterHandle, s.scopeUse(), nil)
	c.reportScopeQueries(s)
}

// outerHandleSavepoint reports whether outer is a transaction, so that a
// transaction opened on it, by an opener or a begin that nests, is a
// savepoint. The savepoint runs on the
// transaction's connection, and so does a statement on the transaction:
// it bypasses nothing. outer is a transaction when it is a begin's result,
// as gorm's tx := db.Begin(), or the transaction a callback is given, as
// gorm's tx in db.Transaction(func(tx *gorm.DB) error { ... }).
func outerHandleSavepoint(outer handle.Path) bool {
	switch root := outer.Root().(type) {
	case *ssa.Extract:
		return root.Index == 0 && outerHandleBegun(root.Tuple)
	case *ssa.Call:
		return outerHandleBegun(root)
	case *ssa.Parameter:
		return scopeGiven(root, opener.CallbackTx)
	}
	return false
}

// outerHandleBegun reports whether v is the call of a begin.
func outerHandleBegun(v ssa.Value) bool {
	call, ok := v.(*ssa.Call)
	if !ok {
		return false
	}
	fn, ok := scopeCallee(call.Common())
	return ok && opener.IsBeginFunc(fn)
}
