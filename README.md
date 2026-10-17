<div align="center">
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
order.go:14:10: r.db bypasses the transaction it opened with Transaction; use tx instead [outer-handle]
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
| [`outer-handle`](#outer-handle) | 🟢 On | The handle a transaction was opened on, used inside the callback that runs in it<br>→ Use the transaction the callback receives |

Each rule has a flag of its name:

```bash
txbypass -outer-handle=false ./...  # turn outer-handle off
```

Nothing is reported in a generated file. Test files are checked like any other file.

### `outer-handle`

A call that opens a transaction and runs a callback in it is an **opener**. txbypass knows these openers:

| Library | Opener | The handle | The transaction |
| --- | --- | --- | --- |
| [gorm](https://gorm.io) | [`(*gorm.DB).Transaction`](https://pkg.go.dev/gorm.io/gorm#DB.Transaction) | The receiver | The callback's parameter |
| [bun](https://bun.uptrace.dev) | `RunInTx` of [`*bun.DB`](https://pkg.go.dev/github.com/uptrace/bun#DB.RunInTx), [`bun.Conn`](https://pkg.go.dev/github.com/uptrace/bun#Conn.RunInTx), [`bun.Tx`](https://pkg.go.dev/github.com/uptrace/bun#Tx.RunInTx) and [`bun.IDB`](https://pkg.go.dev/github.com/uptrace/bun#IDB) | The receiver | The callback's second parameter |
| [pgx v5](https://github.com/jackc/pgx) | [`pgx.BeginFunc`](https://pkg.go.dev/github.com/jackc/pgx/v5#BeginFunc), [`pgx.BeginTxFunc`](https://pkg.go.dev/github.com/jackc/pgx/v5#BeginTxFunc) | The second argument | The callback's parameter |

Inside the callback, every use of the handle is reported. The callback's own function literals count too, such as a goroutine or a deferred call. They run while the transaction is open.

```go
return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, debit); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, credit) // pool bypasses the transaction it opened with pgx.BeginFunc; use tx instead
	return err
})
```

A handle is a variable and the fields selected from it. Two expressions name the same handle when they start from the same variable and select the same fields.

| Inside the callback | Opened on | Reported |
| --- | --- | --- |
| `db.Create(u)` | `db` | Yes |
| `db.WithContext(ctx).Create(u)` | `db` | Yes. A method that returns its receiver's type gives the same handle back |
| `audit(db)` | `db` | Yes. Passing the handle on is a use |
| `r.db.Save(u)` | `r.db` | Yes |
| `s.Create(u)`, where `s` embeds `*gorm.DB` | `s` | Yes, as `s.DB` |
| `dbs.Main.Create(u)` | `dbs.Main`, a variable of another package | Yes |
| `tx.Create(u)` inside a savepoint opened with `tx.Transaction` | `tx` | Yes. The savepoint is a transaction too |
| `logs.Create(u)` | `db` | No. It is another handle |
| `r.logs.Save(u)` | `r.db` | No. It is another field |
| `db.Create(u)`, where the callback's parameter is also named `db` | `db` | No. The parameter shadows the handle |

> [!TIP]
> Some writes must survive a rollback, such as an audit log of a failed attempt. Write an ignore with the reason:
>
> ```go
> //txbypass:ignore outer-handle // the audit log must survive a rollback
> db.Create(&AuditLog{Action: "failed"})
> ```

#### Fix

Use the transaction the callback receives:

```go
return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
	if err := tx.Create(o).Error; err != nil {
		return err
	}
	return tx.Create(&AuditLog{OrderID: o.ID}).Error
})
```

A helper that takes the handle takes the transaction instead:

| Library | A helper can take |
| --- | --- |
| gorm | `*gorm.DB`. The transaction has the handle's type |
| bun | [`bun.IDB`](https://pkg.go.dev/github.com/uptrace/bun#IDB). `*bun.DB`, `bun.Conn` and `bun.Tx` implement it |
| pgx | An interface of its own with `Exec`, `Query` and `QueryRow`. A pool, a connection and a transaction all have them |

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

| Case | Why |
| --- | --- |
| A callback that is not a function literal, as in `db.Transaction(r.place)` | Its body is not read |
| A function the callback calls, as in `r.audit(ctx)` where `audit` uses `r.db` | Only the callback's own body is read |
| A copy of the handle, as in `d := db` before the transaction | Names are compared, not values |
| A handle given by a call, as in `open().Transaction(...)` | It is named from no variable |
| A transaction opened without a callback, from `Begin` to `Commit` | Only openers that take a callback are known |
| A method expression, as in `(*gorm.DB).Transaction(db, fn)` | Its receiver is an argument |
| pgx v4, sqlx, ent, sqlc, and other libraries | Their openers are not known yet |

The full specification, with every limit, is [design/rules.md](design/rules.md).

## License

[MIT](LICENSE)
