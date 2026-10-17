package txbypass_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/mpyw/txbypass"
)

// No test in this file may call t.Parallel: the flags live on the global
// txbypass.Analyzer, and a test that sets one would leak it into the others.

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer,
		"gormtx",
		"buntx",
		"sqltx",
		"sqlxtx",
		"pgxtx",
		"pgx4tx",
		"bobtx",
		"goqutx",
		"poptx",
		"uppertx",
		"gopgtx",
		"xormtx",
		"redistx",
		"gcptx",
		"enttx",
		"mongotx",
		"reltx",
		"trmtx",
		"directives",
		"generated",
	)
}

// TestOuterHandleOff runs with the default flags, then with outer-handle
// off, then with the default again, to prove that the flags are read on each
// run rather than once.
func TestOuterHandleOff(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer, "gormtx")
	restore := setFlag(t, "outer-handle", "false")
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer, "outerhandleoff")
	restore()
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer, "gormtx")
}

// TestOuterContextOff does for outer-context what TestOuterHandleOff does
// for outer-handle.
func TestOuterContextOff(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer, "trmtx")
	restore := setFlag(t, "outer-context", "false")
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer, "outercontextoff")
	restore()
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer, "trmtx")
}

// TestFlags pins the name and the default of each rule's flag.
func TestFlags(t *testing.T) {
	defaults := map[string]string{
		"outer-handle":  "true",
		"outer-context": "true",
	}
	for name, want := range defaults {
		f := txbypass.Analyzer.Flags.Lookup(name)
		if f == nil {
			t.Errorf("flag %s is not defined", name)
			continue
		}
		if f.DefValue != want {
			t.Errorf("flag %s defaults to %s, want %s", name, f.DefValue, want)
		}
	}
}

// setFlag sets a flag of the analyzer. It returns a function that restores
// the default, which also runs when the test ends.
func setFlag(t *testing.T, name, value string) func() {
	t.Helper()
	f := txbypass.Analyzer.Flags.Lookup(name)
	if f == nil {
		t.Fatalf("flag %s is not defined", name)
	}
	def := f.DefValue
	if err := txbypass.Analyzer.Flags.Set(name, value); err != nil {
		t.Fatalf("setting %s=%s: %v", name, value, err)
	}
	restore := func() {
		if err := txbypass.Analyzer.Flags.Set(name, def); err != nil {
			t.Errorf("resetting %s: %v", name, err)
		}
	}
	t.Cleanup(restore)
	return restore
}
