package main

import (
	skillembed "github.com/mpyw/go-skill-embed"

	"github.com/mpyw/txbypass"
)

// skills is the command that puts the skills where an agent reads them. It
// carries the release of this binary, so an installed copy names the version
// of the rules it describes.
//
//declscope:shared // main.go's init hands it the arguments before the driver sees them
var skills = skillembed.NewInstaller(
	txbypass.Skills,
	skillembed.WithToolName("txbypass"),
	skillembed.WithVersion(versionRelease()),
)
