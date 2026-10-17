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
