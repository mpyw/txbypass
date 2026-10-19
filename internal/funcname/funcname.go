// Package funcname reads the names of functions and methods that a flag
// declares, as -invokes, -begins and -closes and the golangci-lint plugin's
// settings spell them.
package funcname

import (
	"fmt"
	"go/types"
	"regexp"
	"slices"
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
// settings take it. An item with two names, split by a comma outside
// brackets, is refused, since it would otherwise pass as one name that
// matches nothing. A trailing comma, as in "a.F,", leaves one name.
func ParseEach(names []string) (map[string]bool, error) {
	var each []string
	for _, s := range names {
		parts := slices.DeleteFunc(split(s), func(p string) bool { return strings.TrimSpace(p) == "" })
		if len(parts) > 1 {
			return nil, fmt.Errorf("%q holds more than one name; give one per item", s)
		}
		each = append(each, parts...)
	}
	return parseNames(each)
}

// parseNames reads each name. The * of a pointer receiver and the type
// arguments of a generic one are dropped, so that any spelling names the
// method whatever its receiver is. A name that is
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
		m[normalize(s)] = true
	}
	return m, nil
}

// typeArgs is the type arguments of a receiver.
var typeArgs = regexp.MustCompile(`\[[^\[\]]*\]`)

// normalize drops the * of a pointer receiver and the type arguments of a
// generic one. A method declares its own names for the type parameters, and
// go/types writes them with a space after each comma, so a name that kept
// them would match only when spelled exactly as the method's receiver.
func normalize(name string) string {
	return typeArgs.ReplaceAllString(strings.Replace(name, "(*", "(", 1), "")
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
// pointer receiver and the type parameters of a generic one dropped.
func Key(fn *types.Func) string {
	return normalize(fn.Origin().FullName())
}
