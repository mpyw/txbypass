// Package txcall knows the calls that open a transaction.
//
// There are two shapes. An invoker runs a function literal inside the
// transaction, as gorm's Transaction does. A begin returns the transaction,
// which runs until it is committed or rolled back, as database/sql's BeginTx
// does.
//
// Each is read from types, by its package path, its receiver and its name, so
// an alias or a wrapper type of the caller's does not hide it. ent generates
// its client into the caller's package, so its begin is read from the shape
// of the generated code instead.
package txcall

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/types/typeutil"
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

// known is one call that opens a transaction.
type known struct {
	pkg string
	// recv is the name of the receiver's type, with any pointer removed. It
	// is empty for a function.
	recv string
	name string
	// outer is the index of the argument that is the handle, or the
	// context. It is -1 when the handle is the receiver.
	outer int
	// callback is the index of the argument that is the callback.
	callback int
	// tx is the index of the callback's parameter that is the transaction,
	// or the context that carries it.
	tx        int
	inContext bool
	builds    bool
	nests     bool
	// session reports that the callback's context carries a session, as
	// MongoDB's WithSession gives it. A transaction on that session runs
	// with the context too.
	session bool
}

// knownInvokers lists every invoker. Each group names the version of
// its library whose signatures it was read from.
var knownInvokers = []known{
	// gorm v1.31: func (db *DB) Transaction(fc func(tx *DB) error, opts ...*sql.TxOptions) error
	{pkg: "gorm.io/gorm", recv: "DB", name: "Transaction", outer: -1, callback: 0, tx: 0, nests: true},

	// bun v1.3: RunInTx(ctx, opts, fn func(ctx context.Context, tx Tx) error) of DB, Tx
	// (a savepoint), and the IDB interface. None is read on a Conn: see dedicated.
	{pkg: "github.com/uptrace/bun", recv: "DB", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "Tx", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "IDB", name: "RunInTx", outer: -1, callback: 2, tx: 1},

	// bob: func (d DB) RunInTx(ctx, txOptions, fn func(context.Context, Transaction) error) error
	{pkg: "github.com/stephenafamo/bob", recv: "DB", name: "RunInTx", outer: -1, callback: 2, tx: 1},

	// pgx v5: BeginFunc(ctx, db, fn func(Tx) error) and BeginTxFunc(ctx, db, txOptions, fn).
	{pkg: "github.com/jackc/pgx/v5", name: "BeginFunc", outer: 1, callback: 2, tx: 0},
	{pkg: "github.com/jackc/pgx/v5", name: "BeginTxFunc", outer: 1, callback: 3, tx: 0},

	// pgx v4.18: BeginFunc(ctx, f func(Tx) error) and BeginTxFunc(ctx, txOptions, f) of Tx
	// (a savepoint) and of pgxpool's Pool.
	{pkg: "github.com/jackc/pgx/v4", recv: "Tx", name: "BeginFunc", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "BeginFunc", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "BeginTxFunc", outer: -1, callback: 2, tx: 0},

	// goqu v9: func (d *Database) WithTx(fn func(*TxDatabase) error) error
	{pkg: "github.com/doug-martin/goqu/v9", recv: "Database", name: "WithTx", outer: -1, callback: 0, tx: 0},

	// pop v6: Transaction(fn func(tx *Connection) error) and Rollback(fn func(tx *Connection)).
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "Transaction", outer: -1, callback: 0, tx: 0, nests: true},
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "Rollback", outer: -1, callback: 0, tx: 0, nests: true},

	// upper/db v4: Tx(fn func(sess Session) error) and TxContext(ctx, fn, opts) of Session.
	{pkg: "github.com/upper/db/v4", recv: "Session", name: "Tx", outer: -1, callback: 0, tx: 0},
	{pkg: "github.com/upper/db/v4", recv: "Session", name: "TxContext", outer: -1, callback: 1, tx: 0},

	// go-pg v10: RunInTransaction(ctx, fn func(*Tx) error) of DB, through its embedded
	// baseDB. Tx has one too, but it runs fn on the same transaction: go-pg opens no
	// savepoint.
	{pkg: "github.com/go-pg/pg/v10", recv: "baseDB", name: "RunInTransaction", outer: -1, callback: 1, tx: 0},

	// xorm v1.3 and v2: func (engine *Engine) Transaction(f func(*Session) (any, error)) (any, error)
	{pkg: "xorm.io/xorm", recv: "Engine", name: "Transaction", outer: -1, callback: 0, tx: 0},
	{pkg: "xorm.io/xorm/v2", recv: "Engine", name: "Transaction", outer: -1, callback: 0, tx: 0},

	// go-redis v9, and v8 under its old path: Watch(ctx, fn func(*Tx) error, keys...) and
	// TxPipelined(ctx, fn func(Pipeliner) error) of each client and of the interfaces that
	// declare them. Tx's TxPipelined is not read: inside Watch, the optimistic pattern reads
	// and watches keys on tx in it on purpose.
	{pkg: "github.com/redis/go-redis/v9", recv: "Client", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "ClusterClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Ring", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "UniversalClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Client", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Conn", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "ClusterClient", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Ring", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Cmdable", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Client", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "ClusterClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Ring", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "UniversalClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Client", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Conn", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "ClusterClient", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Ring", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Cmdable", name: "TxPipelined", outer: -1, callback: 1, tx: 0},

	// Cloud Firestore: RunTransaction(ctx, f func(context.Context, *Transaction) error, opts...)
	// The transaction reads and writes through references the client builds, as in
	// tx.Get(client.Doc("users/a")).
	{pkg: "cloud.google.com/go/firestore", recv: "Client", name: "RunTransaction", outer: -1, callback: 1, tx: 1, builds: true},
	// Cloud Spanner: ReadWriteTransaction(ctx, f func(context.Context, *ReadWriteTransaction) error),
	// and the same with options.
	{pkg: "cloud.google.com/go/spanner", recv: "Client", name: "ReadWriteTransaction", outer: -1, callback: 1, tx: 1},
	{pkg: "cloud.google.com/go/spanner", recv: "Client", name: "ReadWriteTransactionWithOptions", outer: -1, callback: 1, tx: 1},
	// Cloud Datastore: RunInTransaction(ctx, f func(tx *Transaction) error, opts...)
	{pkg: "cloud.google.com/go/datastore", recv: "Client", name: "RunInTransaction", outer: -1, callback: 1, tx: 0},

	// The rest carry the transaction in the callback's context.

	// MongoDB v2: func (s *Session) WithTransaction(ctx, fn func(ctx context.Context) (any, error), opts...)
	// and func WithSession(ctx, sess *Session, fn func(context.Context) error) error, and
	// UseSession(ctx, fn) and UseSessionWithOptions(ctx, opts, fn) of *Client.
	{pkg: "go.mongodb.org/mongo-driver/v2/mongo", recv: "Session", name: "WithTransaction", outer: 0, callback: 1, tx: 0, inContext: true},
	{pkg: "go.mongodb.org/mongo-driver/v2/mongo", name: "WithSession", outer: 0, callback: 2, tx: 0, inContext: true, session: true},
	{pkg: "go.mongodb.org/mongo-driver/v2/mongo", recv: "Client", name: "UseSession", outer: 0, callback: 1, tx: 0, inContext: true, session: true},
	{pkg: "go.mongodb.org/mongo-driver/v2/mongo", recv: "Client", name: "UseSessionWithOptions", outer: 0, callback: 2, tx: 0, inContext: true, session: true},
	// MongoDB v1.17: the same, with the Session interface and SessionContext.
	{pkg: "go.mongodb.org/mongo-driver/mongo", recv: "Session", name: "WithTransaction", outer: 0, callback: 1, tx: 0, inContext: true},
	{pkg: "go.mongodb.org/mongo-driver/mongo", name: "WithSession", outer: 0, callback: 2, tx: 0, inContext: true, session: true},
	{pkg: "go.mongodb.org/mongo-driver/mongo", recv: "Client", name: "UseSession", outer: 0, callback: 1, tx: 0, inContext: true, session: true},
	{pkg: "go.mongodb.org/mongo-driver/mongo", recv: "Client", name: "UseSessionWithOptions", outer: 0, callback: 2, tx: 0, inContext: true, session: true},
	// go-rel: Transaction(ctx, fn func(ctx context.Context) error) of the Repository interface.
	{pkg: "github.com/go-rel/rel", recv: "Repository", name: "Transaction", outer: 0, callback: 1, tx: 0, inContext: true, nests: true},
	// avito-tech/go-transaction-manager v2: Do(ctx, fn) and DoWithSettings(ctx, s, fn) of the
	// trm.Manager interface and of manager.Manager.
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2", recv: "Manager", name: "Do", outer: 0, callback: 1, tx: 0, inContext: true, nests: true},
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2", recv: "Manager", name: "DoWithSettings", outer: 0, callback: 2, tx: 0, inContext: true, nests: true},
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2/manager", recv: "Manager", name: "Do", outer: 0, callback: 1, tx: 0, inContext: true, nests: true},
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2/manager", recv: "Manager", name: "DoWithSettings", outer: 0, callback: 2, tx: 0, inContext: true, nests: true},
}

