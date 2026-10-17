package internal

import (
	"errors"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestRunWithoutCtrlflow(t *testing.T) {
	if _, err := Run(&analysis.Pass{ResultOf: map[*analysis.Analyzer]any{}}, Config{}); !errors.Is(err, errRunWithoutCtrlflow) {
		t.Errorf("Run() error = %v, want %v", err, errRunWithoutCtrlflow)
	}
}

// TestRunModule covers the drivers that give no module: go vet for a package
// outside any, as the standard library, and nogo always.
func TestRunModule(t *testing.T) {
	if got := runModule(nil); got != "" {
		t.Errorf("runModule(nil) = %q, want empty", got)
	}
	if got := runModule(&analysis.Module{Path: "example.com/app"}); got != "example.com/app" {
		t.Errorf("runModule() = %q, want example.com/app", got)
	}
}
