---
name: txbypass-authoring
description: "Write or change Go code in a repository that runs txbypass, which shows as txbypass in CI or a mise.toml, or as //txbypass:ignore comments. Read this before acting on a txbypass report. Read it too before writing code inside a database transaction: a callback such as gorm's Transaction or MongoDB's WithTransaction, or the code between BeginTx and Commit. Covers how to fix each report, the fixes that only hide a problem, and when an ignore is right."
license: MIT
---

# Writing code under txbypass

Written against **txbypass 0.1.0**. Check the version first with `txbypass -V=full`: this describes how that release behaves, not how an older one does.

txbypass reports code that runs outside a transaction while the transaction is open. The full specification is `design/rules.md` in the txbypass repository. This skill says how to write code that satisfies it.

## The model

**While a transaction is open, every statement goes through the transaction.** The handle the transaction was opened on is a pool, or another connection. A statement run through it is not part of the transaction.

| Rule | Default | Reports | The fix in one line |
| --- | --- | --- | --- |
| `outer-handle` | On | The handle a transaction was opened on, used while it is open | Use the transaction |
| `outer-context` | On | The outer context, inside a callback whose context carries the transaction | Use the callback's context |

A transaction is open in two places:

| Opened by | Open in | Use |
| --- | --- | --- |
| A callback, as `db.Transaction(func(tx *gorm.DB) error { ... })` | The callback's body, with its goroutines and deferred calls | The callback's transaction |
| A begin, as `tx, err := db.BeginTx(ctx, nil)` | From the begin to its commit or rollback, on every path | `tx` |
| A callback whose context carries the transaction, as MongoDB's `sess.WithTransaction(ctx, func(sc context.Context) ...)` | The callback's body | The callback's context, `sc` |

The README lists every library and call txbypass knows.

## Fixing a report

| The report is on | Fix |
| --- | --- |
| A statement, as `db.Create(x)` | Run it on the transaction: `tx.Create(x)` |
| A helper call that takes the handle, as `audit(db)` or jet's `stmt.Query(db, &dest)` | Pass the transaction. Widen the helper's parameter when the types differ |
| sqlc's `q`, in code that calls `q.WithTx(tx)` | Use the rebound queries, `qtx` |
| A goroutine inside the transaction | Use the transaction, and make sure the goroutine ends before the commit. One that outlives it uses a closed transaction |
| A nested transaction opened on the outer handle | Open it on the transaction, as `tx.Transaction(...)` or `tx.Begin(ctx)`. That is a savepoint |
| A Firestore `client.Doc(p).Set(ctx, v)` inside `RunTransaction` | Write through the transaction: `tx.Set(client.Doc(p), v)`. Building the reference with the client is fine |
| The outer `ctx`, inside MongoDB's, go-rel's or go-transaction-manager's callback | Use the callback's context. Name it `ctx`, so the outer one cannot be reached |

A helper parameter that takes either the handle or the transaction:

| Library | Type |
| --- | --- |
| database/sql | An interface of your own with `ExecContext` and `QueryContext` |
| gorm | `*gorm.DB` |
| bun | `bun.IDB` |
| pgx | An interface of your own with `Exec`, `Query` and `QueryRow` |
| sqlc | `DBTX`, or call `q.WithTx(tx)` |

## Fixes that only hide a problem

| Do not | Why |
| --- | --- |
| Move the statement into a helper that reads `r.db` itself | Only the transaction's own code is read for uses. The report stops, and the bug stays |
| Keep the transaction in a field, as `r.tx, err = db.Begin()` | Only a begin kept in a variable opens a region, so nothing is read. The bug stays |
| Store the transaction somewhere, so that the region ends | Storing it counts as a possible end, so the report stops, and the bug stays |
| Pass the transaction beside the handle, as `audit(db, tx)` | A call given both keeps the handle beside the transaction, as a constructor does. The report stops, and the bug stays if the helper uses `db` |
| Commit in a branch, then go on with the handle | Code after a branch that may commit is not read. Commit where the function ends |
| Build a Firestore reference into a variable, then run it outside the transaction | `ref := client.Doc(p)` then `ref.Set(ctx, v)` is not read, and still runs outside |
| Write an ignore without saying why the statement must run outside | The next reader cannot tell a decision from a shortcut |

## When an ignore is right

Only when the statement must not be part of the transaction:

| Case | Example |
| --- | --- |
| A record that must survive a rollback | An audit log of a failed attempt |
| A statement the database forbids inside a transaction | `CREATE INDEX CONCURRENTLY` in PostgreSQL |
| A function value or an interface method that ends the transaction, before more work on the handle | `end(tx)`, where `end` is a parameter, then a cache refresh through `db` |

```go
//txbypass:ignore outer-handle // the audit log must survive a rollback
db.Create(&AuditLog{Action: "failed"})
```

> [!WARNING]
> A statement on the outer handle can wait on a lock the transaction holds. With a pool of one connection, it waits forever. Check this before you ignore a report.

The reason after `//` is required. An ignore that silences nothing is reported.

## What txbypass does not see

A clean run does not prove that no statement bypasses a transaction:

| Not seen | Example |
| --- | --- |
| A callback that is not a function literal | `db.Transaction(r.place)` |
| A callback given to a transaction helper of your own that may run it outside the transaction, or keeps it | `s.reuse(ctx, cur, func(tx *sql.Tx) error { ... })`, where `reuse` runs `fn(cur)` when `cur` is set |
| A function the transaction's code calls | `r.audit(ctx)`, where `audit` uses `r.db` |
| A transaction kept in a field | `r.tx, err = db.Begin()` |
| A transaction stored, or sent away | `h.tx = tx`, `ch <- tx` |
| A call given the transaction too | `audit(db, tx)`, `newStore(s.db, tx)` |
| A library or a call txbypass does not know | A begin helper of your own that wraps the transaction in a struct, sqlboiler's global executor |

Write such code as if txbypass read it.
