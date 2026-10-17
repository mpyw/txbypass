// Package region finds where a transaction from a begin is open.
//
// A transaction from database/sql's BeginTx, or any other begin, is open from
// the begin until it is committed or rolled back. Inside reads the control
// flow graph of the function that holds the begin. A node is inside when the
// transaction is open there on every path from the function's entry: the
// begin comes before it, and no commit, rollback or new assignment of the
// transaction comes between.
//
// The analysis asks "on every path", so a branch that might have committed
// leaves the code after it outside. The branch on the begin's own error is
// outside too: the transaction did not begin there. That is the "if err !=
// nil" right after the begin, or the one the begin is the init of. A later
// check of the same variable checks another error.
package region

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/cfg"

	"github.com/mpyw/txbypass/internal/handle"
)

// closers are the names of the methods that end a transaction. go-pg's take
// a context as CommitContext and RollbackContext.
var closers = map[string]bool{"Commit": true, "Rollback": true, "CommitContext": true, "RollbackContext": true}

// Inside lists the nodes of body where the transaction that begin assigns to
// tx is open on every path. err is the begin's error, or nil. A deferred
// statement is left out: it runs when the function returns.
func Inside(info *types.Info, body *ast.BlockStmt, begin ast.Node, tx, err types.Object) []ast.Node {
	g := cfg.New(body, func(*ast.CallExpr) bool { return true })
	a := &analysis{info: info, begin: begin, tx: handle.Var(tx), txObj: tx, err: err, check: checkAfter(body, begin)}
	preds := make(map[*cfg.Block][]*cfg.Block)
	for _, b := range g.Blocks {
		for _, s := range b.Succs {
			preds[s] = append(preds[s], b)
		}
	}
	// open[b] is whether the transaction is open on entry to b, on every
	// path. It starts true everywhere but the entry, and only ever falls.
	open := make(map[*cfg.Block]bool)
	for _, b := range g.Blocks[1:] {
		open[b] = true
	}
	for changed := true; changed; {
		changed = false
		for _, b := range g.Blocks[1:] {
			in := a.entry(b, preds[b], open)
			if in != open[b] {
				open[b] = in
				changed = true
			}
		}
	}
	var out []ast.Node
	for _, b := range g.Blocks {
		if !b.Live {
			continue
		}
		state := open[b]
		for _, n := range b.Nodes {
			if _, deferred := n.(*ast.DeferStmt); state && !deferred {
				out = append(out, n)
			}
			state = a.step(n, state)
		}
	}
	return out
}

// analysis is what one begin's region is computed from.
type analysis struct {
	info  *types.Info
	begin ast.Node
	tx    handle.Path
	txObj types.Object
	err   types.Object
	// check is the if statement that checks the begin's error, or nil.
	check *ast.IfStmt
}

// checkAfter finds the if statement that checks begin's error: the one whose
// init is begin, or the one right after begin in its block.
func checkAfter(body *ast.BlockStmt, begin ast.Node) *ast.IfStmt {
	var found *ast.IfStmt
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt:
			if n.Init == begin {
				found = n
			}
		case *ast.BlockStmt:
			for i, s := range n.List[:max(len(n.List)-1, 0)] {
				if checkHolds(s, begin) {
					found, _ = n.List[i+1].(*ast.IfStmt)
				}
			}
		}
		return found == nil
	})
	return found
}

// checkHolds reports whether s is begin, or declares it.
func checkHolds(s ast.Stmt, begin ast.Node) bool {
	if s == begin {
		return true
	}
	decl, ok := s.(*ast.DeclStmt)
	if !ok {
		return false
	}
	// A DeclStmt always holds a GenDecl.
	gen := decl.Decl.(*ast.GenDecl)
	return slices.ContainsFunc(gen.Specs, func(sp ast.Spec) bool { return sp == begin })
}

// entry is whether the transaction is open on entry to b, from what its
// predecessors leave. A block no path reaches stays open, so it does not
// close the blocks after it.
func (a *analysis) entry(b *cfg.Block, preds []*cfg.Block, open map[*cfg.Block]bool) bool {
	if a.failed(b) {
		return false
	}
	in := true
	for _, p := range preds {
		state := open[p]
		for _, n := range p.Nodes {
			state = a.step(n, state)
		}
		in = in && state
	}
	return in
}

// failed reports whether b is the branch where the begin's error is set, so
// the transaction did not begin: the body of "if err != nil", or the else of
// "if err == nil".
func (a *analysis) failed(b *cfg.Block) bool {
	s, ok := b.Stmt.(*ast.IfStmt)
	if !ok || a.err == nil || s != a.check {
		return false
	}
	cond, ok := ast.Unparen(s.Cond).(*ast.BinaryExpr)
	if !ok || !a.isErr(cond.X) && !a.isErr(cond.Y) {
		return false
	}
	switch b.Kind {
	case cfg.KindIfThen:
		return cond.Op == token.NEQ
	case cfg.KindIfElse:
		return cond.Op == token.EQL
	}
	return false
}

func (a *analysis) isErr(e ast.Expr) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	return ok && a.info.Uses[id] == a.err
}

// step is whether the transaction is open after n, given whether it was
// open before.
func (a *analysis) step(n ast.Node, open bool) bool {
	if n == a.begin {
		return true
	}
	if !open {
		return false
	}
	return !a.ends(n)
}

// ends reports whether n commits or rolls back the transaction, or assigns
// another value to its variable. A function literal inside n is not read: it
// does not run where it is written. A deferred call does not run there
// either.
func (a *analysis) ends(n ast.Node) bool {
	if _, deferred := n.(*ast.DeferStmt); deferred {
		return false
	}
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			for _, e := range n.Lhs {
				if id, ok := e.(*ast.Ident); ok && a.info.ObjectOf(id) == a.txObj {
					found = true
				}
			}
		case *ast.CallExpr:
			sel, ok := ast.Unparen(n.Fun).(*ast.SelectorExpr)
			if !ok || !closers[sel.Sel.Name] {
				return true
			}
			if p, ok := handle.Of(a.info, sel.X); ok && p.Within(a.tx) {
				found = true
			}
		}
		return !found
	})
	return found
}
