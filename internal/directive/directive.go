// Package directive reads the //txbypass: comments in a package.
//
// //txbypass:ignore <rules> // <reason> silences the named rules, or every
// rule when none is named. On a line of its own it silences the line below.
// After code on the same line, it silences that line only. The reason is
// required, so that the next reader can tell a decision from a shortcut.
//
// //txbypass:invokes, //txbypass:begins and //txbypass:closes declare what
// a function, a method or an interface method does with a transaction. They
// go in its doc comment, or trail an interface method. Any other directive
// is reported.
package directive

import (
	"bytes"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/mpyw/txbypass/internal/rule"
)

// tool is the tool name of every directive, as in //txbypass:ignore.
// Directives follow the syntax of https://go.dev/doc/comment#directives, and
// go/ast parses them. A comment with a space after the slashes is prose.
const tool = "txbypass"

// Set is what the directives in a package say.
type Set struct {
	fset         *token.FileSet
	ignores      map[string]map[int][]*ignore
	all          []*ignore
	problems     []Problem
	declarations []Declaration
}

// Kind is what a declaring directive says a function does.
type Kind string

// The declaring directives.
const (
	// Invokes declares an invoker: it opens a transaction and runs a
	// callback inside it.
	Invokes Kind = "invokes"
	// Begins declares a begin: it returns a transaction.
	Begins Kind = "begins"
	// Closes declares an end: it ends every transaction it is given.
	Closes Kind = "closes"
)

// Declaration is one declaring directive, and the function it declares.
type Declaration struct {
	Obj  *types.Func
	Kind Kind
	Pos  token.Pos
}

// Problem is a directive that is malformed or unknown.
type Problem struct {
	Pos     token.Pos
	Message string
}

// ignore is one ignore comment, and whether it silenced anything.
type ignore struct {
	pos token.Pos
	// rules are the rules it silences. It silences every rule when empty.
	rules []rule.Name
	used  bool
}

// Scan reads the directives in files. src returns the text of a file, which
// tells a directive on a line of its own from one after code. info gives
// the function a declaring directive declares. A generated file is skipped:
// nothing is reported in it, so neither its ignores nor its problems mean
// anything.
func Scan(fset *token.FileSet, files []*ast.File, src func(filename string) ([]byte, error), info *types.Info) *Set {
	s := &Set{fset: fset, ignores: make(map[string]map[int][]*ignore)}
	for _, f := range files {
		if Generated(f) {
			continue
		}
		name := fset.PositionFor(f.Pos(), false).Filename
		text, _ := src(name)
		s.scanFile(f, name, text)
		s.scanDeclarations(f, info)
	}
	return s
}

// scanDeclarations reads the declaring directives of f, and reports one
// with an argument, or one that is not in the doc comment of a function or
// an interface method.
func (s *Set) scanDeclarations(f *ast.File, info *types.Info) {
	placed := make(map[*ast.Comment]Kind)
	for _, cg := range f.Comments {
		for _, cm := range cg.List {
			d, ok := parse(cm)
			if !ok || !declaring(d.name) {
				continue
			}
			if strings.TrimSpace(d.args) != "" {
				s.report(cm, "txbypass:"+d.name+" takes no argument; write a reason after //")
				continue
			}
			placed[cm] = ""
		}
	}
	if len(placed) == 0 {
		return
	}
	claim := func(doc *ast.CommentGroup, id *ast.Ident) {
		if doc == nil {
			return
		}
		for _, cm := range doc.List {
			if _, ok := placed[cm]; !ok {
				continue
			}
			d, _ := parse(cm)
			placed[cm] = Kind(d.name)
			s.declarations = append(s.declarations, Declaration{Obj: info.Defs[id].(*types.Func), Kind: Kind(d.name), Pos: cm.Pos()})
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			claim(n.Doc, n.Name)
		case *ast.InterfaceType:
			for _, m := range n.Methods.List {
				if len(m.Names) == 1 {
					claim(m.Doc, m.Names[0])
					claim(m.Comment, m.Names[0])
				}
			}
		}
		return true
	})
	for _, cg := range f.Comments {
		for _, cm := range cg.List {
			if kind, ok := placed[cm]; ok && kind == "" {
				d, _ := parse(cm)
				s.report(cm, "txbypass:"+d.name+" belongs in the doc comment of a function or an interface method")
			}
		}
	}
}

// declaring reports whether name is a declaring directive.
func declaring(name string) bool {
	switch Kind(name) {
	case Invokes, Begins, Closes:
		return true
	}
	return false
}

// Declarations lists the declaring directives, in the order of the files
// and of the declarations in them.
func (s *Set) Declarations() []Declaration {
	return s.declarations
}

