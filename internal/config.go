package internal

import "github.com/mpyw/txbypass/internal/rule"

// Config is what the analyzer's flags resolve to.
type Config struct {
	// On reports whether a rule is on.
	On func(rule.Name) bool
}
