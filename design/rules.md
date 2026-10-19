# Rules

This file specifies what txbypass reports. It is the source of truth for the fixtures under `testdata/src/`. When a rule changes, change this file first, then its fixtures, then the code.

## Scope

txbypass reports code that runs outside a transaction while the transaction is open. It finds uses in the syntax of a transaction's scope, and asks the package's SSA form what each use holds. It follows values inside a function, and a transaction into the helpers it is given.

| Rule | What it reports |
| --- | --- |
| [`outer-handle`](#outer-handle) | The handle a transaction was opened on, used while the transaction is open |
| [`outer-context`](#outer-context) | The context given to an invoker, used inside a callback whose context carries the transaction |

### Terms

| Term | Meaning |
| --- | --- |
| Invoker | A call that opens a transaction and invokes a function literal in it. See [Invokers](#invokers) |
| Begin | A call that returns a transaction, which stays open until it is committed or rolled back. See [Begins](#begins) |
| Handle | A variable, and the fields selected from it. See [Handles](#handles) |
| Outer handle | The handle a transaction is opened on |
| Scope | The code that runs inside one transaction. See [Scopes](#scopes) |
| Generated file | A file with a `// Code generated ... DO NOT EDIT.` comment before the package clause, as `ast.IsGenerated` reads it. A file that cgo rewrote is not one |

Nothing is reported in a generated file. Test files are checked like any other file. Code in a package-level declaration is read too, as a function literal in a table of test cases: the package initializer runs it.

## Invokers

An invoker is recognized by the package path, the receiver's type and the name of the function it calls. An alias of the type does not hide it.

| Library | Package | Receiver | Name | Outer handle | Callback | Transaction |
| --- | --- | --- | --- | --- | --- | --- |
| gorm | `gorm.io/gorm` | `*DB` | `Transaction` | The receiver | Argument 0 | Parameter 0 |
| bun | `github.com/uptrace/bun` | `*DB`, `Tx`, `IDB` | `RunInTx` | The receiver | Argument 2 | Parameter 1 |
| bob | `github.com/stephenafamo/bob` | `DB` | `RunInTx` | The receiver | Argument 2 | Parameter 1 |
| pgx v5 | `github.com/jackc/pgx/v5` | None | `BeginFunc` | Argument 1 | Argument 2 | Parameter 0 |
| pgx v5 | `github.com/jackc/pgx/v5` | None | `BeginTxFunc` | Argument 1 | Argument 3 | Parameter 0 |
| pgx v4 | `github.com/jackc/pgx/v4` | `Tx` | `BeginFunc` | The receiver | Argument 1 | Parameter 0 |
| pgx v4 | `github.com/jackc/pgx/v4/pgxpool` | `*Pool` | `BeginFunc` | The receiver | Argument 1 | Parameter 0 |
| pgx v4 | `github.com/jackc/pgx/v4/pgxpool` | `*Pool` | `BeginTxFunc` | The receiver | Argument 2 | Parameter 0 |
| goqu | `github.com/doug-martin/goqu/v9` | `*Database` | `WithTx` | The receiver | Argument 0 | Parameter 0 |
| pop | `github.com/gobuffalo/pop/v6` | `*Connection` | `Transaction`, `Rollback` | The receiver | Argument 0 | Parameter 0 |
| upper/db | `github.com/upper/db/v4` | `Session` | `Tx` | The receiver | Argument 0 | Parameter 0 |
| upper/db | `github.com/upper/db/v4` | `Session` | `TxContext` | The receiver | Argument 1 | Parameter 0 |
| go-pg | `github.com/go-pg/pg/v10` | `*baseDB`, embedded in `*DB` | `RunInTransaction` | The receiver | Argument 1 | Parameter 0 |
| xorm | `xorm.io/xorm`, `xorm.io/xorm/v2` | `*Engine` | `Transaction` | The receiver | Argument 0 | Parameter 0 |
| go-redis | `github.com/redis/go-redis/v9`, `github.com/go-redis/redis/v8` | `*Client`, `*ClusterClient`, `*Ring`, `UniversalClient` | `Watch` | The receiver | Argument 1 | Parameter 0 |
| go-redis | The same | `*Client`, `*Conn`, `*ClusterClient`, `*Ring`, `Cmdable` | `TxPipelined` | The receiver | Argument 1 | Parameter 0 |
| Firestore | `cloud.google.com/go/firestore` | `*Client` | `RunTransaction` | The receiver, which [builds references](#references) | Argument 1 | Parameter 1 |
| Spanner | `cloud.google.com/go/spanner` | `*Client` | `ReadWriteTransaction`, `ReadWriteTransactionWithOptions` | The receiver | Argument 1 | Parameter 1 |
| Datastore | `cloud.google.com/go/datastore` | `*Client` | `RunInTransaction` | The receiver | Argument 1 | Parameter 0 |

These carry the transaction in the callback's context. They are read by [`outer-context`](#outer-context):

| Library | Package | Receiver | Name | Outer context | Callback | Inner context |
| --- | --- | --- | --- | --- | --- | --- |
| MongoDB v2 | `go.mongodb.org/mongo-driver/v2/mongo` | `*Session` | `WithTransaction` | Argument 0 | Argument 1 | Parameter 0 |
| MongoDB v2 | The same | None | `WithSession` | Argument 0 | Argument 2 | Parameter 0 |
| MongoDB v2 | The same | `*Client` | `UseSession` | Argument 0 | Argument 1 | Parameter 0 |
| MongoDB v2 | The same | `*Client` | `UseSessionWithOptions` | Argument 0 | Argument 2 | Parameter 0 |
| MongoDB v1 | `go.mongodb.org/mongo-driver/mongo` | `Session` | `WithTransaction` | Argument 0 | Argument 1 | Parameter 0 |
| MongoDB v1 | The same | None | `WithSession` | Argument 0 | Argument 2 | Parameter 0 |
| MongoDB v1 | The same | `*Client` | `UseSession` | Argument 0 | Argument 1 | Parameter 0 |
| MongoDB v1 | The same | `*Client` | `UseSessionWithOptions` | Argument 0 | Argument 2 | Parameter 0 |
| go-rel | `github.com/go-rel/rel` | `Repository` | `Transaction` | Argument 0 | Argument 1 | Parameter 0 |
| go-transaction-manager | `github.com/avito-tech/go-transaction-manager/trm/v2`, and its `manager` package | `Manager` | `Do` | Argument 0 | Argument 1 | Parameter 0 |
| go-transaction-manager | The same | `Manager` | `DoWithSettings` | Argument 0 | Argument 2 | Parameter 0 |

A call is not an invoker for txbypass when:

| Case | Why |
| --- | --- |
| The callback is not a function literal | Its body is not read |
| The receiver is given as an argument, through a method expression | The arguments are shifted, and the selector names no handle |
| The arguments are one call with several results, as `pgx.BeginFunc(args())` | There is no function literal to read |
| The outer handle is one dedicated connection, as pgx v5's `*pgx.Conn` or go-pg's `*Conn`, an alias of one, or a type whose `Begin` is declared on one | See [Dedicated connections](#dedicated-connections) |
| The outer handle is a transaction | See [Savepoints](#savepoints) |

A method promoted from an embedded field is an invoker of that field. `s.Transaction(fn)`, where `s` embeds `*gorm.DB`, opens the transaction on `s.DB`, which is the handle `s`.

### Dedicated connections

A transaction begun on one dedicated connection runs on that connection. A statement on the connection runs inside the transaction too, so it bypasses nothing. A begin or an invoker on one is not read:

| Library | Type |
| --- | --- |
| database/sql | `*Conn` |
| sqlx | `*Conn` |
| pgx v5, v4 | `*Conn`, and `*Conn` of their `pgxpool` packages |
| bun | `Conn` |
| bob | `Conn` |
| go-pg | `*Conn`. Its methods come from the `*baseDB` it embeds, as `*DB`'s do, so the type of the receiver decides |

go-redis's `*Conn` is listed. `TxPipelined` queues commands until the callback returns. A command run on the connection inside the callback is sent before the queued ones, outside `MULTI`.

### Savepoints

A transaction opened on a transaction is a savepoint. It runs on the connection of the transaction, and so does a statement on the transaction. A use of the transaction inside the savepoint bypasses nothing, so a savepoint is not read:

| The handle | Known by |
| --- | --- |
| pgx's `Tx`, v5 and v4, and bun's `Tx`, or a type that embeds one, as pgxmock's mock | Its type, or the type the method is declared on |
| A begin's result, as gorm's `tx := db.Begin()` | Where it comes from |
| The transaction an invoker gives, as gorm's `tx` in `db.Transaction(func(tx *gorm.DB) error { ... })` | Where it comes from |

A savepoint's begins stay in the table of begins. Its result is then a handle of its own, so its commit ends nothing of the outer transaction.

Only these open a savepoint, or reuse the transaction, when called on one. The others are read on any handle:

| Library | Invoker or begin |
| --- | --- |
| gorm | `Transaction`, `Begin` |
| pop | `Transaction`, `Rollback`, `NewTransaction`, `NewTransactionContext`, `NewTransactionContextOptions` |
| pgx, bun | Every one, by the type of the transaction |

The `Tx` of upper/db on a transaction begins another transaction on the pool. The `TxPipelined` of go-redis queues commands, and a command on the handle runs at once, before the queued ones. A use of the outer transaction in either is reported.

### Helpers that are invokers

A function of the project can open a transaction and run a callback it is given inside it. Such a helper is an invoker too. It is read from its body, so it needs no configuration:

```go
func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}
```

`s.withTx(ctx, func(tx *sql.Tx) error { ... })` opens a scope on `s.db`. A use of `s.db` inside the callback is reported. A use of `s.audit` is not.

A function is a helper when all of these hold:

| Condition | Why |
| --- | --- |
| It is a declared function or method, and one of its parameters is of a function type | That parameter is the callback |
| It opens a transaction with an invoker, or a helper, and calls the callback in that invoker's callback. Or it calls the callback in the region of a begin | The callback runs inside the transaction |
| Each call of the callback is given the transaction, at the same parameter | That parameter is the callback's transaction |
| The transaction is opened on a parameter, or on a field selected from one. Every call agrees on which | That handle is the outer one. A package-level variable is no parameter |
| The callback is only called, or compared with `nil`. It is not stored, passed on, or called by `go` or `defer` | It might then run outside the transaction. A guard such as `if fn == nil { return errNoFunc }` hands it nowhere |
| No call of the callback runs outside a transaction | A helper that reuses a transaction it is given, as `if cur != nil { return fn(cur) }`, also runs the callback outside the one it opens |

The first parameter of a function type that meets them is the callback. A second callback that runs inside the transaction too is not read. A helper that calls itself is read as no helper at its own call.

A helper of another package is known by the fact `invokes[pO.f, pC(T)]`, which the analysis of its package exports. `pO` is the parameter that holds the handle, and `.f` the fields selected from it. `pC` is the callback, and `T` the callback's parameter that is given the transaction. The receiver of a method is parameter 0.

At the call, the scope is read as an invoker's is. The message names the helper as it names an invoker: a method by its name, a function by its package's name and its own. A helper may open with an invoker or a begin that opens a [savepoint](#savepoints) on a transaction. Called on a transaction, such a helper opens a savepoint too, and its scope is not read. A call is not read when:

| Case | Why |
| --- | --- |
| The callback is not a function literal | Its body is not read |
| The callback is the receiver, or the handle is the receiver of a method expression | The selector names no handle, or no literal |
| The arguments are one call with several results | There is no function literal to read |
| The handle given is one dedicated connection, or a transaction that holds one, as pgx's `Tx` | See [Dedicated connections](#dedicated-connections) |

A transaction that a helper gives its callback, or that a helper returns, is a transaction for [Savepoints](#savepoints) too.

A helper may begin its transaction with a [helper that is a begin](#helpers-that-are-begins). A helper that carries the transaction in a context is not read yet.

## Begins

A begin is a method recognized as an invoker is. It returns the transaction first, and an error second when it may fail.

| Library | Package | Receiver | Name |
| --- | --- | --- | --- |
| database/sql | `database/sql` | `*DB` | `Begin`, `BeginTx` |
| sqlx | `github.com/jmoiron/sqlx` | `*DB` | `Beginx`, `BeginTxx`, `MustBegin`, `MustBeginTx` |
| pgx v5, v4 | `github.com/jackc/pgx/v5`, `github.com/jackc/pgx/v4` | `Tx` | `Begin` |
| pgx v5, v4 | Their `pgxpool` packages | `*Pool` | `Begin`, `BeginTx` |
| gorm | `gorm.io/gorm` | `*DB` | `Begin` |
| bun | `github.com/uptrace/bun` | `*DB`, `Tx` | `Begin`, `BeginTx` |
| bun | `github.com/uptrace/bun` | `IDB` | `BeginTx` |
| bob | `github.com/stephenafamo/bob` | `DB` | `Begin`, `BeginTx` |
| goqu | `github.com/doug-martin/goqu/v9` | `*Database` | `Begin`, `BeginTx` |
| go-pg | `github.com/go-pg/pg/v10` | `*baseDB`, embedded in `*DB` | `Begin`, `BeginContext` |
| pop | `github.com/gobuffalo/pop/v6` | `*Connection` | `NewTransaction`, `NewTransactionContext`, `NewTransactionContextOptions` |
| Datastore | `cloud.google.com/go/datastore` | `*Client` | `NewTransaction` |
| ent | The caller's package | `*Client` | `Tx`, `BeginTx` |

ent generates its client into the caller's package, so its begin is read from the shape of the generated code:

| The method | Must |
| --- | --- |
| Name | Be `Tx` or `BeginTx` |
| Receiver | Be a type named `Client` |
| Results | Be two. The first is a pointer, `*Tx`, with a method that ends a transaction, as `Commit` |
| The first result's `Client` method | Return one value, of the receiver's type |

go-pg's `*Tx` has a `Begin` and a `RunInTransaction` too, but neither opens a transaction. `Begin` returns the same transaction, and `RunInTransaction` runs its callback on it. They are not in either table.

A begin is read where its result is kept in a variable:

| Written | Read |
| --- | --- |
| `tx, err := db.BeginTx(ctx, nil)`, or with `=` | Yes. `tx` is the transaction, and `err` its error |
| `tx := db.MustBegin()` | Yes, with no error |
| `var tx, err = db.Begin()` | Yes |
| `if tx, err := db.Begin(); err == nil { ... }` | Yes |
| `_, err := db.Begin()` | No. No transaction is kept |
| `r.tx, err = db.Begin()` | No. A field is not followed |

### Helpers that are begins

A function of the project that returns a transaction it begins is a begin too. It is read from its body:

```go
func (s *Store) begin(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	return tx, nil
}
```

`tx, err := s.begin(ctx)` opens a region on `s.db`, as `s.db.BeginTx` would. A function is such a helper when all of these hold:

| Condition | Why |
| --- | --- |
| It is a declared function or method. Its first result has a method that ends a transaction. It has no other result, or a second of type `error` | The results read as a begin's do |
| Each return gives `nil`, or a transaction it began with a begin or a helper, still open there | A transaction committed before the return, or one it was given, is not new |
| A return of `nil` gives an error beside it that is not the constant `nil` | A helper that may begin nothing, with no error, gives the caller a begin that did not happen |
| A function literal sets a named error result only under `err != nil`, and hands it nowhere. The address of the error is not given to a call, or stored, as `defer annotate(&err)` or `check(&err)` does | A deferred literal that wraps the error keeps a `nil` error `nil`. One that sets it otherwise may return an open transaction beside an error |
| No deferred call is given the transaction, as `defer tx.Rollback()` is | It runs after the return, and may end the transaction before the caller uses it |
| A return that gives the transaction gives `nil` or the begin's own error beside it. For gorm's `Begin`, which has no error result, that is `tx.Error` | The caller reads the error as a failed begin. A transaction returned with another error stays open where the caller reads none |
| The transactions are begun on a parameter, or on a field selected from one, and every return agrees on which | That handle is the outer one |

A transaction wrapped in a struct of the project is not read. A helper of another package is known by the fact `begins[pO.f]`, spelled as `invokes` spells its handle. At the call, a method expression or arguments of one call with several results are not read.

## Handles

A handle is a root value and the fields selected from it. The root is what the SSA form gives a value before any field is selected: a parameter, a package-level variable, the result of a call.

| The value | Is the handle of |
| --- | --- |
| A copy, as `d := db` | What it copies. The SSA form makes them one value |
| A variable a function literal captures, written by one store | The value stored |
| A variable in memory written by more than one store, or through a field, all in its own function | The variable itself, as `r` of `r := &Repo{db: db}`. A callback runs while it holds one value. A scope that writes it is below |
| A variable written by a function literal that captures it, or whose address, or a field's, goes elsewhere | No handle: it is the same handle as nothing |
| A variable whose address, or a field's, is given to a function of the module or a function value, as `r.Bind(tx)` | No handle. The function may store another handle there, in any package of the module. Outside module mode, only a function of the package counts |
| A variable whose address is given to a function of another module, as `db.Begin()` on a `gorm.DB` held by value | Followed. A library's method does not rebind a field of the caller's struct |
| A load of a field, `x.f`, or a field of a struct value | The handle of `x`, then `f` |
| A method value, as `db.Create` | Its receiver |
| A call of a method whose result has the receiver's type, as `db.WithContext(ctx)` | Its receiver, with three exceptions. A [begin](#begins), as gorm's `db.Begin()`, gives the transaction. `WithTx` gives queries bound to it: see [sqlc's `WithTx`](#sqlcs-withtx). A method that takes a value of the receiver's type binds a new value to it, as `repo.Using(tx)` does. So does one that takes a value with a method that ends a transaction |
| A conversion, a conversion to an interface, or an assertion that panics when it fails | What it converts |
| A phi whose edges all give one handle | That handle. An edge back to the phi, or to the phi of an outer loop, gives nothing |
| A free variable of a literal | What the closure binds it to |
| The address of a variable or a field, as the receiver of `r.db.Begin()` when `db` is held by value | The handle it holds |
| Anything else | A root of its own |

Two values are the same handle when they have the same root and the same fields. A use reaches the outer handle when it is the outer handle, or a field selected from it: reading `db.Error` reads `db`. A method or a field promoted from an embedded handle reaches it through the embedded field, as `s.Create(u)` reaches `s.DB`.

A struct that embeds the handle is not the handle. It may hold other things, as a logger, a cache, a mutex or a second handle. So `s.validate(u)`, `s.logger.Printf(...)` and `s.Lock()` are no uses of `s.DB`. A parameter of the callback with the outer handle's name is another value.

A read of a field of the handle that holds a basic value or an error is no use, nor is a variable that holds one. In gorm, `db.Create(x).Error` is reported once, at the statement. Reading the error or `RowsAffected` afterwards runs nothing, and an ignore on the statement leaves no report behind.

A chain of calls on the handle that is given the transaction runs on it, as bun's `db.NewInsert().Model(u).Conn(tx).Exec(ctx)` does. So does a chain on a call given the handle, as sqlc's `New(pool).WithTx(tx)`. A call of the handle given a value built with the transaction runs in it, as Datastore's `client.GetAll(ctx, q, &dst)` with `q := datastore.NewQuery(k).Transaction(tx)`. Only a method that returns its receiver's type binds the value to the transaction. A method of a bound value gives a bound value too, as Datastore's `q.NewAggregationQuery()`. A function given the transaction, as `context.WithValue(ctx, key, tx)`, binds nothing. The call's result may be kept first, as `q := New(pool)` and then `q.WithTx(tx)`. A literal built on the handle and then given the transaction is the same, as `m := &Manager{DB: db}` and then `m.CreateWithTx(tx, ...)`. Every use of it must be a call given the transaction. A call without it, as `m.Exists(name)`, uses the handle. The call that makes a handle is no use of it either. The handle there is no use.

The handle given to a call that takes the transaction too is no use. A constructor such as `newStore(s.db, tx)` keeps the handle beside the transaction, as the composite literal `&Store{db: s.db, tx: tx}` does.

A call of a method that reads or sets the pool itself is no use. The transaction has no such method, and the call runs no statement:

| Library | Methods |
| --- | --- |
| database/sql | `Stats`, `Driver`, `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`, `SetConnMaxIdleTime` of `*DB` |
| pgx v5, v4 | `Stat` and `Config` of `*pgxpool.Pool` |
| bun | `DBStats` of `*DB` |

A scope that writes the outer handle, or a variable or a field that holds it, reports no use of it. After the write, it may hold another value. So `db = tx` or `s.db = tx` inside the callback makes `db` or `s.db` no use. A store of a value taken from the transaction into a field of the outer handle is such a write too. In gorm, `base.Statement.ConnPool = tx.Statement.ConnPool` makes `base` run on the transaction. In a region, only the writes inside the region count, and a deferred literal writes when the function returns.

What ends a transaction is matched by where it is stored, not by its value. A commit through a load of a variable assigned twice still ends the transaction the variable holds. So does a commit on a value that may be the transaction on one path, as `tx` in a loop that commits and begins again.

A use is an expression with a value in the SSA form, or a variable used through its address. What stands on the left of an assignment is written, not used, and so is a variable its declaration names. An index on the left is read: `m[k] = v` uses `m` and `k`.

## Scopes

A scope is the code that runs inside one transaction. Each use of the outer handle in it is reported.

| Opened by | Scope |
| --- | --- |
| An invoker | The callback's body, with its own function literals |
| A begin | The region where the transaction is open. See [Regions](#regions) |

In a callback, a function literal kept for after it is not read. Such a literal is stored in a variable declared outside the callback, or in a field or an element of one. It may be stored directly, appended, set as a field of a composite literal, or converted to a named function type. It may also be named first in a local of the callback that nothing else uses. That is how an after-commit hook is kept: it runs once the callback has returned, outside the transaction, on purpose.

A literal given to any other call, as `after = wrap(func() {...})`, is still read: the call may run it inside the callback.

Code that no path reaches, as after a `return` or a call of `log.Fatal`, is not read: it never runs. Neither is a function literal that is never made, as one assigned to `_`, nor a begin or an invoker inside one. A literal that a call runs is made, whatever becomes of the call's result, as `func() {...}()` or `g.Go(func() error {...})`.

Once an expression is reported, nothing inside it is read again. A position is reported once for each rule. A use inside two transactions opened on the same handle is reported for the first one that reads it.

### Regions

A begin's region is read from the SSA form of the function that holds it. A begin inside a function literal belongs to the literal.

An instruction is in the region when the transaction is open there on every path from the function's entry:

| On a path | The transaction is |
| --- | --- |
| The begin | Open after it |
| A call of `Commit`, `Rollback`, `CommitContext` or `RollbackContext` on the transaction, a copy of it, or a field of it, that takes no function | Closed after it. pop commits through `tx.TX.Commit()`. pop's `Rollback(fn)` takes a function, and opens a transaction of its own |
| A call of a function that ends a parameter it is given the transaction in | Closed after it. See [Helpers](#helpers) |
| A call of a function literal that ends a variable it captures | Closed after it |
| A store of the transaction anywhere but in a variable of the function. An insert of it into a map, as a key or a value. A send of it on a channel, in a `select` too | Closed after it. Code this analysis does not read may end it |
| A function literal that ends the transaction, made and given to anything but a call or a `defer` | Closed after it, for the same reason. A literal nothing uses, as one assigned to `_`, never runs and ends nothing |
| The edge where the begin's error is set | Closed. See below |
| A call that never returns, as `log.Fatal` or `os.Exit`, or a panic | The path ends. It takes no part |

| Not in the region | Why |
| --- | --- |
| A deferred statement of the function that holds the begin | It runs when the function returns. A deferred statement in a function literal runs when the literal returns, so it is read |
| A function given to `t.Cleanup`, anywhere in a scope | It runs after the test returns, as a deferred call does. A rollback in it does not end the region, and a use in it is not read. A call among the arguments of `t.Cleanup` or of a deferred call runs where it stands, so it is read. A test helper of the package may use a handle only in a function it gives to `t.Cleanup`, as one that drops a table. It may be a function, a method or a generic function. Giving it the handle is no use either. A defer in the helper runs when it returns, inside the transaction, so it is a use |
| Code after a branch that may end the transaction | Not every path leaves it open |

The begin's error is set on the true edge of `err != nil`, and on the false edge of `err == nil`. A begin with no error result, as gorm's `Begin`, reports failure in the transaction's `Error` field, so `tx.Error` is its error. That is the field of the begin's value only: `tx.Create(u).Error` is another statement's error. Here `err` is the begin's own error value. A later `err` that another call assigned is another value.

A test may assert that the begin failed instead, with testify's `require.Error`, `ErrorIs`, `ErrorAs` or `ErrorContains`, or their `f` forms and the methods of `require.Assertions`. Each returns only when the error is set, so no transaction is open after it. `assert` returns either way, so it changes nothing.

An error can live in memory, as a named result that a deferred literal reads. A load of it counts when every path to the load last stored the begin's error there. With gorm, that is a store of `tx.Error`, as `err = tx.Error`. A branch between the begin and the check changes nothing, but another error stored on one path does.

A transaction given to a function value or an interface method, whose body is not known, stays open.

### Helpers

A function ends its parameter when its body calls one of the ending methods on it. So does a function that gives the parameter to a function or a literal that ends it. Deferred calls count. Only a parameter whose type has an ending method is read.

| The function is | It is read |
| --- | --- |
| Of the package being analyzed | Directly, through the generic origin of an instance. A call of itself counts as no end |
| Of another package | Through the fact `closes[i...]` that the analysis of that package exports. The receiver of a method is parameter 0 |

A library's own methods are helpers too. go-pg's `Tx.Close` calls `RollbackContext` when the transaction has not ended. So its fact says it ends its receiver, and a region ends there.

A store or a send inside a helper does not count. A library's own methods store their receiver in many places, and would seem to end it.

### References

Firestore's client builds the references a transaction reads and writes, as in `tx.Get(client.Doc("users/a"))`. Building one runs nothing. So in a scope of `RunTransaction`, a use of the client counts only when it runs:

| Inside the callback | Reported | Why |
| --- | --- | --- |
| `tx.Get(client.Doc("users/a"))` | No | The chain of calls on `client` takes no context |
| `ref := client.Collection("users").Doc("b")` | No | The same |
| `client.Doc("users/c").Set(ctx, v)` | Yes | A call in the chain takes a context: a parameter of the method has type `context.Context`, whatever the caller passes there |
| `save(ctx, client)` | Yes | The handle on its own is given to a function |

### sqlc's `WithTx`

`q.WithTx(tx)` rebinds `q` to the transaction, as sqlc's generated code does. Code that calls it means to run on the transaction, so every other use of `q` in the scope is reported by `outer-handle`.

| The call | Must |
| --- | --- |
| The method | Be named `WithTx`, take one argument, and be a method. It may be promoted from an embedded field, as from the `*Queries` of sqlc's `Store` |
| The argument | Be the scope's transaction, or a field of it, as `tx.Tx` of an sqlx or a bun transaction |
| The receiver | Name a handle |
| The result | Have the type of the method's own receiver |
| The receiver | Be a struct, or a pointer to one, whose every field can hold the transaction or is a `*sql.Stmt`. The `Queries` of sqlc holds only its `DBTX`, and with `emit_prepared_queries` a statement per query. A struct that holds more, as a logger or metrics, is not only a database: its other members are no uses |

The call itself is not a use, and neither is a chain built on it, as in `q.WithTx(tx).CreateUser(ctx)`. A callback whose transaction parameter is unnamed rebinds nothing.

## Declared helpers

A helper whose body cannot be read is declared. That is an interface method, as a transaction manager given by dependency injection, or a function of a package you cannot edit. A declaration also wins over what a body would say.

| Declare | With | It says |
| --- | --- | --- |
| An invoker | `//txbypass:invokes`, or `-invokes` | A call opens a transaction, and runs the callback inside it |
| A begin | `//txbypass:begins`, or `-begins` | A call returns a transaction, open |
| An end | `//txbypass:closes`, or `-closes` | A call ends every transaction it is given, its receiver included |

The directive goes in the doc comment of a function, a method, or an interface method, or trails an interface method. A flag takes a comma-separated list of names, spelled as `go/types` spells them:

| Kind | Spelling |
| --- | --- |
| A function | `example.com/db.InTx` |
| A method | `(example.com/db.Manager).InTx`. The `*` of a pointer receiver may be written or left out |
| A method of a generic type | `(example.com/db.Store[T]).InTx` |

A name not spelled this way stops the run. A name spelled right that names nothing is not reported.

What a declaration says is read from the signature:

| Part | Read as |
| --- | --- |
| The handle | The receiver of a method. For a function, its first parameter that is neither a `context.Context` nor of a function type. A body of the package that opens every transaction on one parameter, or one field of it, names that instead, as `s.db` for a method of `s` |
| The callback of an invoker | The first parameter of a function type |
| The transaction the callback is given | The callback's first parameter that is not a `context.Context` |
| A callback that takes only a context | The transaction rides in the context, read by [`outer-context`](#outer-context). The outer context is the invoker's first `context.Context` parameter |
| The transaction a begin returns | Its first result. A second result is its error, and has to be an `error` |

A declaration that the signature does not fit is reported, and declares nothing:

| Directive | Reported when |
| --- | --- |
| `invokes` | There is no parameter of a function type, no handle, or a callback that takes neither a transaction nor a context. A callback that takes only a context needs a context parameter of the invoker |
| `begins` | There is no result, more than two, a second that is not an `error`, or no handle |
| `closes` | There is no receiver and no parameter |

A directive declared in one package holds in every package that calls it, through the facts `invokes`, `begins` and `closes`.

## `outer-handle`

In each scope opened on a handle, each expression that names the outer handle is reported. So is each expression that names a handle the scope rebinds with `WithTx`.

## `outer-context`

In the callback of an invoker that carries the transaction in its context, each expression that names the outer context is reported. The outer context is a handle too: `s.ctx` is one.

The handle the invoker is called on is not reported there. With MongoDB, `coll.InsertOne(sc, doc)` runs in the transaction, because `sc` carries it.

The context of a callback that carries a session, as MongoDB's `WithSession` and `UseSession` give, is not reported inside a transaction on that session. `sess.WithTransaction(sc, ...)` runs with `sc` too, so a statement on `sc` runs inside it.

A transaction manager that nests, as go-transaction-manager's `Do` and go-rel's `Transaction`, joins the transaction of the context it is given, or opens a savepoint in it. Inside one given another callback's context, that context is not reported.

The outer context is reported where it is used as a context: given to a call, or with a method of `context.Context` called on it. A web framework's context, as gin's `*gin.Context`, has methods and fields of its own, as `c.JSON(...)` and `c.Request`. They run no statement, and the inner context has none of them, so they are not reported. A method of them that gives a context back is a use, as `c.Request.Context()`. Giving the outer context where a plain context does not fit is no use either. That is a parameter, a field, an element, a channel or a variable of a concrete type, as `*gin.Context`. It is also an interface with methods `context.Context` lacks, as fiber v3's `fiber.Ctx`. MongoDB's session ends with `CommitTransaction`, `AbortTransaction` and `EndSession`. They take a context only for its deadline, so giving them the outer context is no use.

## Messages

Every report of a rule has one shape. A report about a directive has its own, listed in [Directives](#directives):

```text
<use> bypasses the transaction opened with <call>; use <instead> instead [<rule>]
```

| Part | Spelled as |
| --- | --- |
| `<use>` | The variable, then each field, joined by dots, up to the outer handle's depth and without the embedded fields, as the source leaves them out. `db.writer`, promoted from an embedded `dbbase`, is spelled `db.writer`. An embedded field stays when a field of the same name shadows the one it holds: `o.Inner.db` stays `o.Inner.db` when `o` has a `db` of its own. A variable of another package is qualified by its package's name |
| `<call>` | A method by its name, as `Transaction`. A function by its package's name and its own, as `pgx.BeginFunc` |
| `<instead>` | The transaction's variable or parameter. For a handle rebound with `WithTx`, the variable its result is assigned to, or the call as written |

When the callback's parameter is unnamed or `_`:

| Rule | `<instead>` |
| --- | --- |
| `outer-handle` | `the callback's transaction` |
| `outer-context` | `the callback's context` |

## Positions

A use is reported at the start of the expression that names the handle. For `r.db.Create(u)`, that is `r`.

## Directives

`//txbypass:ignore <rules> // <reason>` silences the named rules. It is a line comment: `/*txbypass:ignore*/` is not a directive. A directive on a line of its own silences the line below. A directive that trails code silences its own line only. The rules are comma-separated. With no rules, it silences every rule. Reports about directives themselves cannot be silenced. An ignore aimed at one silences nothing, so it is reported as unused.

| Directive | Result |
| --- | --- |
| `//txbypass:ignore outer-handle // reason` | Silences `outer-handle` there |
| `//txbypass:ignore // reason` | Silences every rule |
| No reason after `//` | Reported: `txbypass:ignore needs a reason after //` |
| An unknown rule name | Reported: `unknown rule x in txbypass:ignore`. The known rules beside it still apply. An ignore whose rules are all unknown is not also reported as unused |
| An empty rule name, as in `outer-handle, // reason` | Reported: `empty rule name in txbypass:ignore`. The known rules beside it still apply |
| An ignore that silences nothing | Reported: `unused txbypass:ignore directive`. An ignore that covers a rule turned off by a flag is not reported: that rule did not run. A bare ignore covers every rule |
| `//txbypass:invokes`, `//txbypass:begins`, `//txbypass:closes` | Declares a helper. See [Declared helpers](#declared-helpers) |
| One of them with text after the name that is not behind `//` | Reported: `txbypass:invokes takes no argument; write a reason after //` |
| One of them anywhere but the doc comment of a function, a method or an interface method | Reported: `txbypass:invokes belongs in the doc comment of a function or an interface method` |
| Any other `//txbypass:` directive | Reported: `unknown directive txbypass:x` |

A directive in a generated file is not read.

## Flags

Each rule has a boolean flag of its name. Every rule is on by default. The golangci-lint plugin takes the same names, as keys of its `settings`. A rule left out keeps its default, and an unknown name is an error.

`-invokes`, `-begins` and `-closes` declare helpers by name. See [Declared helpers](#declared-helpers). The plugin takes each as a list, one name per item.

## Gaps

| Gap | Why |
| --- | --- |
| A callback that is not a function literal | Its body is not read |
| A function literal given to a helper of the project that is not read as one. A helper that opens on a package-level variable is one |
| A helper method with a pointer receiver, called on a struct held by value | The call takes the variable's address. A method of the module given an address may change what it holds, so the variable is no known handle after it | See [Helpers that are invokers](#helpers-that-are-invokers) |
| A function or method the scope calls, which uses the outer handle itself | Only the scope's own code is read for uses |
| An outer handle given by a call, as `open().Transaction(...)` | Each call gives a root of its own |
| A variable assigned two handles on two paths, then used | The phi gives no single handle |
| A scope that writes the outer handle, then uses it before the write | No use of it in the scope is read |
| A begin whose result is kept in a field, or is not kept | The region follows the begin's value |
| A transaction ended through a function value or an interface method | Its body is not known, so the transaction stays open, and a use after it is reported |
| A helper that stores the transaction somewhere, and code elsewhere ends it | Only an end the helper writes counts. A use after the helper is reported |
| A Firestore reference built in the scope, then used to run outside it. `ref := client.Doc(p)` then `ref.Set(ctx, v)` is one | `ref` is a reference, not the client |
| An invoker or a begin called through a method expression or a function value | The selector names no handle, or there is no selector |
| sqlboiler's global executor, Spanner's `NewReadWriteStmtBasedTransaction`, MongoDB's `StartTransaction` | They open no scope this analysis reads |
| xorm's `session.Begin()` ... `session.Commit()` | The session is the transaction, and its begin returns only an error. No value marks where the transaction is, so no region is read. Use `engine.Transaction(...)` for a scope this analysis reads |
| A method of a wrapper that embeds the handle, after a begin promoted from it. `db.MustExec(...)` after `db.Begin()` on a `*sqlx.DB` is one | The begin is opened on the embedded `db.DB`, and `db` is a struct that embeds it. Use sqlx's `Beginx` |
| A use that runs no statement, such as reading the handle's dialect | It is reported, unless it reads a basic value or an error. The transaction gives the same answer, so the fix still applies. A pool's own settings and statistics are not reported: see [Handles](#handles) |
| A hook kept for after the callback, but called inside it too | The literal is not read |
| A hook kept for after a begin's commit, in a region | A region reads every literal it makes. Write an ignore, or open the transaction with a callback |
| A transaction placed in a field of a struct, even a local one, as `job := Job{tx: tx}` | The region ends there, so nothing after it is read. Following the struct to see where the transaction goes would be a guess, and a miss is preferred |
| A begin through bun's `IDB` that holds a `Conn`, then a use of it | It is reported. The type does not say that the value is one dedicated connection |
| A savepoint on a gorm transaction this analysis does not know as one, as a `*gorm.DB` parameter of a declared function | It is reported. The type does not say that the handle is a transaction |
| A helper given the handle and the transaction, as `audit(db, tx)` | Not reported. A call given both keeps the handle beside the transaction, as a constructor does |
| A framework context given to a helper that takes its own type, as `saveOrder(c)` with a `*gin.Context` parameter, which runs a statement with it | Not reported. A parameter of a concrete type takes it as that type, not as a context, and the helper is not read |
| A write on go-redis's `tx` inside `tx.TxPipelined`, in a `Watch` callback | Not reported. The optimistic pattern reads and watches keys on `tx` there on purpose, before `MULTI`. Reading the pipeline of `tx` would report those, with a fix that breaks them |
| A use of the outer context for something other than the database, such as logging | It is reported. The inner context carries the same values, so the fix still applies. A field of the outer context, or a method `context.Context` lacks, as gin's `c.JSON(...)`, is not reported: the inner context has none of them |