// InvokerOf reports the invoker that call is. It reports nothing for a call
// that opens no transaction, and for one whose callback is not a function
// literal: a callback held in a variable or named as a function has no body
// here to read.
func InvokerOf(info *types.Info, call *ast.CallExpr) (Invoker, bool) {
	fn, k, ok := lookup(info, call, knownInvokers)
	// The arguments may be one call with several results, as in
	// pgx.BeginFunc(args()). Then there is no literal to read.
	if !ok || len(call.Args) <= max(k.outer, k.callback) {
		return Invoker{}, false
	}
	o := Invoker{Name: displayName(fn), InContext: k.inContext, Builds: k.builds, Nests: k.nests}
	if k.outer < 0 {
		// A method expression, as in (*gorm.DB).Transaction(db, fn), takes
		// its receiver as an argument, so its arguments are shifted and its
		// selector names no handle.
		sel, ok := methodSelector(info, call)
		if !ok {
			return Invoker{}, false
		}
		if dedicated(info.TypeOf(sel.X)) || dedicated(fn.Signature().Recv().Type()) {
			return Invoker{}, false
		}
		o.Outer, o.Receiver = sel, true
	} else {
		o.Outer = call.Args[k.outer]
		if t := info.TypeOf(o.Outer); dedicated(t) || dedicatedBegin(t) {
			return Invoker{}, false
		}
	}
	lit, ok := ast.Unparen(call.Args[k.callback]).(*ast.FuncLit)
	if !ok {
		return Invoker{}, false
	}
	o.Callback = lit
	o.Tx = paramIdent(lit, k.tx)
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
	return Invoker{Name: displayName(fn), Outer: outer, Receiver: receiver, Fields: fields, Callback: lit, Tx: paramIdent(lit, s.Tx), Nests: s.Nests, InContext: s.InContext}, true
}

