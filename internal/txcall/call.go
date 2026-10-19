//declscope:shared // invoker.go, begin.go, shape.go and known.go read a call through it
package txcall

import (
	"go/ast"
	"go/types"
)

// callSelector is the selector of call when call calls a method on a
// value. A method expression and a function have none.
func callSelector(info *types.Info, call *ast.CallExpr) (*ast.SelectorExpr, bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || info.Selections[sel] == nil || info.Selections[sel].Kind() != types.MethodVal {
		return nil, false
	}
	return sel, true
}

// callName is how a message names fn: a method by its name, a function
// by its package's name and its own.
func callName(fn *types.Func) string {
	if fn.Signature().Recv() != nil {
		return fn.Name()
	}
	return fn.Pkg().Name() + "." + fn.Name()
}

// callRecvName is the name of the type a method is declared on, with any pointer
// removed. It is empty for a function.
func callRecvName(fn *types.Func) string {
	recv := fn.Signature().Recv()
	if recv == nil {
		return ""
	}
	t := recv.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}
