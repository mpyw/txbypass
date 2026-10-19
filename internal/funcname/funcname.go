// Package funcname reads the names of functions and methods that a flag
// declares, as -invokes, -begins and -closes and the golangci-lint plugin's
// settings spell them.
package funcname

import (
	"fmt"
	"go/types"
	"regexp"
	"strings"
)

// spelling is a function or a method as go/types names it:
// pkg/path.Func, or (pkg/path.Type).Method with an optional * before the
// type, and type arguments after it for a generic type.
var spelling = regexp.MustCompile(`^(\(\*?[^()*\s\[\]]+\.[\pL_][\pL\pN_]*(\[[^()\[\]]+\])?\)|[^()*\s\[\]]+)\.[\pL_][\pL\pN_]*$`)

// Parse reads a comma-separated list of names, as a flag takes it.
func Parse(list string) (map[string]bool, error) {
	return parseNames(split(list))
}

// ParseEach reads a list that holds one name per item, as the plugin's
// settings take it. An item with a comma outside brackets is refused, since
// it would otherwise pass as one name that matches nothing.
func ParseEach(names []string) (map[string]bool, error) {
	for _, s := range names {
		if len(split(s)) > 1 {
			return nil, fmt.Errorf("%q holds more than one name; give one per item", s)
		}
	}
	return parseNames(names)
}

// parseNames reads each name. The * of a pointer receiver is dropped, so that
// either spelling names the method whatever its receiver is. A name that is
// not spelled like a function is refused, since a typo would otherwise switch
// its declaration off in silence.
func parseNames(names []string) (map[string]bool, error) {
	m := make(map[string]bool)
	for _, s := range names {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !spelling.MatchString(s) {
			return nil, fmt.Errorf("%q is not spelled pkg/path.Func or (pkg/path.Type).Method", s)
		}
		m[strings.Replace(s, "(*", "(", 1)] = true
	}
	return m, nil
}

// split splits a list at the commas outside brackets, so that the type
// arguments of (pkg.Map[K, V]).Put stay in one name.
func split(list string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range list {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, list[start:i])
				start = i + 1
			}
		}
	}
	return append(out, list[start:])
}

// Key is how a set from Parse names fn: its full name, with the * of a
// pointer receiver dropped. A method of a generic type is named by its
// origin, with the type parameters.
func Key(fn *types.Func) string {
	return strings.Replace(fn.Origin().FullName(), "(*", "(", 1)
}
