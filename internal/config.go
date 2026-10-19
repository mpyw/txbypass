package internal

import (
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/rule"
)

// Config is what the analyzer's flags resolve to.
type Config struct {
	// On reports whether a rule is on.
	On func(rule.Name) bool
	// Declared holds, for each kind of declaration, the functions that
	// -invokes, -begins and -closes name, keyed as funcname.Key keys them.
	Declared map[directive.Kind]map[string]bool
}
