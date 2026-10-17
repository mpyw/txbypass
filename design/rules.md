# Rules

This file specifies what txbypass reports. It is the source of truth for the fixtures under `testdata/src/`. When a rule changes, change this file first, then its fixtures, then the code.

## Scope

txbypass reports a database handle used inside a transaction it opened. It reads names and types inside the callback of a known opener. It follows no values and no calls.

| Rule | What it reports |
| --- | --- |
| `outer-handle` | The handle a transaction was opened on, used inside the callback that runs in it |

### Terms

| Term | Meaning |
| --- | --- |
| Opener | A call that opens a transaction and runs a callback in it. See [Openers](#openers) |
| Handle | A variable, and the fields selected from it. See [Handles](#handles) |
| Outer handle | The handle an opener opens its transaction on |
| Callback | The function literal an opener runs inside the transaction |
| Generated file | A file with a `// Code generated ... DO NOT EDIT.` comment before the package clause, as `ast.IsGenerated` reads it. A file that cgo rewrote is not one |

Nothing is reported in a generated file. Test files are checked like any other file.

## Openers

An opener is recognized by the package path, the receiver's type and the name of the function it calls. An alias of the type does not hide it.

| Package | Receiver | Name | Outer handle | Callback | Transaction |
| --- | --- | --- | --- | --- | --- |
| `gorm.io/gorm` | `*DB` | `Transaction` | The receiver | Argument 0 | Parameter 0 |
| `github.com/uptrace/bun` | `*DB`, `Conn`, `Tx`, `IDB` | `RunInTx` | The receiver | Argument 2 | Parameter 1 |
| `github.com/jackc/pgx/v5` | None | `BeginFunc` | Argument 1 | Argument 2 | Parameter 0 |
| `github.com/jackc/pgx/v5` | None | `BeginTxFunc` | Argument 1 | Argument 3 | Parameter 0 |

A call is not an opener for txbypass when:

| Case | Why |
| --- | --- |
| The callback is not a function literal | Its body is not read |
| The receiver is given as an argument, through a method expression | The arguments are shifted, and the selector names no handle |
| The outer handle names no handle | Nothing inside can be compared with it |

A method promoted from an embedded field is an opener of that field. `s.Transaction(fn)`, where `s` embeds `*gorm.DB`, opens the transaction on `s.DB`.

## Handles

A handle is named by one of these:

| Expression | Handle |
| --- | --- |
| A variable, local or package-level | The variable |
| A qualified identifier of a variable, as `dbs.Main` | The variable |
| `x.f`, a field | The handle of `x`, then `f` |
| `x.m`, a method value | The handle of `x`, then each embedded field the promotion of `m` passes |
| `x.m(...)`, a call of a method whose result has the type of `x` | The handle of `x` |
| `(e)`, `*e` | The handle of `e` |

Anything else names no handle. That includes a call that returns another type, a method expression, a composite literal's key, and a type.

Two handles are the same when they start from the same variable and select the same fields. A parameter of the callback with the outer handle's name is another variable. So is a copy, as in `d := db`.

## `outer-handle`

Inside the callback, each expression that names the outer handle is reported. The callback's own function literals are inside it. Once an expression is reported, nothing inside it is read again.

The message names the handle, the opener and the transaction:

| Case | Message |
| --- | --- |
| The transaction parameter has a name | `<handle> bypasses the transaction it opened with <opener>; use <name> instead` |
| It is unnamed or `_` | `<handle> bypasses the transaction it opened with <opener>; use the callback's transaction instead` |

| Part | Spelled as |
| --- | --- |
| `<handle>` | The variable, then each field, joined by dots. A variable of another package is qualified by its package's name |
| `<opener>` | A method by its name, as `Transaction`. A function by its package's name and its own, as `pgx.BeginFunc` |

A position is reported once. A use inside two transactions opened on the same handle is reported for the outer one.

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
| An ignore that silences nothing | Reported: `unused txbypass:ignore directive`. An ignore whose every rule is turned off by a flag is not reported |
| Any other `//txbypass:` directive | Reported: `unknown directive txbypass:x` |

A directive in a generated file is not read.

## Flags

Each rule has a boolean flag of its name. Every rule is on by default. The golangci-lint plugin takes the same names, as keys of its `settings`. A rule left out keeps its default, and an unknown name is an error.

## Gaps

| Gap | Why |
| --- | --- |
| A callback that is not a function literal | Its body is not read |
| A function or method the callback calls, which uses the outer handle itself | Only the callback's own body is read. Following calls needs facts across packages |
| A copy of the outer handle, made before the transaction | Names are compared, not values |
| A field of a struct that holds the handle, read through another variable. `rr := r` then `rr.db` is one | The variable differs |
| An outer handle given by a call, as `open().Transaction(...)` | It is named from no variable |
| A transaction from `Begin` to `Commit` or `Rollback` | Only openers that take a callback are known |
| An opener called through a method expression or a function value | The selector names no handle, or there is no selector |
| pgx v4, database/sql, sqlx, ent, sqlc, bob, and others | Their openers are not in the table yet |
| A use that runs no statement, such as reading the handle's dialect | It is reported. The transaction gives the same answer, so the fix still applies |
