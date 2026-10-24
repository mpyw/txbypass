// Command txbypass reports a database handle used inside a transaction it
// opened.
//
// Usage:
//
//	txbypass [flags] [packages]   analyze
//	txbypass skill install        install the authoring skill for an AI agent
//	txbypass -V=full              print the release, as go vet -vettool asks
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/mpyw/txbypass"
)

func main() {
	// Unreachable: the driver ends the process, so no test runs it. The
	// tests run the analyzer, and mainPrepare, instead.
	mainPrepare()
	singlechecker.Main(txbypass.Analyzer)
}

// mainPrepare runs both before the driver. The driver reads every non-flag
// argument as a package pattern, and it registers its own -V only when none
// is.
//
// TestMain runs it too. Intercept reads only the first argument, and a test
// binary's first argument is a -test flag, so it hands over at once.
func mainPrepare() {
	skillsInstaller.Intercept()
	registerVersionFlag()
}
