package txbypass_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/mpyw/txbypass"
	"github.com/mpyw/txbypass/internal/fact"
)

// No test in this file may call t.Parallel: the flags live on the global
// txbypass.Analyzer, and a test that sets one would leak it into the others.
// An analyzer that New builds has none, so new_test.go runs in parallel.

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
		"helpertx",
		"declaredlib",
		"declaredtx",
		"directives",
		"generated",
	)
}

// TestModule runs in module mode, where the module of a callee says whether
// it is a library.
func TestModule(t *testing.T) {
	analysistest.Run(t, filepath.Join(analysistest.TestData(), "mod"), txbypass.Analyzer, "example.com/app/svc")
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

// TestDeclaredFlags runs with -invokes, -begins and -closes set, which
// declare the helpers of a library flagtx calls.
func TestDeclaredFlags(t *testing.T) {
	setFlag(t, "invokes", "(flaglib.Manager).InTx,(flaglib.Store).WithTx")
	setFlag(t, "begins", "(*flaglib.Manager).Begin")
	setFlag(t, "closes", "(flaglib.Tx).Finish")
	analysistest.Run(t, analysistest.TestData(), txbypass.Analyzer, "flaglib", "flagtx")
}

// TestDeclaredFlagAddsUp keeps the names of a flag given twice.
func TestDeclaredFlagAddsUp(t *testing.T) {
	setFlag(t, "invokes", "(flaglib.Manager).InTx")
	setFlag(t, "invokes", "flaglib.Other")
	f := txbypass.Analyzer.Flags.Lookup("invokes")
	if got, want := f.Value.String(), "(flaglib.Manager).InTx,flaglib.Other"; got != want {
		t.Errorf("-invokes = %q, want %q", got, want)
	}
}

// TestDeclaredFlagRejectsMisspelling refuses a name that is not spelled like
// a function, since a typo would switch its declaration off in silence.
func TestDeclaredFlagRejectsMisspelling(t *testing.T) {
	if err := txbypass.Analyzer.Flags.Set("invokes", "InTx"); err == nil {
		t.Error("-invokes=InTx was accepted")
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

// TestFactTypes pins every fact the analyzer exports. go vet encodes facts
// by their registered types, and panics on one left out. analysistest keeps
// facts in memory, so it does not see a type missing from the list.
func TestFactTypes(t *testing.T) {
	want := []any{new(fact.Closes), new(fact.Invokes), new(fact.Begins)}
	got := make(map[reflect.Type]bool)
	for _, f := range txbypass.Analyzer.FactTypes {
		got[reflect.TypeOf(f)] = true
	}
	for _, f := range want {
		if !got[reflect.TypeOf(f)] {
			t.Errorf("FactTypes lacks %T", f)
		}
	}
	if len(got) != len(want) {
		t.Errorf("FactTypes has %d types, want %d", len(got), len(want))
	}
}
