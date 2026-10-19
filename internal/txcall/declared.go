package txcall

import (
	"go/types"
	"slices"
)

// DeclaredInvoker is the shape that a declaration of fn as an invoker says,
// read from its signature. It is empty, with the problem, when the
// signature does not fit.
func DeclaredInvoker(fn *types.Func) (InvokerShape, string) {
	params := declaredParams(fn.Signature())
	// The receiver is the handle, never the callback.
	first := 0
	if fn.Signature().Recv() != nil {
		first = 1
	}
	cb := slices.IndexFunc(params[first:], declaredFunc)
	if cb < 0 {
		return InvokerShape{}, "needs a parameter of a function type"
	}
	cb += first
	takes := declaredParams(params[cb].Underlying().(*types.Signature))
	// The transaction is the first input that may be one. A context, an
	// attempt number or an error beside it is none.
	s := InvokerShape{Callback: cb, Tx: slices.IndexFunc(takes, MayBeTx)}
	if s.Tx >= 0 {
		outer, ok := declaredOuter(fn.Signature(), params)
		if !ok {
			return InvokerShape{}, "needs a handle: a receiver, or a parameter that is neither a context nor a function"
		}
		s.Outer = outer
		return s, ""
	}
	if !slices.ContainsFunc(takes, IsContext) {
		return InvokerShape{}, "needs a callback that takes a transaction or a context"
	}
	// A transaction manager that carries the transaction in a context
	// joins the one its context carries, as go-transaction-manager's Do
	// does.
	s.Tx, s.InContext, s.Nests = slices.IndexFunc(takes, IsContext), true, true
	s.Outer = slices.IndexFunc(params, IsContext)
	if s.Outer < 0 {
		return InvokerShape{}, "needs a context parameter, since the callback takes only a context"
	}
	return s, ""
}

// DeclaredBegin is the shape that a declaration of fn as a begin says, or
// the problem with it.
func DeclaredBegin(fn *types.Func) (BeginShape, string) {
	res := fn.Signature().Results()
	// A number, a string, an error, a context or a function is no
	// transaction.
	if res.Len() == 0 || res.Len() > 2 || res.Len() == 2 && !types.Identical(res.At(1).Type(), types.Universe.Lookup("error").Type()) || !MayBeTx(res.At(0).Type()) {
		return BeginShape{}, "needs a transaction as its first result, and nothing but an error after it"
	}
	outer, ok := declaredOuter(fn.Signature(), declaredParams(fn.Signature()))
	if !ok {
		return BeginShape{}, "needs a handle: a receiver, or a parameter that is neither a context nor a function"
	}
	return BeginShape{Outer: outer}, ""
}

// DeclaredCloser lists the parameters that a declaration of fn as an end
// says it ends: every one, the receiver of a method as 0. It is empty, with
// the problem, when fn takes nothing.
func DeclaredCloser(fn *types.Func) ([]int, string) {
	n := len(declaredParams(fn.Signature()))
	if n == 0 {
		return nil, "needs a receiver or a parameter"
	}
	params := make([]int, n)
	for i := range params {
		params[i] = i
	}
	return params, ""
}

// declaredParams lists the types of the parameters of sig, the receiver
// first.
func declaredParams(sig *types.Signature) []types.Type {
	var out []types.Type
	if sig.Recv() != nil {
		out = append(out, sig.Recv().Type())
	}
	for p := range sig.Params().Variables() {
		out = append(out, p.Type())
	}
	return out
}

// declaredOuter is the parameter that holds the handle: the receiver, or
// the first parameter that is neither a context nor a function.
func declaredOuter(sig *types.Signature, params []types.Type) (int, bool) {
	if sig.Recv() != nil {
		return 0, true
	}
	i := slices.IndexFunc(params, func(t types.Type) bool { return !IsContext(t) && !declaredFunc(t) })
	return i, i >= 0
}

// declaredFunc reports whether t is a function type.
func declaredFunc(t types.Type) bool {
	_, ok := t.Underlying().(*types.Signature)
	return ok
}
