package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

// TestVersionFlagReplacesTheDrivers checks that init registered -V. The
// driver then leaves it alone, since it registers its own only when none is.
func TestVersionFlagReplacesTheDrivers(t *testing.T) {
	f := flag.Lookup("V")
	if f == nil {
		t.Fatal("-V is not registered")
	}
	if _, ok := f.Value.(versionFlag); !ok {
		t.Errorf("-V is %T, want versionFlag", f.Value)
	}
}

// TestVersionFlagPrintsTheToolID checks the line the way the go command
// parses it: "<progname> version <version> ... buildID=<id>".
func TestVersionFlagPrintsTheToolID(t *testing.T) {
	var out bytes.Buffer
	code := -1
	v := versionFlag{out: &out, exit: func(c int) { code = c }, executable: os.Executable}
	if !v.IsBoolFlag() || v.String() != "" {
		t.Error("-V must be a boolean flag with no default")
	}
	if err := v.Set("full"); err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	fields := strings.Fields(out.String())
	if len(fields) < 4 || fields[1] != "version" || fields[2] != "devel" {
		t.Errorf("-V=full printed %q", out.String())
	}
	if last := fields[len(fields)-1]; !strings.HasPrefix(last, "buildID=") || len(last) == len("buildID=") {
		t.Errorf("last field is %q, want a buildID", last)
	}
}

func TestVersionFlagRefuses(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "missing")
	cases := []struct {
		name       string
		value      string
		executable func() (string, error)
		want       string
	}{
		{"another value", "short", os.Executable, "use -V=full"},
		{"no executable", "full", func() (string, error) { return "", errors.New("no executable") }, "no executable"},
		{"a missing binary", "full", func() (string, error) { return exe, nil }, "missing"},
		{"a directory", "full", func() (string, error) { return t.TempDir(), nil }, "directory"},
	}
	for _, c := range cases {
		v := versionFlag{
			out:        &bytes.Buffer{},
			exit:       func(int) { t.Errorf("%s: exited", c.name) },
			executable: c.executable,
		}
		err := v.Set(c.value)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
}

func TestVersionString(t *testing.T) {
	info := func(v string, ok bool) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			if !ok {
				return nil, false
			}
			return &debug.BuildInfo{Main: debug.Module{Version: v}}, true
		}
	}
	cases := []struct {
		name  string
		stamp string
		info  func() (*debug.BuildInfo, bool)
		want  string
	}{
		{"a release build", "v0.2.0", info("v0.2.0+dirty", true), "0.2.0"},
		{"go install @version", "", info("v0.2.0", true), "0.2.0"},
		{"a checkout", "", info("(devel)", true), "devel"},
		{"no version", "", info("", true), "devel"},
		{"no build info", "", info("", false), "devel"},
	}
	for _, c := range cases {
		if got := versionString(c.stamp, c.info); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
