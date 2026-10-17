package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillInstall installs the skills into a directory, as `txbypass skill
// install --dir` does, and checks that the copy is the skill and is stamped
// with this tool.
func TestSkillInstall(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "skills")
	if err := skills.Run(t.Context(), []string{"install", "--dir", dest}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "txbypass-authoring", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# Writing code under txbypass") {
		t.Errorf("the installed manifest is not the skill:\n%.200s", body)
	}
	if !strings.Contains(string(body), "x-embedded-by: txbypass") {
		t.Errorf("the installed manifest carries no stamp:\n%.400s", body)
	}
}