// DeclaredInvoker is the shape that a declaration of fn as an invoker says,
// read from its signature. It is empty, with the problem, when the
// signature does not fit.
func DeclaredInvoker(fn *types.Func) (InvokerShape, string) {
	params := declaredParams(fn.Signature())
	// The receiver is the handle, never the callback.
	first := 0
	if fn.Signature().Recv() != nil {
		first = 1
	}
	cb := slices.IndexFunc(params[first:], declaredFunc)
	if cb < 0 {
		return InvokerShape{}, "needs a parameter of a function type"
	}
	cb += first
	takes := declaredParams(params[cb].Underlying().(*types.Signature))
	s := InvokerShape{Callback: cb, Tx: slices.IndexFunc(takes, func(t types.Type) bool { return !IsContext(t) })}
	if s.Tx >= 0 {
		outer, ok := declaredOuter(fn.Signature(), params)
		if !ok {
			return InvokerShape{}, "needs a handle: a receiver, or a parameter that is neither a context nor a function"
		}
		s.Outer = outer
		return s, ""
	}
	if len(takes) == 0 {
		return InvokerShape{}, "needs a callback that takes a transaction or a context"
	}
	s.Tx, s.InContext = 0, true
	s.Outer = slices.IndexFunc(params, IsContext)
	if s.Outer < 0 {
		return InvokerShape{}, "needs a context parameter, since the callback takes only a context"
	}
	return s, ""
}

// DeclaredBegin is the shape that a declaration of fn as a begin says, or
// the problem with it.
func DeclaredBegin(fn *types.Func) (BeginShape, string) {
	res := fn.Signature().Results()
	// A transaction is a value with methods. A number or a string is none.
	if res.Len() == 0 || res.Len() > 2 || res.Len() == 2 && !types.Identical(res.At(1).Type(), types.Universe.Lookup("error").Type()) || declaredBasic(res.At(0).Type()) {
		return BeginShape{}, "needs a transaction as its first result, and nothing but an error after it"
	}
	outer, ok := declaredOuter(fn.Signature(), declaredParams(fn.Signature()))
	if !ok {
		return BeginShape{}, "needs a handle: a receiver, or a parameter that is neither a context nor a function"
	}
	return BeginShape{Outer: outer}, ""
}

