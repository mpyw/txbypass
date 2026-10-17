package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

// version is the release this binary was built from, written at link time
// with -X main.version=<version>. .goreleaser.yaml passes it. It is empty in
// every other build.
//
// The module version the go command records is not enough for a release.
// goreleaser builds every platform into dist/ in one checkout, so each build
// after the first sees untracked files and records the version as +dirty.
var version string

// registerVersionFlag claims -V before the driver does. x/tools registers a
// -V that prints "devel" for every binary, but only when no -V is registered
// yet, so registering this one first replaces it.
//
//declscope:shared // main.go's init registers it before the driver runs
func registerVersionFlag() {
	flag.Var(versionFlag{out: os.Stdout, exit: os.Exit, executable: os.Executable}, "V", "print version and exit")
}

// versionFlag is the -V protocol that `go vet` speaks to a -vettool. Its
// output, its exit and how it finds this binary are fields, so a test can
// reach every branch without ending the process.
type versionFlag struct {
	out        io.Writer
	exit       func(int)
	executable func() (string, error)
}

func (versionFlag) IsBoolFlag() bool { return true }
func (versionFlag) String() string   { return "" }

// Set prints the line the go command reads to identify the tool, and exits.
//
// The shape is the one x/tools prints, with the release in place of its
// "devel". The go command reads the third field as the version. When it is
// not "devel", it takes the whole line as the tool's identity, so the
// buildID stays on it. That ties the identity to this exact binary, not only
// to a version that two builds could share.
func (f versionFlag) Set(s string) error {
	if s != "full" {
		return fmt.Errorf("unsupported flag value: -V=%s (use -V=full)", s)
	}
	progname, err := f.executable()
	if err != nil {
		return err
	}
	id, err := buildIDForVersion(progname)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(f.out, "%s version %s comments-go-here buildID=%s\n", progname, versionRelease(), id)
	f.exit(0)
	return nil
}

// versionRelease is the release this binary was built from, as -V=full
// prints it.
//
//declscope:shared // skills.go stamps an installed skill with the same release
func versionRelease() string {
	return versionString(version, debug.ReadBuildInfo)
}

// versionString is the release, without its leading v, or "devel". The
// stamp comes first. Then the module version the go command records, which
// is what `go install <pkg>@<v>` gives. A build of a checkout has neither.
func versionString(stamp string, buildInfo func() (*debug.BuildInfo, bool)) string {
	if stamp != "" {
		return strings.TrimPrefix(stamp, "v")
	}
	if info, ok := buildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "devel"
}

// buildIDForVersion hashes the binary, as x/tools does. The go command falls
// back to it when the version is "devel", and a build of a checkout has no
// other identity.
func buildIDForVersion(progname string) (string, error) {
	f, err := os.Open(progname)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%02x", h.Sum(nil)), nil
}
