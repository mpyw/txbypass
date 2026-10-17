// Package rule names the rules of txbypass.
//
// A rule's name is shared by its flag, its diagnostics, and the directives
// that silence it, so it is spelled once, here.
package rule

// Name is the name of a rule.
type Name string

const (
	// OuterHandle reports the handle that opened a transaction, used while
	// the transaction is open.
	OuterHandle Name = "outer-handle"
	// OuterContext reports the context outside a transaction, used inside a
	// callback whose context carries the transaction.
	OuterContext Name = "outer-context"
)

// All lists every rule, in the order the documentation gives them.
var All = []Name{OuterHandle, OuterContext}

// OnByDefault reports whether a rule is on when no flag says otherwise. Every
// rule is: what they report runs outside the transaction it is written in.
func OnByDefault(Name) bool {
	return true
}

// Known reports whether s names a rule.
func Known(s string) bool {
	for _, n := range All {
		if string(n) == s {
			return true
		}
	}
	return false
}
