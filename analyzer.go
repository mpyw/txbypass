// Package txbypass reports a database handle used inside a transaction it
// opened.
//
// A callback that runs inside a transaction receives the transaction. A
// statement run through the handle the transaction was opened on runs
// outside it instead: it is not rolled back with the transaction, and it may
// wait on a lock the transaction holds. The rules are listed in
// design/rules.md, and each has a flag of its name.
package txbypass

import (
	"fmt"
	"maps"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"

	"github.com/mpyw/txbypass/internal"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/fact"
	"github.com/mpyw/txbypass/internal/funcname"
	"github.com/mpyw/txbypass/internal/rule"
)

// Analyzer reports a database handle used inside a transaction it opened. It
// takes its settings from flags, as the txbypass command and go vet give
// them. A program that holds its own settings builds an analyzer with New.
var Analyzer = analyzerWithFlags()

// Config holds the settings of an analyzer that New builds. The zero Config
// keeps every rule at its default and declares no function.
type Config struct {
	// Rules turns a rule on or off by its name, such as "outer-handle". A
	// rule left out keeps its default.
	Rules map[string]bool
	// Invokes names functions that open a transaction and run a callback in
	// it, each spelled pkg/path.Func or (pkg/path.Type).Method.
	Invokes []string
	// Begins names functions that return a transaction they begin.
	Begins []string
	// Closes names functions that end every transaction they are given.
	Closes []string
}

// New builds an analyzer with the settings of cfg, and no flags. Each call
// builds one of its own, so two of them share nothing. It fails on a rule
// it does not know, and on a name that is not spelled like a function.
func New(cfg Config) (*analysis.Analyzer, error) {
	on := make(map[rule.Name]bool, len(cfg.Rules))
	for name, v := range cfg.Rules {
		if !rule.Known(name) {
			return nil, fmt.Errorf("unknown rule %q", name)
		}
		on[rule.Name(name)] = v
	}
	given := map[directive.Kind][]string{
		directive.KindInvokes: cfg.Invokes,
		directive.KindBegins:  cfg.Begins,
		directive.KindCloses:  cfg.Closes,
	}
	declared := make(map[directive.Kind]map[string]bool, len(given))
	for _, k := range directive.Kinds {
		if len(given[k]) == 0 {
			continue
		}
		names, err := funcname.ParseEach(given[k])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		declared[k] = names
	}
	c := internal.Config{
		On: func(r rule.Name) bool {
			if v, ok := on[r]; ok {
				return v
			}
			return rule.OnByDefault(r)
		},
		Declared: declared,
	}
	return analyzerBuild(func() internal.Config { return c }), nil
}

// analyzerWithFlags builds the analyzer with a flag for each rule and for
// each kind of declaration. The flags live in the closure of Run, so nothing
// is shared but the analyzer itself.
func analyzerWithFlags() *analysis.Analyzer {
	on := make(map[rule.Name]*bool, len(rule.All))
	names := make(map[directive.Kind]*analyzerNames, len(directive.Kinds))
	a := analyzerBuild(func() internal.Config {
		declared := make(map[directive.Kind]map[string]bool, len(names))
		for k, f := range names {
			declared[k] = f.names
		}
		return internal.Config{On: func(r rule.Name) bool { return *on[r] }, Declared: declared}
	})
	for _, r := range rule.All {
		on[r] = a.Flags.Bool(string(r), rule.OnByDefault(r), "report "+string(r))
	}
	usage := map[directive.Kind]string{
		directive.KindInvokes: "functions that open a transaction and run a callback in it",
		directive.KindBegins:  "functions that return a transaction they begin",
		directive.KindCloses:  "functions that end every transaction they are given",
	}
	for _, k := range directive.Kinds {
		names[k] = new(analyzerNames)
		a.Flags.Var(names[k], string(k),
			"comma-separated "+usage[k]+", spelled pkg/path.Func or (pkg/path.Type).Method")
	}
	return a
}

// analyzerBuild builds an analyzer that runs with the settings config gives
// when a pass starts.
func analyzerBuild(config func() internal.Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "txbypass",
		Doc:      "reports a database handle used inside a transaction it opened",
		URL:      "https://github.com/mpyw/txbypass",
		Requires: []*analysis.Analyzer{ctrlflow.Analyzer},
		// The facts say which parameters a function ends, and which function
		// is an invoker or a begin, so that a helper of another package is
		// seen. Every fact type is listed here, or go vet cannot encode it.
		FactTypes: []analysis.Fact{new(fact.Closes), new(fact.Invokes), new(fact.Begins)},
		Run: func(pass *analysis.Pass) (any, error) {
			return internal.Run(pass, config())
		},
	}
}

// analyzerNames is a flag that takes a list of functions. A name that is
// not spelled like a function is refused when the flag is parsed, since a
// typo would otherwise switch its declaration off in silence.
type analyzerNames struct {
	raw   string
	names map[string]bool
}

// String returns the flag as it was set.
func (f *analyzerNames) String() string { return f.raw }

// Set parses a comma-separated list of names. A flag given twice declares
// the names of both. An empty value clears it, as resetting the flag to its
// default does.
func (f *analyzerNames) Set(v string) error {
	if v == "" {
		f.raw, f.names = "", nil
		return nil
	}
	m, err := funcname.Parse(v)
	if err != nil {
		return err
	}
	if f.names == nil {
		f.raw, f.names = v, m
		return nil
	}
	f.raw += "," + v
	maps.Copy(f.names, m)
	return nil
}
