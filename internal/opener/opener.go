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

	"golang.org/x/tools/go/types/typeutil"
)

// Opener is one call that opens a transaction and runs a callback in it.
type Opener struct {
	// Name is how a message names the call: a method by its name, a
	// function by its package's name and its own, as in pgx.BeginFunc.
	Name string
	// Outer is the expression that gives the handle the transaction is
	// opened on. For a method, it is the selector of the call, so that a
	// method promoted from an embedded field names that field. For an
	// opener whose transaction rides in a context, it is the context
	// given to the call.
	Outer ast.Expr
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
}

// knownOpeners lists every callback opener. Each group names the version of
// its library whose signatures it was read from.
var knownOpeners = []known{
	// gorm v1.31: func (db *DB) Transaction(fc func(tx *DB) error, opts ...*sql.TxOptions) error
	{pkg: "gorm.io/gorm", recv: "DB", name: "Transaction", outer: -1, callback: 0, tx: 0},

	// bun v1.3: RunInTx(ctx, opts, fn func(ctx context.Context, tx Tx) error) of DB, Conn,
	// Tx (a savepoint), and the IDB interface they share.
	{pkg: "github.com/uptrace/bun", recv: "DB", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "Conn", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "Tx", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "IDB", name: "RunInTx", outer: -1, callback: 2, tx: 1},

	// bob: func (d DB) RunInTx(ctx, txOptions, fn func(context.Context, Transaction) error) error
	{pkg: "github.com/stephenafamo/bob", recv: "DB", name: "RunInTx", outer: -1, callback: 2, tx: 1},

	// pgx v5: BeginFunc(ctx, db, fn func(Tx) error) and BeginTxFunc(ctx, db, txOptions, fn).
	{pkg: "github.com/jackc/pgx/v5", name: "BeginFunc", outer: 1, callback: 2, tx: 0},
	{pkg: "github.com/jackc/pgx/v5", name: "BeginTxFunc", outer: 1, callback: 3, tx: 0},

	// pgx v4.18: BeginFunc(ctx, f func(Tx) error) and BeginTxFunc(ctx, txOptions, f) of Conn,
	// of Tx (a savepoint), and of pgxpool.Pool.
	{pkg: "github.com/jackc/pgx/v4", recv: "Conn", name: "BeginFunc", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/jackc/pgx/v4", recv: "Conn", name: "BeginTxFunc", outer: -1, callback: 2, tx: 0},
	{pkg: "github.com/jackc/pgx/v4", recv: "Tx", name: "BeginFunc", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "BeginFunc", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "BeginTxFunc", outer: -1, callback: 2, tx: 0},

	// goqu v9: func (d *Database) WithTx(fn func(*TxDatabase) error) error
	{pkg: "github.com/doug-martin/goqu/v9", recv: "Database", name: "WithTx", outer: -1, callback: 0, tx: 0},

	// pop v6: Transaction(fn func(tx *Connection) error) and Rollback(fn func(tx *Connection)).
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "Transaction", outer: -1, callback: 0, tx: 0},
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "Rollback", outer: -1, callback: 0, tx: 0},

	// upper/db v4: Tx(fn func(sess Session) error) and TxContext(ctx, fn, opts) of Session.
	{pkg: "github.com/upper/db/v4", recv: "Session", name: "Tx", outer: -1, callback: 0, tx: 0},
	{pkg: "github.com/upper/db/v4", recv: "Session", name: "TxContext", outer: -1, callback: 1, tx: 0},

	// go-pg v10: RunInTransaction(ctx, fn func(*Tx) error) of DB, through its embedded
	// baseDB, and of Tx.
	{pkg: "github.com/go-pg/pg/v10", recv: "baseDB", name: "RunInTransaction", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-pg/pg/v10", recv: "Tx", name: "RunInTransaction", outer: -1, callback: 1, tx: 0},

	// xorm v1.3 and v2: func (engine *Engine) Transaction(f func(*Session) (any, error)) (any, error)
	{pkg: "xorm.io/xorm", recv: "Engine", name: "Transaction", outer: -1, callback: 0, tx: 0},
	{pkg: "xorm.io/xorm/v2", recv: "Engine", name: "Transaction", outer: -1, callback: 0, tx: 0},

	// go-redis v9, and v8 under its old path: Watch(ctx, fn func(*Tx) error, keys...) and
	// TxPipelined(ctx, fn func(Pipeliner) error) of each client, of Tx inside Watch, and of
	// the interfaces that declare them.
	{pkg: "github.com/redis/go-redis/v9", recv: "Client", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "ClusterClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Ring", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "UniversalClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Client", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Conn", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "ClusterClient", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Ring", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Tx", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/redis/go-redis/v9", recv: "Cmdable", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Client", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "ClusterClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Ring", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "UniversalClient", name: "Watch", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Client", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Conn", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "ClusterClient", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Ring", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
	{pkg: "github.com/go-redis/redis/v8", recv: "Tx", name: "TxPipelined", outer: -1, callback: 1, tx: 0},
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
	// and func WithSession(ctx, sess *Session, fn func(context.Context) error) error.
	{pkg: "go.mongodb.org/mongo-driver/v2/mongo", recv: "Session", name: "WithTransaction", outer: 0, callback: 1, tx: 0, inContext: true},
	{pkg: "go.mongodb.org/mongo-driver/v2/mongo", name: "WithSession", outer: 0, callback: 2, tx: 0, inContext: true},
	// MongoDB v1.17: the same, with the Session interface and SessionContext.
	{pkg: "go.mongodb.org/mongo-driver/mongo", recv: "Session", name: "WithTransaction", outer: 0, callback: 1, tx: 0, inContext: true},
	{pkg: "go.mongodb.org/mongo-driver/mongo", name: "WithSession", outer: 0, callback: 2, tx: 0, inContext: true},
	// go-rel: Transaction(ctx, fn func(ctx context.Context) error) of the Repository interface.
	{pkg: "github.com/go-rel/rel", recv: "Repository", name: "Transaction", outer: 0, callback: 1, tx: 0, inContext: true},
	// avito-tech/go-transaction-manager v2: Do(ctx, fn) and DoWithSettings(ctx, s, fn) of the
	// trm.Manager interface and of manager.Manager.
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2", recv: "Manager", name: "Do", outer: 0, callback: 1, tx: 0, inContext: true},
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2", recv: "Manager", name: "DoWithSettings", outer: 0, callback: 2, tx: 0, inContext: true},
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2/manager", recv: "Manager", name: "Do", outer: 0, callback: 1, tx: 0, inContext: true},
	{pkg: "github.com/avito-tech/go-transaction-manager/trm/v2/manager", recv: "Manager", name: "DoWithSettings", outer: 0, callback: 2, tx: 0, inContext: true},
}

// Of reports the callback opener that call is. It reports nothing for a call
// that opens no transaction, and for one whose callback is not a function
// literal: a callback held in a variable or named as a function has no body
// here to read.
func Of(info *types.Info, call *ast.CallExpr) (Opener, bool) {
	fn, k, ok := lookup(info, call, knownOpeners)
	if !ok {
		return Opener{}, false
	}
	o := Opener{Name: displayName(fn), InContext: k.inContext, Builds: k.builds}
	if k.outer < 0 {
		// A method expression, as in (*gorm.DB).Transaction(db, fn), takes
		// its receiver as an argument, so its arguments are shifted and its
		// selector names no handle.
		sel, ok := methodSelector(info, call)
		if !ok {
			return Opener{}, false
		}
		o.Outer = sel
	} else {
		o.Outer = call.Args[k.outer]
	}
	lit, ok := ast.Unparen(call.Args[k.callback]).(*ast.FuncLit)
	if !ok {
		return Opener{}, false
	}
	o.Callback = lit
	o.Tx = paramIdent(lit, k.tx)
	return o, true
}

// Begin is one call that returns a transaction, and the variables it is
// assigned to.
type Begin struct {
	// Name is how a message names the call, as Opener.Name does.
	Name string
	// Outer is the selector of the call, which names the handle.
	Outer *ast.SelectorExpr
	// Tx is the variable the transaction is assigned to.
	Tx *ast.Ident
	// Err is the variable the error is assigned to. It is nil when the call
	// returns no error, or the error is not kept.
	Err *ast.Ident
}

// knownBegins lists every begin. Each is a method that returns the
// transaction first, and an error second when it may fail.
var knownBegins = []known{
	// database/sql
	{pkg: "database/sql", recv: "DB", name: "Begin"},
	{pkg: "database/sql", recv: "DB", name: "BeginTx"},
	{pkg: "database/sql", recv: "Conn", name: "BeginTx"},
	// sqlx
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "Beginx"},
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "BeginTxx"},
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "MustBegin"},
	{pkg: "github.com/jmoiron/sqlx", recv: "DB", name: "MustBeginTx"},
	{pkg: "github.com/jmoiron/sqlx", recv: "Conn", name: "BeginTxx"},
	// pgx v5 and v4: Begin and BeginTx of Conn, of Tx (a savepoint), and of pgxpool's
	// Pool and Conn.
	{pkg: "github.com/jackc/pgx/v5", recv: "Conn", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v5", recv: "Conn", name: "BeginTx"},
	{pkg: "github.com/jackc/pgx/v5", recv: "Tx", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", recv: "Pool", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", recv: "Pool", name: "BeginTx"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", recv: "Conn", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", recv: "Conn", name: "BeginTx"},
	{pkg: "github.com/jackc/pgx/v4", recv: "Conn", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v4", recv: "Conn", name: "BeginTx"},
	{pkg: "github.com/jackc/pgx/v4", recv: "Tx", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Pool", name: "BeginTx"},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Conn", name: "Begin"},
	{pkg: "github.com/jackc/pgx/v4/pgxpool", recv: "Conn", name: "BeginTx"},
	// gorm: func (db *DB) Begin(opts ...*sql.TxOptions) *DB
	{pkg: "gorm.io/gorm", recv: "DB", name: "Begin"},
	// bun: BeginTx of DB, Conn, and Tx (a savepoint).
	{pkg: "github.com/uptrace/bun", recv: "DB", name: "BeginTx"},
	{pkg: "github.com/uptrace/bun", recv: "Conn", name: "BeginTx"},
	{pkg: "github.com/uptrace/bun", recv: "Tx", name: "BeginTx"},
	// bob
	{pkg: "github.com/stephenafamo/bob", recv: "DB", name: "BeginTx"},
	{pkg: "github.com/stephenafamo/bob", recv: "Conn", name: "BeginTx"},
	// goqu
	{pkg: "github.com/doug-martin/goqu/v9", recv: "Database", name: "Begin"},
	{pkg: "github.com/doug-martin/goqu/v9", recv: "Database", name: "BeginTx"},
	// go-pg, through DB's embedded baseDB.
	{pkg: "github.com/go-pg/pg/v10", recv: "baseDB", name: "Begin"},
	{pkg: "github.com/go-pg/pg/v10", recv: "baseDB", name: "BeginContext"},
	// pop: the transaction is a *Connection of its own.
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "NewTransaction"},
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "NewTransactionContext"},
	{pkg: "github.com/gobuffalo/pop/v6", recv: "Connection", name: "NewTransactionContextOptions"},
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
	sel, ok := methodSelector(info, call)
	if !ok {
		return Begin{}, false
	}
	fn := info.Selections[sel].Obj().(*types.Func)
	if _, _, ok := lookup(info, call, knownBegins); !ok && !entBegin(fn) {
		return Begin{}, false
	}
	b := Begin{Name: fn.Name(), Outer: sel, Tx: lhs[0]}
	if len(lhs) == 2 && lhs[1] != nil && lhs[1].Name != "_" {
		b.Err = lhs[1]
	}
	return b, true
}

// entBegin reports whether fn is the Tx or BeginTx method of a client that
// ent generated. ent writes the client into the caller's package, so it has
// no package path to look up. Its shape does: a method of Client that
// returns a *Tx and an error, whose Client method gives the *Client back.
func entBegin(fn *types.Func) bool {
	if fn.Name() != "Tx" && fn.Name() != "BeginTx" || recvName(fn) != "Client" {
		return false
	}
	res := fn.Signature().Results()
	if res.Len() != 2 {
		return false
	}
	ptr, ok := res.At(0).Type().(*types.Pointer)
	if !ok {
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
	if !ok || fn.Pkg() == nil {
		return nil, known{}, false
	}
	recv := recvName(fn)
	for _, k := range table {
		if k.pkg == fn.Pkg().Path() && k.name == fn.Name() && k.recv == recv {
			return fn, k, true
		}
	}
	return nil, known{}, false
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
