//declscope:shared // invoker.go, begin.go and conn.go read the tables, and look calls up in them
package txcall

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

// known is one call that opens a transaction.
type known struct {
	pkg string
	// recv is the name of the receiver's type, with any pointer removed. It
	// is empty for a function.
	//
	//declscope:private
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
	// (a savepoint), and the IDB interface. None is read on a Conn: see onConn.
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

// knownBegins lists every begin. Each is a method that returns the
// transaction first, and an error second when it may fail.
var knownBegins = []known{
	// A begin on one connection, as database/sql's Conn.BeginTx, is not
	// listed: see onConn.
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

// lookupKnown finds the entry of table that call is a call of.
func lookupKnown(info *types.Info, call *ast.CallExpr, table []known) (*types.Func, known, bool) {
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok {
		return nil, known{}, false
	}
	k, ok := lookupKnownFunc(fn, table)
	return fn, k, ok
}

// lookupKnownFunc finds the entry of table for fn. A method of the universe, as
// error's, belongs to no package and has none.
func lookupKnownFunc(fn *types.Func, table []known) (known, bool) {
	if fn.Pkg() == nil {
		return known{}, false
	}
	recv := callRecvName(fn)
	for _, k := range table {
		if k.pkg == fn.Pkg().Path() && k.name == fn.Name() && k.recv == recv {
			return k, true
		}
	}
	return known{}, false
}