func (s *Set) scanFile(f *ast.File, name string, text []byte) {
	// Keyed by the file and line on disk. A //line directive renames the
	// positions below it, and two of them can give two lines one number.
	lines := make(map[int][]*ignore)
	s.ignores[name] = lines
	for _, cg := range f.Comments {
		for _, cm := range cg.List {
			d, ok := parse(cm)
			if !ok {
				continue
			}
			if declaring(d.name) {
				continue
			}
			if d.name != "ignore" {
				s.report(cm, "unknown directive txbypass:"+d.name)
				continue
			}
			if !d.reason {
				s.report(cm, "txbypass:ignore needs a reason after //")
				continue
			}
			ig := &ignore{pos: cm.Pos()}
			known, named := s.readRules(cm, d.args, ig)
			// An ignore whose every rule is unknown silences nothing, and
			// is reported for its names alone.
			if named && !known {
				continue
			}
			p := s.fset.PositionFor(cm.Pos(), false)
			target := p.Line
			if onOwnLine(text, p.Offset) {
				target++
			}
			lines[target] = append(lines[target], ig)
			s.all = append(s.all, ig)
		}
	}
}

// readRules fills in the rules an ignore names, and reports the names that
// are empty or unknown. It reports whether any name is known, and whether any
// name is written at all.
func (s *Set) readRules(cm *ast.Comment, args string, ig *ignore) (known, named bool) {
	if strings.TrimSpace(args) == "" {
		return false, false
	}
	empty := false
	for _, part := range strings.Split(args, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
			empty = true
		case rule.Known(part):
			ig.rules = append(ig.rules, rule.Name(part))
			known = true
		default:
			s.report(cm, "unknown rule "+part+" in txbypass:ignore")
		}
	}
	if empty {
		s.report(cm, "empty rule name in txbypass:ignore")
	}
	return known, true
}

func (s *Set) report(cm *ast.Comment, msg string) {
	s.problems = append(s.problems, Problem{Pos: cm.Pos(), Message: msg})
}

// Generated reports whether f is a generated file, which txbypass neither
// reports in nor reads directives from. A file that cgo rewrote carries the
// marker too, but its code is the user's, so it is not one.
func Generated(f *ast.File) bool {
	if !ast.IsGenerated(f) {
		return false
	}
	for _, cg := range f.Comments {
		if cg.Pos() > f.Package {
			break
		}
		if strings.Contains(cg.Text(), "Code generated by cmd/cgo") {
			return false
		}
	}
	return true
}

// onOwnLine reports whether only blanks come before offset on its line.
func onOwnLine(text []byte, offset int) bool {
	if offset > len(text) {
		return true
	}
	start := bytes.LastIndexByte(text[:offset], '\n') + 1
	return len(bytes.TrimSpace(text[start:offset])) == 0
}

// directive is a parsed //txbypass: comment.
type directive struct {
	name, args string
	// reason reports whether a non-empty reason follows the arguments.
	reason bool
}

// parse reads cm, when it is one of this tool's directives. The reason is a
// trailing comment, so //txbypass:ignore // why and //txbypass:ignore//why both
// have one. The arguments are what sits between the name and the reason.
func parse(cm *ast.Comment) (directive, bool) {
	var d directive
	text := cm.Text
	if body, line := strings.CutPrefix(text, "//"); line {
		if before, after, found := strings.Cut(body, "//"); found {
			text = "//" + before
			d.reason = strings.TrimSpace(after) != ""
		}
	}
	pd, ok := ast.ParseDirective(cm.Slash, text)
	if !ok || pd.Tool != tool {
		return d, false
	}
	d.name, d.args = pd.Name, pd.Args
	return d, true
}

// Ignored reports whether an ignore silences rule r at pos, and records that
// it did.
func (s *Set) Ignored(pos token.Pos, r rule.Name) bool {
	p := s.fset.PositionFor(pos, false)
	silenced := false
	for _, ig := range s.ignores[p.Filename][p.Line] {
		if ig.covers(r) {
			ig.used = true
			silenced = true
		}
	}
	return silenced
}

func (ig *ignore) covers(r rule.Name) bool {
	return len(ig.rules) == 0 || slices.Contains(ig.rules, r)
}

// Unused lists the ignore comments that silenced nothing. An ignore that
// covers a rule that is off is left out: the rule did not run, so whether
// the ignore silences its report is not known. Switching a flag should not
// make it a report. A bare ignore covers every rule.
func (s *Set) Unused(on func(rule.Name) bool) []token.Pos {
	var out []token.Pos
	for _, ig := range s.all {
		if ig.used || !ig.allOn(on) {
			continue
		}
		out = append(out, ig.pos)
	}
	return out
}

func (ig *ignore) allOn(on func(rule.Name) bool) bool {
	rules := ig.rules
	if len(rules) == 0 {
		rules = rule.All
	}
	return !slices.ContainsFunc(rules, func(n rule.Name) bool { return !on(n) })
}

// Problems lists the directives that are malformed or unknown.
func (s *Set) Problems() []Problem {
	return s.problems
}
