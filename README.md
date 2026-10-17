<div align="center">
  <img src=".github/assets/icon.png" alt="" width="128" height="128">
  <h1>txbypass</h1>

  [![CI](https://github.com/mpyw/txbypass/actions/workflows/ci.yml/badge.svg)](https://github.com/mpyw/txbypass/actions/workflows/ci.yml)
  [![Codecov](https://codecov.io/gh/mpyw/txbypass/graph/badge.svg)](https://codecov.io/gh/mpyw/txbypass)
  [![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/txbypass.svg)](https://pkg.go.dev/github.com/mpyw/txbypass)
</div>

Go linter that reports a database handle used inside a transaction it opened.

## Overview

```go
package app

import (
	"context"

	"gorm.io/gorm"
)

func (r *OrderRepo) Place(ctx context.Context, o *Order) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		return r.db.Create(&AuditLog{OrderID: o.ID}).Error
	})
}
```

```console
$ txbypass ./...
order.go:14:10: r.db bypasses the transaction opened with Transaction; use tx instead [outer-handle]
```

The callback receives the transaction as `tx`. `r.db` is the pool the transaction came from, so the audit log is written outside the transaction.

| What goes wrong | Why |
| --- | --- |
| The write survives a rollback | It was never part of the transaction |
| The write can wait forever | It runs on another connection, and may need a lock the transaction holds |
| The write cannot see the transaction's rows | They are not committed yet |

The code compiles. A test where nothing fails passes too. txbypass finds it from the source.

Every report ends with the name of its rule. That name is what `//txbypass:ignore` takes.

## Install

| Method | Command | Needs |
| --- | --- | --- |
| **[mise](https://mise.jdx.dev/)** *(recommended)* | `mise use "github:mpyw/txbypass@0.1.0"` | Nothing. Installs the prebuilt binary |
| `go tool` | `go get -tool github.com/mpyw/txbypass/cmd/txbypass@latest` | Go 1.27+ |
| `go install` | `go install github.com/mpyw/txbypass/cmd/txbypass@latest` | Go 1.27+ |
| Release archive | See below | Nothing |

```bash
txbypass ./...
txbypass -V=full   # the release this binary was built from
```

<details>
<summary>Pin a version, run through <code>go vet</code> or golangci-lint, or install from an archive</summary>

`mise use` pins the version in the project's `mise.toml`, so every checkout and CI run the same one. Add `-g` to install it for every project on your machine instead.

```toml
[tools]
"github:mpyw/txbypass" = "0.1.0"
```

As a tool dependency in `go.mod`:

```bash
go get -tool github.com/mpyw/txbypass/cmd/txbypass@latest
go tool txbypass ./...
```

Through `go vet`, which runs it with the same package loading as the rest of your vet checks, and caches it:

```bash
go vet -vettool=$(which txbypass) ./...
```

Without installing anything:

```bash
go run github.com/mpyw/txbypass/cmd/txbypass@latest ./...
```

From a release archive, verified against the published checksums:

```bash
VERSION=0.1.0
curl -LO "https://github.com/mpyw/txbypass/releases/download/v${VERSION}/txbypass_${VERSION}_darwin_arm64.tar.gz"
curl -LO "https://github.com/mpyw/txbypass/releases/download/v${VERSION}/checksums.txt"
shasum -a 256 -c checksums.txt --ignore-missing
tar xzf "txbypass_${VERSION}_darwin_arm64.tar.gz"
```

Inside golangci-lint, as a [module plugin](https://golangci-lint.run/plugins/module-plugins/). txbypass is not bundled with golangci-lint, so build a binary that holds it. Write `.custom-gcl.yml`:

```yaml
version: v2.13.1  # the golangci-lint release to build
plugins:
  - module: github.com/mpyw/txbypass
    import: github.com/mpyw/txbypass/plugin
    version: v0.1.0
```

Turn it on in `.golangci.yml`. Each key under `settings` is a rule's name, as the flags take it. A rule left out keeps its default. An unknown name stops the run:

```yaml
version: "2"
linters:
  enable:
    - txbypass
  settings:
    custom:
      txbypass:
        type: module
        description: Reports a database handle used inside a transaction it opened.
        settings:
          outer-handle: true
```

Then build and run it:

```bash
golangci-lint custom  # writes ./custom-gcl
./custom-gcl run ./...
```

</details>

## Rules

| Rule | Default | Reports |
| --- | --- | --- |
| [`outer-handle`](#outer-handle) | 🟢 On | The handle a transaction was opened on, used while the transaction is open<br>→ Use the transaction |
| [`outer-context`](#outer-context) | 🟢 On | The outer context, used inside a callback whose context carries the transaction<br>→ Use the callback's context |

Each rule has a flag of its name:

```bash
txbypass -outer-context=false ./...  # every rule except outer-context
```

Nothing is reported in a generated file. Test files are checked like any other file.

### Supported libraries

| Library | Callback | Begin ... Commit |
| --- | --- | --- |
| [database/sql](https://pkg.go.dev/database/sql) | | `Begin`, `BeginTx` |
| [sqlx](https://github.com/jmoiron/sqlx) | | `Beginx`, `BeginTxx`, `MustBegin`, `MustBeginTx` |
| [pgx](https://github.com/jackc/pgx) v5 | `pgx.BeginFunc`, `pgx.BeginTxFunc` | `Begin`, `BeginTx`, and `tx.Begin` for a savepoint |
| [pgx](https://github.com/jackc/pgx) v4 | `BeginFunc`, `BeginTxFunc` | The same as v5 |
| [gorm](https://gorm.io) | `Transaction` | `Begin` |
| [bun](https://bun.uptrace.dev) | `RunInTx` | `BeginTx` |
| [bob](https://github.com/stephenafamo/bob) | `RunInTx` | `BeginTx` |
| [ent](https://entgo.io) | | `client.Tx`, `client.BeginTx` |
| [goqu](https://github.com/doug-martin/goqu) | `WithTx` | `Begin`, `BeginTx` |
| [go-pg](https://github.com/go-pg/pg) | `RunInTransaction` | `Begin`, `BeginContext` |
| [pop](https://github.com/gobuffalo/pop) | `Transaction`, `Rollback` | `NewTransaction`, `NewTransactionContext`, `NewTransactionContextOptions` |
| [upper/db](https://github.com/upper/db) | `Tx`, `TxContext` | |
| [xorm](https://xorm.io) | `Transaction` | |
| [go-redis](https://github.com/redis/go-redis) v9, v8 | `Watch`, `TxPipelined` | |
| [Firestore](https://pkg.go.dev/cloud.google.com/go/firestore) | `RunTransaction` | |
| [Spanner](https://pkg.go.dev/cloud.google.com/go/spanner) | `ReadWriteTransaction`, `ReadWriteTransactionWithOptions` | |
| [Datastore](https://pkg.go.dev/cloud.google.com/go/datastore) | `RunInTransaction` | `NewTransaction` |
| [MongoDB](https://github.com/mongodb/mongo-go-driver) v2, v1 | `WithTransaction`, `mongo.WithSession`, read by `outer-context` | |
| [go-rel](https://github.com/go-rel/rel) | `Transaction`, read by `outer-context` | |
| [go-transaction-manager](https://github.com/avito-tech/go-transaction-manager) v2 | `Do`, `DoWithSettings`, read by `outer-context` | |

Query builders that take the handle as an argument, such as [jet](https://github.com/go-jet/jet) and [squirrel](https://github.com/Masterminds/squirrel), need nothing of their own. Passing the outer handle to them is a use. [sqlc](https://sqlc.dev) is covered through [`WithTx`](#sqlcs-withtx).

The exact receivers and signatures are in [design/rules.md](design/rules.md#callback-openers).

### `outer-handle`

Inside a transaction, every use of the handle it was opened on is reported. The transaction can be opened in two ways.

<table>
<tr><th>Opened by</th><th>Inside means</th><th>Example</th></tr>
<tr>
<td>

A callback

</td>
<td>

The callback's body. Its goroutines and deferred calls count too. They run while the transaction is open

</td>
<td>

```go
return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, debit); err != nil {
		return err
	}
	// pool bypasses the transaction opened with pgx.BeginFunc; use tx instead
	_, err := pool.Exec(ctx, credit)
	return err
})
```

</td>
</tr>
<tr>
<td>

A begin

</td>
<td>

Where the transaction is open on every path: after the begin, and before its commit or rollback

</td>
<td>

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
	return err // the transaction did not begin here
}
defer tx.Rollback()
if _, err := tx.ExecContext(ctx, debit); err != nil {
	return err
}
// db bypasses the transaction opened with BeginTx; use tx instead
if _, err := db.ExecContext(ctx, credit); err != nil {
	return err
}
return tx.Commit()
```

</td>
</tr>
</table>

A handle is a variable and the fields selected from it. Two expressions name the same handle when they start from the same variable and select the same fields.

<table>
<tr><th>Inside the transaction</th><th>Reported</th><th>Why</th></tr>
<tr>
<td>

```go
db.Transaction(func(tx *gorm.DB) error {
	db.Create(u)
	...
```

</td>
<td>

✅ Yes

</td>
<td>

`db` opened the transaction

</td>
</tr>
<tr>
<td>

```go
db.WithContext(ctx).Create(u)
```

</td>
<td>

✅ Yes

</td>
<td>

A method that returns its receiver's type gives the same handle back

</td>
</tr>
<tr>
<td>

```go
audit(db)
stmt.Query(db, &dest) // go-jet
```

</td>
<td>

✅ Yes

</td>
<td>

Passing the handle on is a use

</td>
</tr>
<tr>
<td>

```go
r.db.Transaction(func(tx *gorm.DB) error {
	r.db.Save(u)
	...
```

</td>
<td>

✅ Yes

</td>
<td>

A field selected from the same variable is the same handle

</td>
</tr>
<tr>
<td>

```go
type Store struct{ *gorm.DB }

s.Transaction(func(tx *gorm.DB) error {
	s.Create(u)
	...
```

</td>
<td>

✅ Yes

</td>
<td>

A value is the handle it embeds

</td>
</tr>
<tr>
<td>

```go
dbs.Main.Transaction(func(tx *gorm.DB) error {
	dbs.Main.Create(u)
	...
```

</td>
<td>

✅ Yes

</td>
<td>

A variable of another package is a handle too

</td>
</tr>
<tr>
<td>

```go
tx.Transaction(func(sp *gorm.DB) error {
	tx.Create(u)
	...
```

</td>
<td>

✅ Yes

</td>
<td>

A savepoint is a transaction too

</td>
</tr>
<tr>
<td>

```go
db.Transaction(func(tx *gorm.DB) error {
	logs.Create(u)
	...
```

</td>
<td>

❌ No

</td>
<td>

`logs` is another handle

</td>
</tr>
<tr>
<td>

```go
r.db.Transaction(func(tx *gorm.DB) error {
	r.logs.Save(u)
	...
```

</td>
<td>

❌ No

</td>
<td>

`r.logs` is another field

</td>
</tr>
<tr>
<td>

```go
db.Transaction(func(db *gorm.DB) error {
	db.Create(u)
	...
```

</td>
<td>

❌ No

</td>
<td>

The parameter shadows the outer `db`

</td>
</tr>
</table>

<table>
<tr><th>In a begin's function</th><th>Reported</th><th>Why</th></tr>
<tr>
<td>

```go
tx, err := db.Begin()
if err != nil {
	db.Exec(logFailure)
	return err
}
```

</td>
<td>

❌ No

</td>
<td>

The transaction did not begin

</td>
</tr>
<tr>
<td>

```go
if err := tx.Commit(); err != nil {
	return err
}
db.Exec(refresh)
```

</td>
<td>

❌ No

</td>
<td>

The transaction has ended

</td>
</tr>
<tr>
<td>

```go
if done {
	tx.Commit()
}
db.Exec(refresh)
```

</td>
<td>

❌ No

</td>
<td>

It may have ended

</td>
</tr>
<tr>
<td>

```go
defer db.Exec(audit)
```

</td>
<td>

❌ No

</td>
<td>

It runs when the function returns

</td>
</tr>
<tr>
<td>

```go
go func() {
	db.Exec(audit)
}()
```

</td>
<td>

✅ Yes

</td>
<td>

It starts while the transaction is open

</td>
</tr>
</table>

#### Firestore references

Firestore's client builds the references a transaction reads and writes. Building one runs nothing, so only a use that runs is reported:

<table>
<tr><th>Inside <code>RunTransaction</code></th><th>Reported</th><th>Why</th></tr>
<tr>
<td>

```go
tx.Get(client.Doc("users/a"))
```

</td>
<td>

❌ No

</td>
<td>

The chain of calls on `client` takes no context. It only builds a reference

</td>
</tr>
<tr>
<td>

```go
ref := client.Collection("users").Doc("b")
```

</td>
<td>

❌ No

</td>
<td>

The same

</td>
</tr>
<tr>
<td>

```go
client.Doc("users/c").Set(ctx, v)
```

</td>
<td>

✅ Yes

</td>
<td>

A call in the chain takes a context, so it runs

</td>
</tr>
<tr>
<td>

```go
save(ctx, client)
```

</td>
<td>

✅ Yes

</td>
<td>

The client on its own is given to a function

</td>
</tr>
</table>

#### sqlc's `WithTx`

`q.WithTx(tx)` rebinds sqlc's generated `Queries` to the transaction. Code that calls it means to run on the transaction, so every other use of `q` there is reported:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
	return err
}
defer tx.Rollback()
qtx := q.WithTx(tx)
if err := qtx.CreateUser(ctx, a); err != nil {
	return err
}
if err := q.CreateUser(ctx, b); err != nil { // q bypasses the transaction opened with BeginTx; use qtx instead
	return err
}
return tx.Commit()
```

Any method named `WithTx` qualifies, when it takes the transaction and returns its receiver's type.

> [!TIP]
> Some writes must survive a rollback, such as an audit log of a failed attempt. Write an ignore with the reason:
>
> ```go
> //txbypass:ignore outer-handle // the audit log must survive a rollback
> db.Create(&AuditLog{Action: "failed"})
> ```

#### Fix

Use the transaction:

```go
return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
	if err := tx.Create(o).Error; err != nil {
		return err
	}
	return tx.Create(&AuditLog{OrderID: o.ID}).Error
})
```

A helper that takes the handle takes the transaction instead:

<table>
<tr><th>Library</th><th>A helper can take</th></tr>
<tr>
<td>

database/sql

</td>
<td>

```go
// *sql.DB and *sql.Tx both have these.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

</td>
</tr>
<tr>
<td>

gorm

</td>
<td>

`*gorm.DB`. The transaction has the handle's type

</td>
</tr>
<tr>
<td>

bun

</td>
<td>

[`bun.IDB`](https://pkg.go.dev/github.com/uptrace/bun#IDB). `*bun.DB`, `bun.Conn` and `bun.Tx` implement it

</td>
</tr>
<tr>
<td>

pgx

</td>
<td>

```go
// A pool, a connection and a transaction all have these.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
```

</td>
</tr>
<tr>
<td>

sqlc

</td>
<td>

`DBTX`, which sqlc generates. Or `q.WithTx(tx)`

</td>
</tr>
</table>

### `outer-context`

Some openers carry the transaction in the callback's context, not in a value. A statement run with the outer context runs outside the transaction:

```go
_, err := sess.WithTransaction(ctx, func(sc context.Context) (any, error) {
	if _, err := orders.InsertOne(sc, o); err != nil {
		return nil, err
	}
	return audits.InsertOne(ctx, a) // ctx bypasses the transaction opened with WithTransaction; use sc instead
})
```

<table>
<tr><th>Inside the callback</th><th>Reported</th><th>Why</th></tr>
<tr>
<td>

```go
sess.WithTransaction(ctx, func(sc context.Context) (any, error) {
	return coll.InsertOne(ctx, doc)
	...
```

</td>
<td>

✅ Yes

</td>
<td>

`ctx` was given to the opener, and carries no transaction

</td>
</tr>
<tr>
<td>

```go
sess.WithTransaction(ctx, func(sc context.Context) (any, error) {
	return coll.InsertOne(sc, doc)
	...
```

</td>
<td>

❌ No

</td>
<td>

`sc` carries the transaction

</td>
</tr>
<tr>
<td>

```go
sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
	return coll.InsertOne(ctx, doc)
	...
```

</td>
<td>

❌ No

</td>
<td>

The parameter shadows the outer `ctx`

</td>
</tr>
<tr>
<td>

```go
repo.Transaction(ctx, func(txCtx context.Context) error {
	return repo.Insert(txCtx, u)
	...
```

</td>
<td>

❌ No

</td>
<td>

The repository is not the handle here. The context decides the transaction

</td>
</tr>
</table>

#### Fix

Use the callback's context. Name it as the outer one, so the outer one cannot be reached:

```go
_, err := sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
	if _, err := orders.InsertOne(ctx, o); err != nil {
		return nil, err
	}
	return audits.InsertOne(ctx, a)
})
```

## Agent skill

[`skills/txbypass-authoring`](skills/txbypass-authoring/SKILL.md) is a skill for an AI agent writing code under txbypass. It says how to fix each report, which fixes only hide a problem, and when an ignore is right. The binary carries it:

```bash
txbypass skill install                                # the agents already set up in this project
txbypass skill install --agent claude-code --scope user
txbypass skill list                                   # where it is, and whether it is current
```

Without the binary, `gh skill install mpyw/txbypass txbypass-authoring --agent claude-code` writes to the same directories. The installer is [go-skill-embed](https://github.com/mpyw/go-skill-embed).

## Ignoring a report

Write `//txbypass:ignore` with the rules to silence and a reason after `//`.

```go
//txbypass:ignore outer-handle // the audit log must survive a rollback
db.Create(&AuditLog{Action: "failed"})
```

| Placement | Silences |
| --- | --- |
| On a line of its own | The line below |
| After code | Its own line |

| Directive | Result |
| --- | --- |
| `//txbypass:ignore outer-handle // reason` | Silences `outer-handle` |
| `//txbypass:ignore // reason` | Silences every rule |
| No reason | Reported, and silences nothing |
| An unknown rule name | Reported |
| An ignore that silences nothing | Reported as unused, unless every rule it names is turned off |

## Limits

These are not checked:

<table>
<tr><th>Not checked</th><th>Why</th></tr>
<tr>
<td>

```go
db.Transaction(r.place)
```

</td>
<td>

A callback that is not a function literal has no body here to read

</td>
</tr>
<tr>
<td>

```go
db.Transaction(func(tx *gorm.DB) error {
	return r.audit(ctx) // audit uses r.db
})
```

</td>
<td>

Only the code inside the transaction is read, not the functions it calls

</td>
</tr>
<tr>
<td>

```go
d := db
db.Transaction(func(tx *gorm.DB) error {
	d.Create(u)
	...
```

</td>
<td>

Names are compared, not values

</td>
</tr>
<tr>
<td>

```go
open().Transaction(...)
```

</td>
<td>

The handle is named from no variable

</td>
</tr>
<tr>
<td>

```go
r.tx, err = db.Begin()
```

</td>
<td>

The region follows a variable, not a field

</td>
</tr>
<tr>
<td>

```go
ref := client.Doc(p)
ref.Set(ctx, v)
```

</td>
<td>

Only the chain on Firestore's client is read

</td>
</tr>
<tr>
<td>

```go
(*gorm.DB).Transaction(db, fn)
```

</td>
<td>

A method expression takes its receiver as an argument

</td>
</tr>
</table>

This is reported although it may be fine:

<table>
<tr><th>Reported, but may be fine</th><th>Instead</th></tr>
<tr>
<td>

```go
tx, err := db.Begin()
...
finish(tx, err) // commits or rolls back
db.Exec(refresh)
```

</td>
<td>

Commit in the function, or write an ignore

</td>
</tr>
</table>

The full specification, with every limit, is [design/rules.md](design/rules.md).

## License

[MIT](LICENSE)

The Go gopher was designed by Renée French, under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