// DeclaredCloser lists the parameters that a declaration of fn as an end
// says it ends: every one, the receiver of a method as 0. It is empty, with
// the problem, when fn takes nothing.
func DeclaredCloser(fn *types.Func) ([]int, string) {
	n := len(declaredParams(fn.Signature()))
	if n == 0 {
		return nil, "needs a receiver or a parameter"
	}
	params := make([]int, n)
	for i := range params {
		params[i] = i
	}
	return params, ""
}

// declaredParams lists the types of the parameters of sig, the receiver
// first.
func declaredParams(sig *types.Signature) []types.Type {
	var out []types.Type
	if sig.Recv() != nil {
		out = append(out, sig.Recv().Type())
	}
	for p := range sig.Params().Variables() {
		out = append(out, p.Type())
	}
	return out
}

// declaredOuter is the parameter that holds the handle: the receiver, or
// the first parameter that is neither a context nor a function.
func declaredOuter(sig *types.Signature, params []types.Type) (int, bool) {
	if sig.Recv() != nil {
		return 0, true
	}
	i := slices.IndexFunc(params, func(t types.Type) bool { return !IsContext(t) && !declaredFunc(t) })
	return i, i >= 0
}

// declaredBasic reports whether t is a basic type, as a number or a string.
func declaredBasic(t types.Type) bool {
	_, ok := t.Underlying().(*types.Basic)
	return ok
}

// declaredFunc reports whether t is a function type.
func declaredFunc(t types.Type) bool {
	_, ok := t.Underlying().(*types.Signature)
	return ok
}

// IsContext reports whether t is context.Context.
func IsContext(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "context" && n.Obj().Name() == "Context"
}

// BeginShape is where a helper of the project that returns a transaction
// takes what it begins the transaction on. The receiver of a method is
// parameter 0.
type BeginShape struct {
	// Outer is the parameter that holds the handle, and Fields the fields
	// selected from it, by name.
	Outer  int
	Fields []string
	// Nests is Begin.Nests, for the begin the helper opens with.
	Nests bool
}

// shapeOuter is the expression that gives the handle a call of fn opens
// on, when fn takes it at parameter i, and the fields selected from it. For
// the receiver, the expression is the selector of the call.
func shapeOuter(info *types.Info, call *ast.CallExpr, fn *types.Func, i int, names []string) (ast.Expr, bool, []*types.Var, bool) {
	sig := fn.Signature()
	if sig.Recv() != nil {
		// A method expression takes its receiver as an argument: its
		// selector names no handle, and its arguments are one place off.
		sel, ok := methodSelector(info, call)
		if !ok || i == 0 && len(names) == 0 && dedicated(info.TypeOf(sel.X)) {
			return nil, false, nil, false
		}
		if i == 0 {
			return sel, true, shapeFields(sig.Recv().Type(), names), true
		}
	}
	e, ok := shapeArg(call, fn, i)
	// One call with several results gives no handle of its own. A dedicated
	// connection, or a transaction that holds one, given as the handle
	// itself makes the transaction a savepoint on it, as it does for an
	// invoker of a library.
	if !ok || shapeTuple(info.TypeOf(e)) || len(names) == 0 && (dedicated(info.TypeOf(e)) || dedicatedBegin(info.TypeOf(e))) {
		return nil, false, nil, false
	}
	return e, false, shapeFields(shapeParam(sig, i), names), true
}

// shapeArg is the argument of a call of fn at parameter i, where the
// receiver of a method is parameter 0.
func shapeArg(call *ast.CallExpr, fn *types.Func, i int) (ast.Expr, bool) {
	if fn.Signature().Recv() != nil {
		i--
	}
	// The receiver may be the callback, when it is of a function type. The
	// arguments may be one call with several results.
	if i < 0 || i >= len(call.Args) {
		return nil, false
	}
	return call.Args[i], true
}

// shapeTuple reports whether t is the type of a call with several results.
func shapeTuple(t types.Type) bool {
	_, ok := t.(*types.Tuple)
	return ok
}

// shapeParam is the type of parameter i of sig, where the receiver of a
// method is parameter 0.
func shapeParam(sig *types.Signature, i int) types.Type {
	if sig.Recv() != nil {
		i--
	}
	return sig.Params().At(i).Type()
}

