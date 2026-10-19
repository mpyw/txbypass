package txcall

import "go/types"

// MayBeTx reports whether a value of type t may be a transaction: it is not
// a number, a string, a bool, an error, a context or a function.
func MayBeTx(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Basic, *types.Signature:
		return false
	case *types.Interface:
		return !types.Identical(t, types.Universe.Lookup("error").Type()) && !IsContext(t)
	}
	return true
}

// HoldsTx reports whether a value of type t has a method that ends a
// transaction, directly or through a pointer to it.
func HoldsTx(t types.Type) bool {
	// A lookup per name, rather than the method set: a handle such as
	// gorm's *DB has well over a hundred methods, and Holds is asked of
	// every argument that may be a transaction.
	for name := range endings {
		if fn, _, _ := types.LookupFieldOrMethod(t, true, nil, name); fn != nil {
			if _, ok := fn.(*types.Func); ok {
				return true
			}
		}
	}
	return false
}
