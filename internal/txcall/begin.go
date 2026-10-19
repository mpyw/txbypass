package txcall

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

// Begin is one call that returns a transaction, and the variables it is
// assigned to.
type Begin struct {
	// Name is how a message names the call, as Invoker.Name does.
	Name string
	// Outer, Receiver and Fields give the handle, as they do for an
	// Invoker.
	Outer    ast.Expr
	Receiver bool
	Fields   []*types.Var
	// Call is the call of the begin.
	Call *ast.CallExpr
	// Tx is the variable the transaction is assigned to.
	Tx *ast.Ident
	// Nests is Invoker.Nests, for a begin: gorm's Begin on a transaction
	// opens a savepoint on its connection.
	Nests bool
}

// BeginShape is where a helper of the project that returns a transaction
// takes what it begins the transaction on. The receiver of a method is
// parameter 0.
type BeginShape struct {
	// Outer is the parameter that holds the handle, and Fields the fields
	// selected from it, by name.
	Outer  int
	Fields []string
	// Nests is Begin.Nests, for the begin the helper opens with.
	Nests bool
}

// BeginOf reports the begin that n assigns. n is an assignment or a
// variable declaration with a single call on its right, whose first result
// is kept in a variable. helper gives the shape of a helper of the project
// that returns a transaction.
func BeginOf(info *types.Info, n ast.Node, helper func(*types.Func) (BeginShape, bool)) (Begin, bool) {
	var lhs []*ast.Ident
	var rhs []ast.Expr
	switch n := n.(type) {
	case *ast.AssignStmt:
		for _, e := range n.Lhs {
			id, _ := e.(*ast.Ident)
			lhs = append(lhs, id)
		}
		rhs = n.Rhs
	case *ast.ValueSpec:
		lhs, rhs = n.Names, n.Values
	}
	if len(rhs) != 1 || lhs[0] == nil || lhs[0].Name == "_" {
		return Begin{}, false
	}
	call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr)
	if !ok {
		return Begin{}, false
	}
	b, ok := BeginCall(info, call, helper)
	b.Tx = lhs[0]
	return b, ok
}

// BeginCall reports the begin that call is, whether its result is kept or
// not. Its Tx is nil.
func BeginCall(info *types.Info, call *ast.CallExpr, helper func(*types.Func) (BeginShape, bool)) (Begin, bool) {
	if IsBegin(info, call) {
		sel, _ := callSelector(info, call)
		k, _ := lookupKnownFunc(info.Selections[sel].Obj().(*types.Func), knownBegins)
		return Begin{Name: sel.Sel.Name, Outer: sel, Receiver: true, Call: call, Nests: k.nests}, true
	}
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok {
		return Begin{}, false
	}
	s, ok := helper(fn)
	if !ok {
		return Begin{}, false
	}
	outer, receiver, fields, ok := shapeOuter(info, call, fn, s.Outer, s.Fields)
	if !ok {
		return Begin{}, false
	}
	return Begin{Name: callName(fn), Outer: outer, Receiver: receiver, Fields: fields, Call: call, Nests: s.Nests}, true
}

// IsBegin reports whether call is a call of a begin.
func IsBegin(info *types.Info, call *ast.CallExpr) bool {
	sel, ok := callSelector(info, call)
	if !ok {
		return false
	}
	fn := info.Selections[sel].Obj().(*types.Func)
	return IsBeginFunc(fn) && !onConn(info.TypeOf(sel.X)) && !onConn(fn.Signature().Recv().Type())
}

// IsBeginFunc reports whether fn is a begin. A begin gives a new handle, the
// transaction, even when it has the receiver's type, as gorm's Begin does.
func IsBeginFunc(fn *types.Func) bool {
	_, known := lookupKnownFunc(fn, knownBegins)
	return known || entBegin(fn)
}

// entBegin reports whether fn is the Tx or BeginTx method of a client that
// ent generated. ent writes the client into the caller's package, so it has
// no package path to look up. Its shape does: a method of Client that
// returns a *Tx and an error. The *Tx can be committed or rolled back, and
// its Client method gives the *Client back.
func entBegin(fn *types.Func) bool {
	if fn.Name() != "Tx" && fn.Name() != "BeginTx" || callRecvName(fn) != "Client" {
		return false
	}
	res := fn.Signature().Results()
	if res.Len() != 2 {
		return false
	}
	ptr, ok := res.At(0).Type().(*types.Pointer)
	if !ok || !HoldsTx(ptr) {
		return false
	}
	back, _, _ := types.LookupFieldOrMethod(ptr, false, fn.Pkg(), "Client")
	m, ok := back.(*types.Func)
	if !ok || m.Signature().Results().Len() != 1 {
		return false
	}
	return types.Identical(m.Signature().Results().At(0).Type(), fn.Signature().Recv().Type())
}
