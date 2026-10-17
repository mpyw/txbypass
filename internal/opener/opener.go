// Package opener knows the calls that open a transaction.
//
// There are two shapes. A callback opener runs a function literal inside the
// transaction, as gorm's Transaction does. A begin returns the transaction,
// which runs until it is committed or rolled back, as database/sql's BeginTx
// does.
//
// Each is read from types, by its package path, its receiver and its name, so
// an alias or a wrapper type of the caller's does not hide it. ent generates
// its client into the caller's package, so its begin is read from the shape
// of the generated code instead.
package opener

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/types/typeutil"
)

// Opener is one call that opens a transaction and runs a callback in it.
type Opener struct {
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
	// Nests reports whether the opener, called on a transaction, runs on the
	// transaction's connection, as gorm's Transaction opens a savepoint
	// there. upper/db's Tx begins another transaction on the pool instead,
	// and go-redis's TxPipelined queues commands that a command on the
	// handle runs before.
	Nests bool
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

// knownOpeners lists every callback opener. Each group names the version of
// its library whose signatures it was read from.
var knownOpeners = []known{
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

// Of reports the callback opener that call is. It reports nothing for a call
// that opens no transaction, and for one whose callback is not a function
// literal: a callback held in a variable or named as a function has no body
// here to read.
func Of(info *types.Info, call *ast.CallExpr) (Opener, bool) {
	fn, k, ok := lookup(info, call, knownOpeners)
	// The arguments may be one call with several results, as in
	// pgx.BeginFunc(args()). Then there is no literal to read.
	if !ok || len(call.Args) <= max(k.outer, k.callback) {
		return Opener{}, false
	}
	o := Opener{Name: displayName(fn), InContext: k.inContext, Builds: k.builds, Nests: k.nests}
	if k.outer < 0 {
		// A method expression, as in (*gorm.DB).Transaction(db, fn), takes
		// its receiver as an argument, so its arguments are shifted and its
		// selector names no handle.
		sel, ok := methodSelector(info, call)
		if !ok {
			return Opener{}, false
		}
		if dedicated(info.TypeOf(sel.X)) || dedicated(fn.Signature().Recv().Type()) {
			return Opener{}, false
		}
		o.Outer, o.Receiver = sel, true
	} else {
		o.Outer = call.Args[k.outer]
		if t := info.TypeOf(o.Outer); dedicated(t) || dedicatedBegin(t) {
			return Opener{}, false
		}
	}
	lit, ok := ast.Unparen(call.Args[k.callback]).(*ast.FuncLit)
	if !ok {
		return Opener{}, false
	}
	o.Callback = lit
	o.Tx = paramIdent(lit, k.tx)
	return o, true
}

// dedicatedConns are the types whose begin or opener runs on one
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
	// Name is how a message names the call, as Opener.Name does.
	Name string
	// Outer is the selector of the call, which names the handle.
	Outer *ast.SelectorExpr
	// Call is the call of the begin.
	Call *ast.CallExpr
	// Tx is the variable the transaction is assigned to.
	Tx *ast.Ident
	// Nests is Opener.Nests, for a begin: gorm's Begin on a transaction
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
// is kept in a variable.
func BeginOf(info *types.Info, n ast.Node) (Begin, bool) {
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
	if !IsBegin(info, call) {
		return Begin{}, false
	}
	sel, _ := methodSelector(info, call)
	k, _ := lookupFunc(info.Selections[sel].Obj().(*types.Func), knownBegins)
	b := Begin{Name: sel.Sel.Name, Outer: sel, Call: call, Tx: lhs[0], Nests: k.nests}
	return b, true
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

// CallbackTx reports whether fn is a callback opener whose callback takes
// the transaction itself as its parameter i.
func CallbackTx(fn *types.Func, i int) bool {
	k, ok := lookupFunc(fn, knownOpeners)
	return ok && !k.inContext && k.tx == i
}

// CallbackContext reports whether fn is an opener whose callback takes, as
// its parameter i, the context that carries the transaction.
func CallbackContext(fn *types.Func, i int) bool {
	k, ok := lookupFunc(fn, knownOpeners)
	return ok && k.inContext && k.tx == i
}

// CallbackSession reports whether fn is an opener whose callback takes, as
// its parameter i, a context that carries a session, as MongoDB's
// WithSession and UseSession do.
func CallbackSession(fn *types.Func, i int) bool {
	k, ok := lookupFunc(fn, knownOpeners)
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
