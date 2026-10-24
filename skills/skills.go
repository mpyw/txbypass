// Package skills carries the skills of txbypass: txbypass-authoring, for
// writing code under txbypass.
//
// It is a package of its own, imported by the command alone, so that the
// analyzer does not depend on go-skill-embed. The skills sit in this
// directory because that is where `gh skill install` looks, which is the
// other way to reach them, and a //go:embed path cannot leave its own
// directory.
package skills

import (
	"embed"

	skillembed "github.com/mpyw/go-skill-embed"
)

// Set holds every skill in this directory.
var Set = skillembed.MustSkillsFromFS(fsys, ".")

// The pattern takes each skill's directory and leaves the files of this
// package out. Like a bare //go:embed, it drops every file whose name begins
// with a dot or an underscore. The skills hold none.
//
//go:embed */*
var fsys embed.FS
