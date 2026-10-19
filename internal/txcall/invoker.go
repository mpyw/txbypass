package txcall

import (
	"go/ast"
	"go/types"
)

// Invoker is one call that opens a transaction and runs a callback in it.
type Invoker struct {
	// Name is how a message names the call: a method by its name, a
	// function by its package's name and its own, as in pgx.BeginFunc.
	Name string
	// Outer is the expression that gives the handle the transaction is
	// opened on, or the context outside it. For a method opened on its
	// receiver, it is the selector of the call, so that a method promoted
	// from an embedded field names that field.
	Outer ast.Expr
	// Receiver reports whether Outer is the selector of the call, rather
	// than an argument.
	Receiver bool
	// Callback is the function literal that runs inside the transaction.
	Callback *ast.FuncLit
	// Tx is the callback's parameter that holds the transaction, or the
	// context that carries it. It is nil when the parameter is unnamed or
	// blank.
	Tx *ast.Ident
	// InContext reports whether the transaction rides in the callback's
	// context, as in MongoDB's WithTransaction, rather than in a value of
	// its own.
	InContext bool
	// Builds reports whether the handle also builds references that the
	// transaction takes, as Firestore's client.Doc does. Such a call does
	// no I/O, so only a chain that ends in a call taking a context runs
	// outside the transaction.
	Builds bool
	// Nests reports whether the invoker, called on a transaction, runs on the
	// transaction's connection, as gorm's Transaction opens a savepoint
	// there. upper/db's Tx begins another transaction on the pool instead,
	// and go-redis's TxPipelined queues commands that a command on the
	// handle runs before.
	Nests bool
	// Fields are the fields that a helper of the project selects from
	// Outer, as withTx opens on s.db when it is called on s. They are
	// empty for an invoker of a library.
	Fields []*types.Var
}

// InvokerOf reports the invoker that call is. It reports nothing for a call
// that opens no transaction, and for one whose callback is not a function
// literal: a callback held in a variable or named as a function has no body
// here to read.
func InvokerOf(info *types.Info, call *ast.CallExpr) (Invoker, bool) {
	fn, k, ok := lookupKnown(info, call, knownInvokers)
	// The arguments may be one call with several results, as in
	// pgx.BeginFunc(args()). Then there is no literal to read.
	if !ok || len(call.Args) <= max(k.outer, k.callback) {
		return Invoker{}, false
	}
	o := Invoker{Name: callName(fn), InContext: k.inContext, Builds: k.builds, Nests: k.nests}
	if k.outer < 0 {
		// A method expression, as in (*gorm.DB).Transaction(db, fn), takes
		// its receiver as an argument, so its arguments are shifted and its
		// selector names no handle.
		sel, ok := callSelector(info, call)
		if !ok {
			return Invoker{}, false
		}
		if onConn(info.TypeOf(sel.X)) || onConn(fn.Signature().Recv().Type()) {
			return Invoker{}, false
		}
		o.Outer, o.Receiver = sel, true
	} else {
		o.Outer = call.Args[k.outer]
		if t := info.TypeOf(o.Outer); onConn(t) || beginsOnConn(t) {
			return Invoker{}, false
		}
	}
	lit, ok := ast.Unparen(call.Args[k.callback]).(*ast.FuncLit)
	if !ok {
		return Invoker{}, false
	}
	o.Callback = lit
	o.Tx = invokerParamIdent(lit, k.tx)
	return o, true
}

// InvokerShape is where a helper of the project takes what it opens a transaction
// on, and the function it runs inside it. The receiver of a method is
// parameter 0.
type InvokerShape struct {
	// Outer is the parameter that holds the handle, and Fields the fields
	// selected from it, by name.
	Outer  int
	Fields []string
	// Callback is the parameter that is run inside the transaction, and Tx
	// the parameter of the callback that it gives the transaction.
	Callback int
	Tx       int
	// Nests is Invoker.Nests, for the invoker or begin the helper opens
	// with.
	Nests bool
	// InContext is Invoker.InContext: the callback is given a context that
	// carries the transaction, at Tx, and Outer is the invoker's context.
	InContext bool
}

// InvokerOfShape reports the invoker that call is, when it calls fn, a
// helper of shape s. As with InvokerOf, the callback has to be a function
// literal.
func InvokerOfShape(info *types.Info, call *ast.CallExpr, fn *types.Func, s InvokerShape) (Invoker, bool) {
	outer, receiver, fields, ok := shapeOuter(info, call, fn, s.Outer, s.Fields)
	if !ok {
		return Invoker{}, false
	}
	e, ok := shapeArg(call, fn, s.Callback)
	if !ok {
		return Invoker{}, false
	}
	lit, ok := ast.Unparen(e).(*ast.FuncLit)
	if !ok {
		return Invoker{}, false
	}
	return Invoker{Name: callName(fn), Outer: outer, Receiver: receiver, Fields: fields, Callback: lit, Tx: invokerParamIdent(lit, s.Tx), Nests: s.Nests, InContext: s.InContext}, true
}

// InvokerGivesTx reports whether fn is an invoker whose callback takes
// the transaction itself as its parameter i.
func InvokerGivesTx(fn *types.Func, i int) bool {
	k, ok := lookupKnownFunc(fn, knownInvokers)
	return ok && !k.inContext && k.tx == i
}

// InvokerGivesContext reports whether fn is an invoker whose callback takes, as
// its parameter i, the context that carries the transaction.
func InvokerGivesContext(fn *types.Func, i int) bool {
	k, ok := lookupKnownFunc(fn, knownInvokers)
	return ok && k.inContext && k.tx == i
}

// InvokerGivesSession reports whether fn is an invoker whose callback takes, as
// its parameter i, a context that carries a session, as MongoDB's
// WithSession and UseSession do.
func InvokerGivesSession(fn *types.Func, i int) bool {
	k, ok := lookupKnownFunc(fn, knownInvokers)
	return ok && k.session && k.tx == i
}

// invokerParamIdent is the name of the parameter at index i of lit. It is nil when
// the parameter is unnamed or blank.
func invokerParamIdent(lit *ast.FuncLit, i int) *ast.Ident {
	var names []*ast.Ident
	for _, field := range lit.Type.Params.List {
		if len(field.Names) == 0 {
			names = append(names, nil)
		}
		names = append(names, field.Names...)
	}
	if names[i] == nil || names[i].Name == "_" {
		return nil
	}
	return names[i]
}