// shapeFields finds the fields named by names, selected one after another
// from a value of type t, through pointers. The helper's own analysis read
// them from a parameter of this type, so each is there.
func shapeFields(t types.Type, names []string) []*types.Var {
	var out []*types.Var
	for _, name := range names {
		if p, ok := t.Underlying().(*types.Pointer); ok {
			t = p.Elem()
		}
		for f := range t.Underlying().(*types.Struct).Fields() {
			if f.Name() == name {
				out = append(out, f)
				t = f.Type()
				break
			}
		}
	}
	return out
}

// dedicatedConns are the types whose begin or invoker runs on one
// connection, which the tables cannot tell apart by the receiver. pgx v5's
// BeginFunc takes any value that begins. go-pg's Conn begins through the
// *baseDB it embeds, as DB does. A transaction of pgx or bun opens a
// savepoint, on the connection the transaction holds. The begins of a
// transaction stay in the table of begins, so that the savepoint is a
// handle of its own: its commit ends nothing of the outer transaction.
var dedicatedConns = []known{
	{pkg: "github.com/jackc/pgx/v5", name: "Conn"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", name: "Conn"},
	{pkg: "github.com/go-pg/pg/v10", name: "Conn"},
	{pkg: "github.com/jackc/pgx/v5", name: "Tx"},
	{pkg: "github.com/jackc/pgx/v4", name: "Tx"},
	{pkg: "github.com/uptrace/bun", name: "Tx"},
}

// dedicated reports whether t is one dedicated connection, or a transaction
// that holds one. A method declared on such a type is read as one too, when
// it is promoted to a type that embeds it, as pgxmock's mock embeds pgx.Tx. A transaction begun on it runs on that connection, so a
// statement on the connection runs inside the transaction: it is no
// bypass. Begins on a connection are not listed at all.
func dedicated(t types.Type) bool {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return false
	}
	return slices.ContainsFunc(dedicatedConns, func(k known) bool {
		return k.pkg == n.Obj().Pkg().Path() && k.name == n.Obj().Name()
	})
}

// dedicatedBegin reports whether the Begin method of t, which pgx v5's
// BeginFunc calls, is declared on a dedicated type, as it is for a type
// that embeds pgx.Tx or *pgx.Conn.
func dedicatedBegin(t types.Type) bool {
	m, _, _ := types.LookupFieldOrMethod(t, true, nil, "Begin")
	fn, ok := m.(*types.Func)
	return ok && dedicated(fn.Signature().Recv().Type())
}

// endings are the names of the methods that end a transaction. go-pg's take
// a context as CommitContext and RollbackContext.
var endings = map[string]bool{"Commit": true, "Rollback": true, "CommitContext": true, "RollbackContext": true}

// Ending reports whether a method of this name ends a transaction.
func Ending(name string) bool {
	return endings[name]
}

// Holds reports whether a value of type t has a method that ends a
// transaction, directly or through a pointer to it.
func Holds(t types.Type) bool {
	// A lookup per name, rather than the method set: a handle such as
	// gorm's *DB has well over a hundred methods, and Holds is asked of
	// every argument that may be a transaction.
	for name := range endings {
		if fn, _, _ := types.LookupFieldOrMethod(t, true, nil, name); fn != nil {
			if _, ok := fn.(*types.Func); ok {
				return true
			}
		}
	}
	return false
}

// Begin is one call that returns a transaction, and the variables it is
// assigned to.
type Begin struct {
	// Name is how a message names the call, as Invoker.Name does.
	Name string
	// Outer, Receiver and Fields give the handle, as they do for an
	// Invoker.
	Outer    ast.Expr
	Receiver bool
	Fields   []*types.Var
	// Call is the call of the begin.
	Call *ast.CallExpr
	// Tx is the variable the transaction is assigned to.
	Tx *ast.Ident
	// Nests is Invoker.Nests, for a begin: gorm's Begin on a transaction
	// opens a savepoint on its connection.
	Nests bool
}

