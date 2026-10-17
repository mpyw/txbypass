package internal

import (
	"go/ast"
	"go/types"
	"slices"

	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/rule"
)

// txScope is the code that runs inside one transaction: the body of a
// callback, or the nodes of a region where a begun transaction is open.
//
//declscope:shared // outer_handle.go and outer_context.go build one for each transaction
type txScope struct {
	nodes []ast.Node
	// opener names the call that opened the transaction, for messages.
	opener string
	// tx is the variable or parameter that holds the transaction, or the
	// context that carries it. It is nil when there is none to name.
	tx *ast.Ident
	// unnamed is what a message suggests when tx is nil.
	unnamed string
	// builds reports whether the handle builds references for the
	// transaction, so that a use counts only when it runs. See
	// opener.Opener.Builds.
	builds bool
}

// reportScopeUses reports, as rule r, each expression in s that names outer.
// use is what the message tells the reader to use instead. An expression in
// exempt is not read, and neither is a chain built on one, as in
// q.WithTx(tx).CreateUser(ctx).
//
//declscope:shared // the rules report what their scopes find through it
func (c *checker) reportScopeUses(s txScope, outer handle.Path, r rule.Name, use string, exempt map[ast.Node]bool) {
	info := c.pass.TypesInfo
	for _, n := range s.nodes {
		ast.PreorderStack(n, nil, func(n ast.Node, stack []ast.Node) bool {
			e, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			if exempt[e] {
				return false
			}
			if scopeChainsOn(e, exempt) {
				return true
			}
			p, ok := handle.Of(info, e)
			if !ok || !p.Equal(outer) {
				return true
			}
			if !s.builds || scopeRuns(info, e, stack) {
				c.report(e.Pos(), r, p.Spell(c.pass.Pkg)+" bypasses the transaction opened with "+s.opener+"; use "+use+" instead")
			}
			return false
		})
	}
}

// scopeRuns reports whether the handle e names runs something where it is
// used, for a handle that builds references. stack holds e's ancestors. A
// chain of method calls on e runs when one of its calls takes a context, as
// client.Doc(p).Set(ctx, v) does. A chain of builders alone, as in
// client.Doc(p), only makes a reference. The handle on its own, given to a
// function, runs whatever the function does with it.
func scopeRuns(info *types.Info, e ast.Expr, stack []ast.Node) bool {
	chained := false
walk:
	for _, n := range slices.Backward(stack) {
		switch parent := n.(type) {
		case *ast.SelectorExpr:
			// e is what the selector selects from: a handle is never the
			// name after the dot.
			e = parent
		case *ast.CallExpr:
			if parent.Fun != e {
				break walk
			}
			if scopeTakesContext(info, parent) {
				return true
			}
			chained = true
			e = parent
		default:
			break walk
		}
	}
	return !chained
}

// scopeTakesContext reports whether call passes a context.Context.
func scopeTakesContext(info *types.Info, call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if t, ok := info.TypeOf(arg).(*types.Named); ok && t.Obj().Pkg() != nil && t.Obj().Pkg().Path() == "context" && t.Obj().Name() == "Context" {
			return true
		}
	}
	return false
}

// scopeChainsOn reports whether e is a chain of selectors and calls built on
// an expression in exempt.
func scopeChainsOn(e ast.Expr, exempt map[ast.Node]bool) bool {
	for {
		if exempt[e] {
			return true
		}
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.SelectorExpr:
			e = x.X
		case *ast.CallExpr:
			e = x.Fun
		default:
			return false
		}
	}
}

// scopeUse is what a message in s suggests in place of the outer handle.
//
//declscope:shared // the rules phrase their messages with it
func (s txScope) scopeUse() string {
	if s.tx == nil {
		return s.unnamed
	}
	return s.tx.Name
}

// scopeQueries is a handle that a WithTx call in a scope rebinds to the
// transaction, as sqlc's q.WithTx(tx) does.
type scopeQueries struct {
	// outer is the handle WithTx is called on.
	outer handle.Path
	// use is what a message suggests: the variable the result is assigned
	// to, or the call itself.
	use string
	// call is the WithTx call. It returns its receiver's type, so it names
	// the handle too, but it is not a use.
	call *ast.CallExpr
}

// reportScopeQueries reports the uses in s of each handle that s rebinds to
// its transaction with WithTx. Code that calls q.WithTx(tx) means to run on
// the transaction, so every other use of q there bypasses it. The method
// must return the type of its receiver, as sqlc's generated one does.
//
//declscope:shared // outer_handle.go reads both kinds of scope through it
func (c *checker) reportScopeQueries(s txScope) {
	info := c.pass.TypesInfo
	if s.tx == nil {
		return
	}
	tx, _ := handle.Of(info, s.tx)
	var found []scopeQueries
	for _, n := range s.nodes {
		ast.Inspect(n, func(n ast.Node) bool {
			var q scopeQueries
			switch n := n.(type) {
			case *ast.AssignStmt:
				call, ok := scopeWithTx(info, n.Rhs, tx)
				if !ok {
					return true
				}
				q = scopeQueriesOf(info, call)
				if id, ok := n.Lhs[0].(*ast.Ident); ok && len(n.Lhs) == 1 && id.Name != "_" {
					q.use = id.Name
				}
				found = append(found, q)
				// The call on the right is this one, so it is not read
				// again.
				return false
			case *ast.CallExpr:
				call, ok := scopeWithTx(info, []ast.Expr{n}, tx)
				if !ok {
					return true
				}
				q = scopeQueriesOf(info, call)
			default:
				return true
			}
			found = append(found, q)
			return true
		})
	}
	exempt := make(map[ast.Node]bool)
	for _, q := range found {
		exempt[q.call] = true
	}
	for _, q := range found {
		c.reportScopeUses(s, q.outer, rule.OuterHandle, q.use, exempt)
	}
}

// scopeWithTx reports the call in exprs, when exprs is one call of WithTx
// with the transaction tx, on a handle, returning the handle's own type.
func scopeWithTx(info *types.Info, exprs []ast.Expr, tx handle.Path) (*ast.CallExpr, bool) {
	if len(exprs) != 1 {
		return nil, false
	}
	call, ok := ast.Unparen(exprs[0]).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, false
	}
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WithTx" {
		return nil, false
	}
	if arg, ok := handle.Of(info, call.Args[0]); !ok || !arg.Equal(tx) {
		return nil, false
	}
	if _, ok := handle.Of(info, sel.X); !ok || !types.Identical(info.TypeOf(call), info.TypeOf(sel.X)) {
		return nil, false
	}
	return call, true
}

// scopeQueriesOf is the rebound handle of a call scopeWithTx accepted. Its
// use is the call as written, until an assignment names a variable.
func scopeQueriesOf(info *types.Info, call *ast.CallExpr) scopeQueries {
	sel := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	p, _ := handle.Of(info, sel.X)
	return scopeQueries{outer: p, use: types.ExprString(call), call: call}
}
