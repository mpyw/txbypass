package internal

import (
	"cmp"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/mpyw/txbypass/internal/closer"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/funcname"
	"github.com/mpyw/txbypass/internal/handle"
	"github.com/mpyw/txbypass/internal/helper"
	"github.com/mpyw/txbypass/internal/program"
	"github.com/mpyw/txbypass/internal/rule"
	"github.com/mpyw/txbypass/internal/txcall"
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
	// holdsDeclared remembers, for each type asked about, whether a method
	// of it is declared to end a transaction. methodSets builds the method
	// sets it reads.
	//
	//declscope:private
	holdsDeclared typeutil.Map
	// txTypes holds the types of the transactions that declared helpers the
	// package calls give or take. txTypesRead reports that they were read.
	//
	//declscope:private
	txTypes typeutil.Map
	//declscope:private
	txTypesRead bool
	//declscope:private
	methodSets typeutil.MethodSetCache
	// keys holds the name a flag would give each function asked about,
	// since every call is asked about and formatting a name is costly.
	//declscope:private
	keys map[*types.Func]string
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
		keys:       make(map[*types.Func]string),
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

// holds reports whether a value of type t is a transaction: it has a method
// that ends one, by its name, as Commit, or by a declaration, here or in
// the package that declares the method, that takes no other transaction.
//
//declscope:shared // run.go hands it to closer and helper, and asks it of signatures
func (c *checker) holds(t types.Type) bool {
	if txcall.Holds(t) || c.declaredTx(t) {
		return true
	}
	if v := c.holdsDeclared.At(t); v != nil {
		return v.(bool)
	}
	found := false
	for _, sel := range typeutil.IntuitiveMethodSet(t, &c.methodSets) {
		m := sel.Obj().(*types.Func)
		var f closer.Fact
		ends := c.declares(directive.KindCloses, m) || m.Pkg() != c.pass.Pkg && c.pass.ImportObjectFact(m.Origin(), &f) && slices.Contains(f.Params, 0)
		// A declared end of the receiver alone, as tx.Done(err), makes
		// the receiver a transaction. One given a value that may be a
		// transaction, as m.End(tx), ends that one: the receiver is what
		// ends it.
		if ends && !slices.ContainsFunc(checkerParamTypes(m), checkerTxLike) {
			found = true
			break
		}
	}
	c.holdsDeclared.Set(t, found)
	return found
}

// begins reports whether fn is a begin: one of a library, one a directive
// or a flag declares, or a helper of another package. A begin that returns
// a value of its receiver's type gives the transaction, not the receiver.
//
//declscope:shared // run.go hands it to handle
func (c *checker) begins(fn *types.Func) bool {
	if txcall.IsBeginFunc(fn) {
		return true
	}
	// A flag's declaration that the signature does not fit declares
	// nothing, as it does for the helpers.
	if c.declares(directive.KindBegins, fn) {
		if _, problem := txcall.DeclaredBegin(fn.Origin()); problem == "" {
			return true
		}
	}
	return fn.Pkg() != c.pass.Pkg && c.pass.ImportObjectFact(fn.Origin(), new(helper.Begins))
}

// checkerParamTypes lists the types of the parameters of m, not its
// receiver.
func checkerParamTypes(m *types.Func) []types.Type {
	var out []types.Type
	for p := range m.Signature().Params().Variables() {
		out = append(out, p.Type())
	}
	return out
}

// declaredTx reports whether t is the type of a transaction that a declared
// helper the package calls gives or takes: the first result of a begin,
// or a parameter of an end other than its receiver. Such a type may have
// no method at all, as an opaque handle that only its manager ends.
func (c *checker) declaredTx(t types.Type) bool {
	if !c.txTypesRead {
		c.txTypesRead = true
		for _, obj := range c.pass.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok {
				c.readTxTypes(fn)
			}
		}
	}
	return c.txTypes.At(t) != nil
}

// readTxTypes records the types of the transactions fn gives or takes, when
// it is a declared begin or end, or an end of another package.
func (c *checker) readTxTypes(fn *types.Func) {
	sig := fn.Signature()
	if !txcall.IsBeginFunc(fn) && c.begins(fn) && sig.Results().Len() > 0 {
		c.txTypes.Set(sig.Results().At(0).Type(), true)
	}
	var f closer.Fact
	declared := c.declares(directive.KindCloses, fn)
	if !declared && (fn.Pkg() == c.pass.Pkg || !c.pass.ImportObjectFact(fn.Origin(), &f)) {
		return
	}
	first := 0
	if sig.Recv() != nil {
		first = 1
	}
	for i := range sig.Params().Len() {
		pt := sig.Params().At(i).Type()
		if (declared || slices.Contains(f.Params, first+i)) && checkerTxLike(pt) {
			c.txTypes.Set(pt, true)
		}
	}
}

// checkerTxLike reports whether a value of type t may be a transaction: it
// is not a number, a string, an error, a context or a function.
func checkerTxLike(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic, *types.Signature:
		return false
	case *types.Interface:
		return !types.Identical(t, types.Universe.Lookup("error").Type()) && !txcall.IsContext(t) && u.NumMethods() > 0
	}
	return true
}

// declares reports whether fn is declared as kind: by a directive of the
// package, or by a flag.
//
//declscope:shared // run.go hands it to closer and helper
func (c *checker) declares(kind directive.Kind, fn *types.Func) bool {
	fn = fn.Origin()
	if c.declared[kind][fn] {
		return true
	}
	// Key formats a name, and every call is asked about: no flag, no name.
	names := c.cfg.Declared[kind]
	if len(names) == 0 {
		return false
	}
	// Keyed by the origin, as funcname.Key reads it, so the instances of a
	// generic function share one name.
	key, ok := c.keys[fn]
	if !ok {
		key = funcname.Key(fn)
		c.keys[fn] = key
	}
	return names[key]
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
