package internal

import (
	"cmp"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/mpyw/txbypass/internal/closer"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/funcname"
	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/helper"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/rule"
)

// checker is one pass's state.
//
//declscope:shared
type checker struct {
	pass *analysis.Pass
	// directives is what the package's //txbypass: comments say.
	directives *directive.Set
	// prog is the package's SSA form.
	prog *program.Program
	// closer says what may end a transaction.
	closer *closer.Closer
	// paths says which handle each SSA value is.
	paths *handle.Paths
	// helpers says which calls are invokers: a library's, and
	// the helpers of the project.
	helpers *helper.Helpers
	// declared holds the functions of the package that a directive
	// declares, by kind, once the declaration fits the signature.
	declared map[directive.Kind]map[*types.Func]bool
	//declscope:private
	cfg Config
	// generated holds the files marked as generated, by name. Nothing is
	// reported in them.
	//
	//declscope:private
	generated map[string]bool
	// diagnostics holds the reports until the pass ends, when they are
	// reported in the order of their positions.
	//
	//declscope:private
	diagnostics []analysis.Diagnostic
	// reported holds the positions already reported for each rule. A use
	// inside nested transactions opened on one handle is reported once.
	//
	//declscope:private
	reported map[rule.Name]map[token.Pos]bool
}

//declscope:shared
func newChecker(pass *analysis.Pass, cfg Config) *checker {
	c := &checker{
		pass:       pass,
		cfg:        cfg,
		directives: directive.Scan(pass.Fset, pass.Files, pass.ReadFile, pass.TypesInfo),
		declared:   make(map[directive.Kind]map[*types.Func]bool),
		generated:  make(map[string]bool),
		reported:   make(map[rule.Name]map[token.Pos]bool),
	}
	for _, f := range pass.Files {
		if directive.Generated(f) {
			c.generated[pass.Fset.PositionFor(f.Pos(), false).Filename] = true
		}
	}
	return c
}

// declares reports whether fn is declared as kind: by a directive of the
// package, or by a flag.
//
//declscope:shared // run.go hands it to closer and helper
func (c *checker) declares(kind directive.Kind, fn *types.Func) bool {
	if c.declared[kind][fn.Origin()] {
		return true
	}
	// Key formats a name, and every call is asked about: no flag, no name.
	names := c.cfg.Declared[kind]
	return len(names) > 0 && names[funcname.Key(fn)]
}

// report reports msg at pos for rule r, unless r is off, an ignore silences
// it there, or r was already reported there.
//
//declscope:shared
func (c *checker) report(pos token.Pos, r rule.Name, msg string) {
	if !c.cfg.On(r) || c.reported[r][pos] {
		return
	}
	if c.reported[r] == nil {
		c.reported[r] = make(map[token.Pos]bool)
	}
	c.reported[r][pos] = true
	if c.directives.Ignored(pos, r) {
		return
	}
	c.diagnostics = append(c.diagnostics, analysis.Diagnostic{Pos: pos, Message: msg + " [" + string(r) + "]"})
}

// inGenerated reports whether pos is in a file marked as generated.
//
//declscope:shared // run.go skips the files
func (c *checker) inGenerated(pos token.Pos) bool {
	return c.generated[c.pass.Fset.PositionFor(pos, false).Filename]
}

// reportf reports a message that belongs to no rule, such as a problem with
// a directive. No ignore silences it.
//
//declscope:shared
func (c *checker) reportf(pos token.Pos, msg string) {
	c.diagnostics = append(c.diagnostics, analysis.Diagnostic{Pos: pos, Message: msg})
}

// flush reports what the pass found, in the order of the positions.
//
//declscope:shared
func (c *checker) flush() {
	slices.SortStableFunc(c.diagnostics, func(a, b analysis.Diagnostic) int { return cmp.Compare(a.Pos, b.Pos) })
	for _, d := range c.diagnostics {
		c.pass.Report(d)
	}
}
