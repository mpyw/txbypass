// Package rule names the rules of txbypass.
//
// A rule's name is shared by its flag, its diagnostics, and the directives
// that silence it, so it is spelled once, here.
package rule

// Name is the name of a rule.
type Name string

// OuterHandle reports the handle that opened a transaction, used inside the
// callback that runs in it.
const OuterHandle Name = "outer-handle"

// All lists every rule, in the order the documentation gives them.
var All = []Name{OuterHandle}

// OnByDefault reports whether a rule is on when no flag says otherwise. Every
// rule is: a use of the outer handle inside the transaction is never what
// the callback means.
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
