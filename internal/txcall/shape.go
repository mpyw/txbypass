package txcall

import (
	"go/ast"
	"go/types"
)

// shapeOuter is the expression that gives the handle a call of fn opens
// on, when fn takes it at parameter i, and the fields selected from it. For
// the receiver, the expression is the selector of the call.
//
//declscope:shared // invoker.go and begin.go find the handle a helper is called with
func shapeOuter(info *types.Info, call *ast.CallExpr, fn *types.Func, i int, names []string) (ast.Expr, bool, []*types.Var, bool) {
	sig := fn.Signature()
	if sig.Recv() != nil {
		// A method expression takes its receiver as an argument: its
		// selector names no handle, and its arguments are one place off.
		sel, ok := callSelector(info, call)
		if !ok || i == 0 && len(names) == 0 && onConn(info.TypeOf(sel.X)) {
			return nil, false, nil, false
		}
		if i == 0 {
			return sel, true, shapeFields(sig.Recv().Type(), names), true
		}
	}
	e, ok := shapeArg(call, fn, i)
	// One call with several results gives no handle of its own. One
	// connection, or a transaction that holds one, given as the handle
	// itself makes the transaction run on it, or a savepoint on it, as it
	// does for an invoker of a library.
	if !ok || shapeTuple(info.TypeOf(e)) || len(names) == 0 && (onConn(info.TypeOf(e)) || beginsOnConn(info.TypeOf(e))) {
		return nil, false, nil, false
	}
	return e, false, shapeFields(shapeParam(sig, i), names), true
}

// shapeArg is the argument of a call of fn at parameter i, where the
// receiver of a method is parameter 0.
//
//declscope:shared // invoker.go finds the callback a helper is called with
func shapeArg(call *ast.CallExpr, fn *types.Func, i int) (ast.Expr, bool) {
	if fn.Signature().Recv() != nil {
		i--
	}
	// The receiver may be the callback, when it is of a function type. The
	// arguments may be one call with several results.
	if i < 0 || i >= len(call.Args) {
		return nil, false
	}
	return call.Args[i], true
}

// shapeTuple reports whether t is the type of a call with several results.
func shapeTuple(t types.Type) bool {
	_, ok := t.(*types.Tuple)
	return ok
}

// shapeParam is the type of parameter i of sig, where the receiver of a
// method is parameter 0.
func shapeParam(sig *types.Signature, i int) types.Type {
	if sig.Recv() != nil {
		i--
	}
	return sig.Params().At(i).Type()
}

// shapeFields finds the fields named by names, selected one after another
// from a value of type t, through pointers. The helper's own analysis read
// them from a parameter of this type, so each is there.
func shapeFields(t types.Type, names []string) []*types.Var {
	var out []*types.Var
	for _, name := range names {
		if p, ok := t.Underlying().(*types.Pointer); ok {
			t = p.Elem()
		}
		for f := range t.Underlying().(*types.Struct).Fields() {
			if f.Name() == name {
				out = append(out, f)
				t = f.Type()
				break
			}
		}
	}
	return out
}
