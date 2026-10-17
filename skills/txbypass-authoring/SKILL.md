---
name: txbypass-authoring
description: "Write or change Go code in a repository that runs txbypass, which shows as txbypass in CI or a mise.toml, or as //txbypass:ignore comments. Read this before acting on a txbypass report. Read it too before writing code inside a gorm Transaction, a bun RunInTx, or a pgx BeginFunc callback. Covers how to fix each report, the fixes that only hide a problem, and when an ignore is right."
license: MIT
---

# Writing code under txbypass

Written against **txbypass 0.1.0**. Check the version first with `txbypass -V=full`: this describes how that release behaves, not how an older one does.

txbypass reports a database handle used inside a transaction it opened. The full specification is `design/rules.md` in the txbypass repository. This skill says how to write code that satisfies it.

## The model

**Inside a transaction's callback, every statement goes through the transaction the callback receives.** The handle the transaction was opened on is a pool, or another connection. A statement run through it is not part of the transaction.

| Rule | Default | Reports | The fix in one line |
| --- | --- | --- | --- |
| `outer-handle` | On | The handle a transaction was opened on, used inside its callback | Use the callback's transaction |

| Library | Opener | Use inside the callback |
| --- | --- | --- |
| gorm | `db.Transaction(func(tx *gorm.DB) error { ... })` | `tx` |
| bun | `db.RunInTx(ctx, opts, func(ctx context.Context, tx bun.Tx) error { ... })` | `tx` |
| pgx v5 | `pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { ... })` | `tx` |

## Fixing a report

| The report is on | Fix |
| --- | --- |
| A statement, as `db.Create(x)` | Run it on the transaction: `tx.Create(x)` |
| A helper call that takes the handle, as `audit(db)` | Pass the transaction: `audit(tx)`. Widen the helper's parameter when the types differ |
| A goroutine or a deferred call inside the callback | Use the transaction, and make sure it finishes before the callback returns. A goroutine that outlives the callback uses a closed transaction |
| A nested transaction opened on the outer handle | Open it on the transaction, as `tx.Transaction(...)`. That is a savepoint |
| The handle of an outer transaction, inside a savepoint | Use the savepoint's parameter |

A helper parameter that takes either:

| Library | Type |
| --- | --- |
| gorm | `*gorm.DB` |
| bun | `bun.IDB` |
| pgx | An interface of your own with `Exec`, `Query` and `QueryRow` |

## Fixes that only hide a problem

| Do not | Why |
| --- | --- |
| Copy the handle first, as `d := db`, and use `d` | It is the same pool. txbypass compares names, so it stops reporting, and the bug stays |
| Move the statement into a helper that reads `r.db` itself | Same as above. Only the callback's own body is read |
| Name the callback's parameter after the handle, then use the outer one some other way | The parameter shadows the handle. Use that parameter for every statement |
| Write an ignore without saying why the statement must run outside | The next reader cannot tell a decision from a shortcut |

## When an ignore is right

Only when the statement must not be part of the transaction:

| Case | Example |
| --- | --- |
| A record that must survive a rollback | An audit log of a failed attempt |
| A statement the database forbids inside a transaction | `CREATE INDEX CONCURRENTLY` in PostgreSQL |

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
| A function the callback calls | `r.audit(ctx)`, where `audit` uses `r.db` |
| A copy of the handle | `d := db` before the transaction |
| A transaction without a callback | `tx := db.Begin()` ... `tx.Commit()` |
| Other libraries | pgx v4, sqlx, ent, sqlc |

Write such code as if txbypass read it.