// knownBegins lists every begin. Each is a method that returns the
// transaction first, and an error second when it may fail.
var knownBegins = []known{
	// A begin on one dedicated connection, as database/sql's Conn.BeginTx, is
	// not listed: see dedicated.
	//
	// database/sql
	{pkg: "database/sql", recv: "DB", name: "Begin"},
	{pkg: "database/sql", recv: "DB", name: "BeginTx"},
	// sqlx
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "Beginx"},
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "BeginTxx"},
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "MustBegin"},
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "MustBeginTx"},
	// pgx v5 and v4: Begin and BeginTx of Tx (a savepoint), and of pgxpool's Pool.
	{pkg: "github.com/jackc/pgx/v5", recv: "Tx", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", recv: "Pool", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", recv: "Pool", name: "BeginTx"},
	{pkg: "github.com/jackc/pgx/v4", recv: "Tx", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "BeginTx"},
	// gorm: func (db *DB) Begin(opts ...*sql.TxOptions) *DB
	{pkg: "gorm.io/gorm", recv: "DB", name: "Begin", nests: true},
	// bun: Begin of DB and Tx, and BeginTx of DB, Tx, and the IDB interface. On a Tx,
	// they open a savepoint.
	{pkg: "github.com/uptrace/bun", recv: "DB", name: "Begin"},
	{pkg: "github.com/uptrace/bun", recv: "DB", name: "BeginTx"},
	{pkg: "github.com/uptrace/bun", recv: "Tx", name: "Begin"},
	{pkg: "github.com/uptrace/bun", recv: "Tx", name: "BeginTx"},
	{pkg: "github.com/uptrace/bun", recv: "IDB", name: "BeginTx"},
	// bob: Begin(ctx) and BeginTx(ctx, opts) of DB.
	{pkg: "github.com/stephenafamo/bob", recv: "DB", name: "Begin"},
	{pkg: "github.com/stephenafamo/bob", recv: "DB", name: "BeginTx"},
	// goqu
	{pkg: "github.com/doug-martin/goqu/v9", recv: "Database", name: "Begin"},
	{pkg: "github.com/doug-martin/goqu/v9", recv: "Database", name: "BeginTx"},
	// go-pg, through DB's embedded baseDB. Tx.Begin returns the same transaction, so
	// it is no begin.
	{pkg: "github.com/go-pg/pg/v10", recv: "baseDB", name: "Begin"},
	{pkg: "github.com/go-pg/pg/v10", recv: "baseDB", name: "BeginContext"},
	// pop: the transaction is a *Connection of its own.
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "NewTransaction", nests: true},
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "NewTransactionContext", nests: true},
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "NewTransactionContextOptions", nests: true},
	// Cloud Datastore
	{pkg: "cloud.google.com/go/datastore", recv: "Client", name: "NewTransaction"},
}

// BeginOf reports the begin that n assigns. n is an assignment or a
// variable declaration with a single call on its right, whose first result
// is kept in a variable. helper gives the shape of a helper of the project
// that returns a transaction.
func BeginOf(info *types.Info, n ast.Node, helper func(*types.Func) (BeginShape, bool)) (Begin, bool) {
	var lhs []*ast.Ident
	var rhs []ast.Expr
	switch n := n.(type) {
	case *ast.AssignStmt:
		for _, e := range n.Lhs {
			id, _ := e.(*ast.Ident)
			lhs = append(lhs, id)
		}
		rhs = n.Rhs
	case *ast.ValueSpec:
		lhs, rhs = n.Names, n.Values
	}
	if len(rhs) != 1 || lhs[0] == nil || lhs[0].Name == "_" {
		return Begin{}, false
	}
	call, ok := ast.Unparen(rhs[0]).(*ast.CallExpr)
	if !ok {
		return Begin{}, false
	}
	b, ok := BeginCall(info, call, helper)
	b.Tx = lhs[0]
	return b, ok
}

// BeginCall reports the begin that call is, whether its result is kept or
// not. Its Tx is nil.
func BeginCall(info *types.Info, call *ast.CallExpr, helper func(*types.Func) (BeginShape, bool)) (Begin, bool) {
	if IsBegin(info, call) {
		sel, _ := methodSelector(info, call)
		k, _ := lookupFunc(info.Selections[sel].Obj().(*types.Func), knownBegins)
		return Begin{Name: sel.Sel.Name, Outer: sel, Receiver: true, Call: call, Nests: k.nests}, true
	}
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok {
		return Begin{}, false
	}
	s, ok := helper(fn)
	if !ok {
		return Begin{}, false
	}
	outer, receiver, fields, ok := shapeOuter(info, call, fn, s.Outer, s.Fields)
	if !ok {
		return Begin{}, false
	}
	return Begin{Name: displayName(fn), Outer: outer, Receiver: receiver, Fields: fields, Call: call, Nests: s.Nests}, true
}

