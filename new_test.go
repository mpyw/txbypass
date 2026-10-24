package txbypass_test

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/mpyw/txbypass"
)

// TestNew runs analyzers that New builds, side by side with different
// settings, to prove that they share nothing with each other or with the
// flags of txbypass.Analyzer.
func TestNew(t *testing.T) {
	t.Parallel()
	off := mustNew(t, txbypass.Config{Rules: map[string]bool{"outer-handle": false}})
	def := mustNew(t, txbypass.Config{})
	if off.Flags.Lookup("outer-handle") != nil {
		t.Error("an analyzer New built has flags, want none")
	}
	analysistest.Run(t, analysistest.TestData(), off, "outerhandleoff")
	analysistest.Run(t, analysistest.TestData(), def, "gormtx")
}

// TestNewDeclares runs with Invokes, Begins and Closes set, as
// TestDeclaredFlags does with the flags.
func TestNewDeclares(t *testing.T) {
	t.Parallel()
	a := mustNew(t, txbypass.Config{
		Invokes: []string{"(flaglib.Manager).InTx", "(flaglib.Store).WithTx"},
		Begins:  []string{"(*flaglib.Manager).Begin"},
		Closes:  []string{"(flaglib.Tx).Finish"},
	})
	analysistest.Run(t, analysistest.TestData(), a, "flaglib", "flagtx")
}

func TestNewRejectsBadConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  txbypass.Config
		want string
	}{
		{"an unknown rule", txbypass.Config{Rules: map[string]bool{"outer": true}}, `unknown rule "outer"`},
		{"a name not spelled like a function", txbypass.Config{Closes: []string{"Finish"}}, "closes: "},
		{"two names in one item", txbypass.Config{Invokes: []string{"a.B, c.D"}}, "invokes: "},
	}
	for _, tt := range tests {
		if _, err := txbypass.New(tt.cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got error %v, want one containing %q", tt.name, err, tt.want)
		}
	}
}

func mustNew(t *testing.T, cfg txbypass.Config) *analysis.Analyzer {
	t.Helper()
	a, err := txbypass.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
