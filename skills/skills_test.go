package skills_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mpyw/txbypass/skills"
)

// TestSkillFrontmatterParses reads the frontmatter of every skill as YAML. An
// unquoted description holding ": ", as in "//txbypass:ignore comments: ...",
// is a mapping key to YAML, and the skill then fails to load or render.
// go-skill-embed reads only the name, with a reader of its own, so it does
// not catch this.
func TestSkillFrontmatterParses(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("*", "SKILL.md"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no skills found: %v", err)
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(string(body), "---\n", 3)
		if len(parts) != 3 || parts[0] != "" {
			t.Errorf("%s: no frontmatter", path)
			continue
		}
		var meta struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
			t.Errorf("%s: frontmatter is not YAML: %v", path, err)
			continue
		}
		if dir := filepath.Base(filepath.Dir(path)); meta.Name != dir {
			t.Errorf("%s: name is %q, want the directory name %q", path, meta.Name, dir)
		}
		if meta.Description == "" {
			t.Errorf("%s: no description", path)
		}
	}
}

// TestSkillsEmbedEverySkill checks that the embedded set is the directory:
// a skill added in this directory ships without another change.
func TestSkillsEmbedEverySkill(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := skills.Set.Len(); got != len(paths) {
		t.Errorf("%d skills embedded, %d in this directory", got, len(paths))
	}
}

// TestSkillsNameTheReleaseTheReadmeInstalls keeps each skill's "Written
// against" line on the release the README tells a reader to install. A
// release bumps both, and this catches the one forgotten.
func TestSkillsNameTheReleaseTheReadmeInstalls(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`github:mpyw/txbypass@([0-9.]+)`).FindSubmatch(readme)
	if m == nil {
		t.Fatal("README.md names no release to install")
	}
	want := "Written against **txbypass " + string(m[1]) + "**."
	paths, err := filepath.Glob(filepath.Join("*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), want) {
			t.Errorf("%s does not say %q", path, want)
		}
	}
}
