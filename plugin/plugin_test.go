package plugin_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	_ "github.com/mpyw/txbypass/plugin"
)

// testdata gives the module's fixtures, shared with the analyzer's tests.
func testdata(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPluginDefaults(t *testing.T) {
	a := buildAnalyzer(t, nil)
	analysistest.Run(t, testdata(t), a, "gormtx")
}

func TestPluginSettingsTurnRulesOn(t *testing.T) {
	a := buildAnalyzer(t, map[string]any{"outer-handle": true})
	analysistest.Run(t, testdata(t), a, "gormtx")
}

func TestPluginSettingsTurnRulesOff(t *testing.T) {
	a := buildAnalyzer(t, map[string]any{"outer-handle": false})
	analysistest.Run(t, testdata(t), a, "outerhandleoff")
}

func TestPluginSettingsDeclareHelpers(t *testing.T) {
	a := buildAnalyzer(t, map[string]any{
		"invokes": []any{"(flaglib.Manager).InTx"},
		"begins":  []any{"(*flaglib.Manager).Begin"},
		"closes":  []any{"(flaglib.Tx).Finish"},
	})
	analysistest.Run(t, testdata(t), a, "flagtx")
}

func TestPluginRejectsBadSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings any
		want     string
	}{
		{"an unknown rule", map[string]any{"outer": true}, `unknown rule "outer"`},
		{"a value that is not a bool", map[string]any{"outer-handle": "yes"}, `rule "outer-handle" in settings takes true or false`},
		{"a declaration that is not a list", map[string]any{"invokes": "flaglib.InTx"}, "invokes in settings: takes a list of functions"},
		{"a declaration with an item that is not a string", map[string]any{"begins": []any{1}}, "begins in settings: takes a list of functions"},
		{"a name not spelled like a function", map[string]any{"closes": []any{"Finish"}}, "closes in settings: "},
		{"two names in one item", map[string]any{"closes": []any{"a.B, c.D"}}, "holds more than one name"},
		{"settings that are not a map", []any{"outer-handle"}, "decoding settings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newPlugin(t)(tt.settings)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got error %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestPluginLoadMode(t *testing.T) {
	p, err := newPlugin(t)(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.GetLoadMode(); got != register.LoadModeTypesInfo {
		t.Errorf("load mode %q, want %q", got, register.LoadModeTypesInfo)
	}
}

// newPlugin finds the constructor the package registered.
func newPlugin(t *testing.T) register.NewPlugin {
	t.Helper()
	np, err := register.GetPlugin("txbypass")
	if err != nil {
		t.Fatal(err)
	}
	return np
}

// buildAnalyzer builds the plugin's one analyzer from settings.
func buildAnalyzer(t *testing.T, settings any) *analysis.Analyzer {
	t.Helper()
	p, err := newPlugin(t)(settings)
	if err != nil {
		t.Fatal(err)
	}
	as, err := p.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	if len(as) != 1 {
		t.Fatalf("got %d analyzers, want 1", len(as))
	}
	return as[0]
}
