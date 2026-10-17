package txbypass

import (
	"embed"

	skillembed "github.com/mpyw/go-skill-embed"
)

// Skills are the skills this module carries: txbypass-authoring, for writing
// code under txbypass.
//
// It is declared here rather than beside the command because a //go:embed
// path cannot leave its own directory, and skills/ sits at the repository
// root. It sits there because that is where `gh skill install` looks, which
// is the other way to reach it.
var Skills = skillembed.MustSkillsFromFS(skillsFS, "skills")

// The skills hold no file whose name begins with a dot or an underscore, so
// the bare form is enough. all:skills is what to write when one does.
//
//go:embed skills
var skillsFS embed.FS