// IsBegin reports whether call is a call of a begin.
func IsBegin(info *types.Info, call *ast.CallExpr) bool {
	sel, ok := methodSelector(info, call)
	if !ok {
		return false
	}
	fn := info.Selections[sel].Obj().(*types.Func)
	return IsBeginFunc(fn) && !dedicated(info.TypeOf(sel.X)) && !dedicated(fn.Signature().Recv().Type())
}

// CallbackTx reports whether fn is an invoker whose callback takes
// the transaction itself as its parameter i.
func CallbackTx(fn *types.Func, i int) bool {
	k, ok := lookupFunc(fn, knownInvokers)
	return ok && !k.inContext && k.tx == i
}

// CallbackContext reports whether fn is an invoker whose callback takes, as
// its parameter i, the context that carries the transaction.
func CallbackContext(fn *types.Func, i int) bool {
	k, ok := lookupFunc(fn, knownInvokers)
	return ok && k.inContext && k.tx == i
}

// CallbackSession reports whether fn is an invoker whose callback takes, as
// its parameter i, a context that carries a session, as MongoDB's
// WithSession and UseSession do.
func CallbackSession(fn *types.Func, i int) bool {
	k, ok := lookupFunc(fn, knownInvokers)
	return ok && k.session && k.tx == i
}

// IsBeginFunc reports whether fn is a begin. A begin gives a new handle, the
// transaction, even when it has the receiver's type, as gorm's Begin does.
func IsBeginFunc(fn *types.Func) bool {
	_, known := lookupFunc(fn, knownBegins)
	return known || entBegin(fn)
}

// entBegin reports whether fn is the Tx or BeginTx method of a client that
// ent generated. ent writes the client into the caller's package, so it has
// no package path to look up. Its shape does: a method of Client that
// returns a *Tx and an error. The *Tx can be committed or rolled back, and
// its Client method gives the *Client back.
func entBegin(fn *types.Func) bool {
	if fn.Name() != "Tx" && fn.Name() != "BeginTx" || recvName(fn) != "Client" {
		return false
	}
	res := fn.Signature().Results()
	if res.Len() != 2 {
		return false
	}
	ptr, ok := res.At(0).Type().(*types.Pointer)
	if !ok || !Holds(ptr) {
		return false
	}
	back, _, _ := types.LookupFieldOrMethod(ptr, false, fn.Pkg(), "Client")
	m, ok := back.(*types.Func)
	if !ok || m.Signature().Results().Len() != 1 {
		return false
	}
	return types.Identical(m.Signature().Results().At(0).Type(), fn.Signature().Recv().Type())
}

// methodSelector is the selector of call when call calls a method on a
// value. A method expression and a function have none.
func methodSelector(info *types.Info, call *ast.CallExpr) (*ast.SelectorExpr, bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || info.Selections[sel] == nil || info.Selections[sel].Kind() != types.MethodVal {
		return nil, false
	}
	return sel, true
}

// lookup finds the entry of table that call is a call of.
func lookup(info *types.Info, call *ast.CallExpr, table []known) (*types.Func, known, bool) {
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok {
		return nil, known{}, false
	}
	k, ok := lookupFunc(fn, table)
	return fn, k, ok
}

// lookupFunc finds the entry of table for fn. A method of the universe, as
// error's, belongs to no package and has none.
func lookupFunc(fn *types.Func, table []known) (known, bool) {
	if fn.Pkg() == nil {
		return known{}, false
	}
	recv := recvName(fn)
	for _, k := range table {
		if k.pkg == fn.Pkg().Path() && k.name == fn.Name() && k.recv == recv {
			return k, true
		}
	}
	return known{}, false
}

// displayName is how a message names fn: a method by its name, a function
// by its package's name and its own.
func displayName(fn *types.Func) string {
	if fn.Signature().Recv() != nil {
		return fn.Name()
	}
	return fn.Pkg().Name() + "." + fn.Name()
}

// recvName is the name of the type a method is declared on, with any pointer
// removed. It is empty for a function.
func recvName(fn *types.Func) string {
	recv := fn.Signature().Recv()
	if recv == nil {
		return ""
	}
	t := recv.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

// paramIdent is the name of the parameter at index i of lit. It is nil when
// the parameter is unnamed or blank.
func paramIdent(lit *ast.FuncLit, i int) *ast.Ident {
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
