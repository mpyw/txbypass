// Package declared knows what a pass is told about transactions beyond the
// tables of libraries: the helpers that a directive or a flag declares,
// and the types whose values are transactions.
//
// A directive is seen only in the package that holds it, so its
// declaration is exported as a fact, which the analysis of a package that
// imports it reads. A flag is seen by every pass alike, so its names are
// matched where a function is asked about.
//
// A value is a transaction when its type has a method that ends one, by
// its name, as Commit. A type a declared begin returns is one too, and so
// is the type of the first parameter of a declared end that may be one.
// Such a type may have no method at all, as an opaque handle that only its
// manager ends.
package declared

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/fact"
	"github.com/mpyw/txbypass/internal/funcname"
	"github.com/mpyw/txbypass/internal/txcall"
)

// Set answers what is declared, and what is a transaction, for one pass.
type Set struct {
	pass *analysis.Pass
	// flags holds, for each kind, the functions a flag names, keyed as
	// funcname.Key keys them.
	flags map[directive.Kind]map[string]bool
	// local holds the functions of the package that a directive declares,
	// by kind, once the declaration fits the signature.
	local map[directive.Kind]map[*types.Func]bool
	// keys holds the name a flag would give each function asked about,
	// since every call is asked about and formatting a name is costly.
	keys map[*types.Func]string
	// holds remembers, for each type asked about, whether it is a
	// transaction. methodSets builds the method sets it reads.
	holds      typeutil.Map
	methodSets typeutil.MethodSetCache
	// txTypes holds the types of the transactions that declared helpers the
	// package calls or defines give or take. txTypesRead reports that they
	// were read.
	txTypes     typeutil.Map
	txTypesRead bool
}

// Problem is a declaring directive that its signature does not fit. It
// declares nothing.
type Problem struct {
	Pos     token.Pos
	Message string
}

// New is the Set of pass. flags holds the functions a flag names, by kind.
// Each of decls is checked against the signature it declares. One that
// fits is exported as a fact. One that does not is a problem.
func New(pass *analysis.Pass, flags map[directive.Kind]map[string]bool, decls []directive.Declaration) (*Set, []Problem) {
	s := &Set{pass: pass, flags: flags, local: make(map[directive.Kind]map[*types.Func]bool), keys: make(map[*types.Func]string)}
	var problems []Problem
	for _, d := range decls {
		var f analysis.Fact
		var problem string
		switch d.Kind {
		case directive.KindInvokes:
			var shape txcall.InvokerShape
			shape, problem = txcall.DeclaredInvoker(d.Obj)
			f = &fact.Invokes{Shape: shape}
		case directive.KindBegins:
			var shape txcall.BeginShape
			shape, problem = txcall.DeclaredBegin(d.Obj)
			f = &fact.Begins{Shape: shape}
		case directive.KindCloses:
			var params []int
			params, problem = txcall.DeclaredCloser(d.Obj)
			f = &fact.Closes{Params: params, Declared: true}
		}
		if problem != "" {
			problems = append(problems, Problem{Pos: d.Pos, Message: "txbypass:" + string(d.Kind) + " " + problem})
			continue
		}
		if s.local[d.Kind] == nil {
			s.local[d.Kind] = make(map[*types.Func]bool)
		}
		s.local[d.Kind][d.Obj] = true
		pass.ExportObjectFact(d.Obj, f)
	}
	return s, problems
}

// Declares reports whether fn is declared as kind: by a directive of the
// package, or by a flag.
func (s *Set) Declares(kind directive.Kind, fn *types.Func) bool {
	fn = fn.Origin()
	if s.local[kind][fn] {
		return true
	}
	// Key formats a name, and every call is asked about: no flag, no name.
	names := s.flags[kind]
	if len(names) == 0 {
		return false
	}
	// Keyed by the origin, as funcname.Key reads it, so the instances of a
	// generic function share one name.
	key, ok := s.keys[fn]
	if !ok {
		key = funcname.Key(fn)
		s.keys[fn] = key
	}
	return names[key]
}

// Calls reports whether call calls a function declared as an invoker or a
// begin.
func (s *Set) Calls(call *ast.CallExpr) bool {
	fn, ok := typeutil.Callee(s.pass.TypesInfo, call).(*types.Func)
	return ok && (s.Declares(directive.KindInvokes, fn) || s.Declares(directive.KindBegins, fn))
}

// Begins reports whether fn is a begin: one of a library, one a directive
// or a flag declares, or a helper of another package. A begin that returns
// a value of its receiver's type gives the transaction, not the receiver.
func (s *Set) Begins(fn *types.Func) bool {
	if txcall.IsBeginFunc(fn) {
		return true
	}
	// A flag's declaration that the signature does not fit declares
	// nothing, as it does for the helpers.
	if s.Declares(directive.KindBegins, fn) {
		if _, problem := txcall.DeclaredBegin(fn.Origin()); problem == "" {
			return true
		}
	}
	return fn.Pkg() != s.pass.Pkg && s.pass.ImportObjectFact(fn.Origin(), new(fact.Begins))
}

// Holds reports whether a value of type t is a transaction: it has a method
// that ends one, by its name, as Commit, or by a declaration, here or in
// the package that declares the method, that takes no other transaction.
// A type that a declared helper gives or takes as its transaction is one
// too.
func (s *Set) Holds(t types.Type) bool {
	if v := s.holds.At(t); v != nil {
		return v.(bool)
	}
	found := txcall.Holds(t) || s.declaredTx(t) || s.endsReceiver(t)
	s.holds.Set(t, found)
	return found
}

