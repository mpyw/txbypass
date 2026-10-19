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
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"

	"github.com/mpyw/txbypass/internal"
	"github.com/mpyw/txbypass/internal/closer"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/funcname"
	"github.com/mpyw/txbypass/internal/helper"
	"github.com/mpyw/txbypass/internal/rule"
)

// Analyzer reports a database handle used inside a transaction it opened.
var Analyzer = &analysis.Analyzer{
	Name:     "txbypass",
	Doc:      "reports a database handle used inside a transaction it opened",
	URL:      "https://github.com/mpyw/txbypass",
	Requires: []*analysis.Analyzer{ctrlflow.Analyzer},
	// The facts say which parameters a function ends, and which function
	// is an invoker or a begin, so that a helper of another package is
	// seen. Every fact type is listed here, or go vet cannot encode it.
	FactTypes: []analysis.Fact{new(closer.Fact), new(helper.Invokes), new(helper.Begins)},
	Run:       runAnalyzer,
}

// analyzerFlagOf holds the flag of each rule.
var analyzerFlagOf = make(map[rule.Name]*bool)

// analyzerDeclared holds -invokes, -begins and -closes.
var analyzerDeclared = map[directive.Kind]*analyzerNames{
	directive.Invokes: {}, directive.Begins: {}, directive.Closes: {},
}

func init() {
	for _, r := range rule.All {
		analyzerFlagOf[r] = Analyzer.Flags.Bool(string(r), rule.OnByDefault(r), "report "+string(r))
	}
	usage := map[directive.Kind]string{
		directive.Invokes: "functions that open a transaction and run a callback in it",
		directive.Begins:  "functions that return a transaction they begin",
		directive.Closes:  "functions that end every transaction they are given",
	}
	for _, k := range []directive.Kind{directive.Invokes, directive.Begins, directive.Closes} {
		Analyzer.Flags.Var(analyzerDeclared[k], string(k),
			"comma-separated "+usage[k]+", spelled pkg/path.Func or (pkg/path.Type).Method")
	}
}

func runAnalyzer(pass *analysis.Pass) (any, error) {
	declared := make(map[directive.Kind]map[string]bool, len(analyzerDeclared))
	for k, f := range analyzerDeclared {
		declared[k] = f.names
	}
	return internal.Run(pass, internal.Config{On: func(r rule.Name) bool { return *analyzerFlagOf[r] }, Declared: declared})
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

// Set parses a comma-separated list of names.
func (f *analyzerNames) Set(v string) error {
	m, err := funcname.Parse(v)
	if err != nil {
		return err
	}
	f.raw, f.names = v, m
	return nil
}
