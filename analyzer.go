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
	"github.com/mpyw/txbypass/internal/rule"
)

// Analyzer reports a database handle used inside a transaction it opened.
var Analyzer = &analysis.Analyzer{
	Name:     "txbypass",
	Doc:      "reports a database handle used inside a transaction it opened",
	URL:      "https://github.com/mpyw/txbypass",
	Requires: []*analysis.Analyzer{ctrlflow.Analyzer},
	// The fact says which parameters a function ends, so that a helper of
	// another package that commits a transaction is seen.
	FactTypes: []analysis.Fact{new(closer.Fact)},
	Run:       runAnalyzer,
}

// analyzerFlagOf holds the flag of each rule.
var analyzerFlagOf = make(map[rule.Name]*bool)

func init() {
	for _, r := range rule.All {
		analyzerFlagOf[r] = Analyzer.Flags.Bool(string(r), rule.OnByDefault(r), "report "+string(r))
	}
}

func runAnalyzer(pass *analysis.Pass) (any, error) {
	return internal.Run(pass, internal.Config{On: func(r rule.Name) bool { return *analyzerFlagOf[r] }})
}