// endsReceiver reports whether a value of type t has a method declared, here
// or in the package that declares the method, to end its receiver alone.
func (s *Set) endsReceiver(t types.Type) bool {
	for _, sel := range typeutil.IntuitiveMethodSet(t, &s.methodSets) {
		m := sel.Obj().(*types.Func)
		var f fact.Closes
		ends := s.Declares(directive.KindCloses, m) || m.Pkg() != s.pass.Pkg && s.pass.ImportObjectFact(m.Origin(), &f) && slices.Contains(f.Params, 0)
		// A declared end of the receiver alone, as tx.Done(err), makes
		// the receiver a transaction. One given a value that may be a
		// transaction, as m.End(tx) or m.End(v any), ends that one: the
		// receiver is what ends it.
		if ends && !takesTx(m) {
			return true
		}
	}
	return false
}

// takesTx reports whether m takes a parameter, not its receiver, that may
// be a transaction.
func takesTx(m *types.Func) bool {
	for p := range m.Signature().Params().Variables() {
		if txcall.MayBeTx(p.Type()) {
			return true
		}
	}
	return false
}

// declaredTx reports whether t is the type of a transaction that a declared
// helper the package calls or defines gives or takes: the first result of a
// begin, or a parameter of an end other than its receiver.
func (s *Set) declaredTx(t types.Type) bool {
	if !s.txTypesRead {
		s.txTypesRead = true
		// A declaration of the package counts where it is defined, so a
		// helper beside it that takes its transaction is read even before
		// anything calls it. A function is named by many identifiers, and
		// is read once.
		read := make(map[*types.Func]bool)
		var fns []*types.Func
		for _, m := range []map[*ast.Ident]types.Object{s.pass.TypesInfo.Uses, s.pass.TypesInfo.Defs} {
			for _, obj := range m {
				if fn, ok := obj.(*types.Func); ok && !read[fn] {
					read[fn] = true
					fns = append(fns, fn)
				}
			}
		}
		// The begins come first: a declared end of a transaction they
		// return takes its receiver as the transaction, not an input.
		for _, fn := range fns {
			s.readBeginTxType(fn)
		}
		// The ends are read against the begins alone, and recorded once
		// all are read. A type one end takes then never turns another's
		// receiver into a transaction, whatever order the map gives.
		var ends []types.Type
		for _, fn := range fns {
			ends = append(ends, s.readEndTxTypes(fn)...)
		}
		for _, t := range ends {
			s.txTypes.Set(t, true)
		}
	}
	// A transaction may be held as T or as *T, whichever the begin returns.
	return s.txTypes.At(t) != nil || s.txTypes.At(alt(t)) != nil
}

// alt is *T for T, and T for *T.
func alt(t types.Type) types.Type {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		return p.Elem()
	}
	return types.NewPointer(t)
}

// txRecv reports whether a method on a receiver of type t ends its
// receiver: t has a method that ends one by its name, or a begin returns
// it. A method on the value or on the pointer counts either way, as a
// begin of *T with an end on T.
func (s *Set) txRecv(t types.Type) bool {
	return txcall.Holds(t) || s.txTypes.At(t) != nil || s.txTypes.At(alt(t)) != nil
}

// readBeginTxType records the type of the transaction fn returns, when it
// is a declared begin or a begin of another package.
func (s *Set) readBeginTxType(fn *types.Func) {
	sig := fn.Signature()
	// A begin that returns any names no type, as an end that takes it does.
	if !txcall.IsBeginFunc(fn) && s.Begins(fn) && sig.Results().Len() > 0 && txLike(sig.Results().At(0).Type()) {
		s.txTypes.Set(sig.Results().At(0).Type(), true)
	}
}

// readEndTxTypes lists the type of the transaction a declared end, or an
// end of another package, takes. A method of a transaction, one with an
// ending method or one a begin returns, ends its receiver: its inputs,
// as options, are no transaction types.
func (s *Set) readEndTxTypes(fn *types.Func) []types.Type {
	sig := fn.Signature()
	if recv := sig.Recv(); recv != nil && s.txRecv(recv.Type()) {
		return nil
	}
	var f fact.Closes
	declared := s.Declares(directive.KindCloses, fn)
	if !declared && (fn.Pkg() == s.pass.Pkg || !s.pass.ImportObjectFact(fn.Origin(), &f)) {
		return nil
	}
	// A directive of another package declares every input too.
	declared = declared || f.Declared
	first := 0
	if sig.Recv() != nil {
		first = 1
	}
	var out []types.Type
	for i := range sig.Params().Len() {
		pt := sig.Params().At(i).Type()
		if (declared || slices.Contains(f.Params, first+i)) && txLike(pt) {
			out = append(out, pt)
			// A declaration ends every input, but only its first that may
			// be a transaction is one, as tx in End(tx, log). An input
			// after it, as a logger, is no transaction type.
			if declared {
				return out
			}
		}
	}
	return out
}

// txLike reports whether t is a type of transaction that a declared end may
// take: one that may be a transaction, and not the empty interface, which
// names no type. A named interface with no method may be an opaque handle.
func txLike(t types.Type) bool {
	if i, ok := types.Unalias(t).(*types.Interface); ok && i.Empty() {
		return false
	}
	return txcall.MayBeTx(t)
}
