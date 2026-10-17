# Implementation notes

This file records design decisions and implementation details. The short, always-loaded instructions are in [AGENTS.md](../AGENTS.md). What txbypass reports is specified in [rules.md](rules.md).

## Project overview

**txbypass** is a Go linter that reports a database handle used inside a transaction it opened. It is built on [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis) and the [`inspect`](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/inspect) pass.

It reads names and types inside a transaction's scope: a callback's body, or the region where a begun transaction is open. It has no analysis facts, so it does not analyze dependencies.

## Architecture

```text
analyzer.go              Analyzer, one flag per rule
skills.go                Skills: the embedded skills/ directory
plugin/                  the golangci-lint module plugin: the settings in place of the flags
skills/                  the agent skills, installed by `txbypass skill install`
cmd/txbypass/            singlechecker entry point, the skill subcommand, and -V=full
internal/                the rules: one flat package, one namespace per file
  run.go                 Run: every call and function in the package, then the reports in order
  checker.go             the per-pass state, and reporting through the directives
  config.go              Config: which rules are on
  scope.go               the code inside one transaction, its uses of a handle, and WithTx
  outer_handle.go        outer-handle, for callbacks and for begins
  outer_context.go       outer-context
internal/rule/           the rule names, shared by flags, messages and directives
internal/directive/      //txbypass: comments
internal/opener/         the calls that open a transaction: the tables, and ent's shape
internal/region/         where a begun transaction is open, from the control flow graph
internal/handle/         whether two expressions name the same handle
```

The rules share one pass's state and nothing else, so they sit in one flat package with a file each. The file boundaries are checked by declscope.

| Package | Why it stands alone |
| --- | --- |
| `rule` | Names only. Flags, messages and directives all spell them |
| `directive` | Reads comments and source text only |
| `opener` | Reads types only. It knows nothing of rules |
| `region` | Reads the control flow graph and handles only |
| `handle` | Reads expressions and types only |

`plugin` builds its own copy of the analyzer from the settings, with the rules on that the settings say. Setting the flags of `txbypass.Analyzer` instead would be state shared by every run in the process. It lives in the main module, since `plugin-module-register` adds no dependency beyond `golang.org/x/tools`. A nested module would need a second tag per release, and would make `declscope shrink` stand down on `internal/`.

### declscope

The repository is checked by [declscope](https://github.com/mpyw/declscope) with `qualify: ondemand` and `exported: true` (`.declscope.yaml`). `declscope shrink` runs before the analyzer.

| Rule | Why |
| --- | --- |
| No `//declscope:core` | A core file hides its names from the naming rule |
| A rule's entry point is `//declscope:shared`, with the file that calls it | `run.go` calls into each rule's file |
| `scope.go` holds what both rules read a scope with | Each name carries `scope`: `txScope`, `reportScopeUses`, `scopeRuns` |

## Comparing handles

A handle is compared by name: a base `types.Object`, then the field `*types.Var`s selected from it. This is what Coder's `intxcheck` does too. It is the line between a report that is always a use of the outer handle and one that might be.

| Decision | Why |
| --- | --- |
| Compare objects, not source text | A parameter that shadows the handle has the same text and another object |
| Keep the embedded fields a promotion passes | `s.Create` and `s.DB.Create` are one handle when `s` embeds `*gorm.DB` |
| A method that returns its receiver's type gives the handle back | gorm chains every call: `db.WithContext(ctx).Debug().Create(u)` |
| Report an expression once, then skip what is inside it | `r.db.Create(u)` names the handle as the call, the selector and `r.db`. One report is enough |
| Deduplicate by position and rule | Two openers on one handle, one inside the other, read the inner callback twice |
| Leave the embedded fields at the end out of a comparison | `db.Begin()` on a `*sqlx.DB` is database/sql's, on the embedded `db.DB`. `db.Get` is sqlx's own, on `db`. Both are one handle |

## Regions

A begin's region is a must-analysis over `go/cfg`. A block's entry is open when every predecessor leaves it open. The blocks start open and only fall, which gives the greatest fixed point: what is open on every path.

| Decision | Why |
| --- | --- |
| On every path, not on some | A transaction that may have been committed gives false reports. A miss is better than a false report |
| Only the error check right after the begin, or with it as the init, closes its branch | `err` is reused. A later `if err != nil` checks a statement's error, while the transaction is still open |
| A deferred statement is outside | It runs at the return, after the commit that usually precedes it |
| A commit inside a function literal does not close | The literal runs where it is called, which is not followed |
| A node no path reaches is not read | It runs nowhere |

## Scopes that build or rebind

| Decision | Why |
| --- | --- |
| Firestore's client counts only when a call in its chain takes a context | `tx.Get(client.Doc(p))` is how a Firestore transaction reads. Every builder takes no context, and every call that runs takes one |
| `WithTx` rebinds only when the scope calls it with its own transaction | Then the code says that `q` belongs to the transaction. Without the call, `q` may be built on another database |
| The `WithTx` call, and a chain on it, is not a use | `q.WithTx(tx)` returns the type of `q`, so it names `q` as a chain would |

## Rejected designs

| Design | Why not |
| --- | --- |
| Follow values with SSA, so that `d := db` is the same handle | A copy before the transaction is rare. A value that flows through phi nodes, fields and calls makes "the same handle" a may-alias question. Names give no false reports |
| Report every value of the handle's type inside the callback that is not the transaction | A second database, such as a log store, is a handle of the same type. It would be reported on every use |
| Follow the callback when it is a named function or a method value | The outer handle is named in the caller. Inside the function, the same field is selected from another variable, the receiver |
| End a region at a helper that takes the transaction | Passing the transaction to repository helpers is how it is used. Ending there would end most regions early. A helper that commits is reported after instead, as a gap |
| Read the bodies of same-package helpers to see whether they commit | It answers for one package, and not for the common case of a helper elsewhere. The rule stays simple, and the gap is documented |
| Report every use of Firestore's client inside the transaction | `tx.Get(client.Doc(p))` is the canonical read, and every transaction has one |
| Treat any value with a `WithTx` method as the outer handle | A scope that never calls it may hold one built on another database |
| Make `outer-context` read bun, bob, Spanner and Firestore callbacks too | Their transaction is a value. The outer context there only carries deadlines and values |

## The command

| Decision | Why |
| --- | --- |
| `-V=full` is registered before the driver | x/tools registers a `-V` that prints `devel` for every binary, but only when no `-V` is registered yet. `go vet -vettool` reads the line to identify the tool |
| A release stamps its version with `-X main.version` | goreleaser builds every platform into `dist/` in one checkout. Each build after the first sees untracked files, so the module version the go command records reads `+dirty` |
| `versionFlag` takes its output, its exit and the executable as fields | The tests reach every branch in one process. The coverage floor leaves no room for code that only a subprocess runs |
| `init` intercepts `txbypass skill` and registers `-V` | The test binary runs `init` too, so both are covered. `main` only hands over to the driver |
| No exported error for a pass without the inspector | A driver runs `Requires` first, so only a hand-built `analysis.Pass` meets it. `internal.errRunWithoutInspector` stays as a guard |

## Testing

```bash
go test ./...            # analysistest over testdata/src, and unit tests
mise x -- ./test_all.sh  # tests, golangci-lint, declscope shrink, declscope, coverage
```

- `testdata/src/*` are analysistest packages. Each library `internal/opener` names has a stub there, with the signatures of the version its table names. `enttx/ent` is shaped like ent's generated client, and `enttx/value`, `enttx/noback` and `enttx/twoback` are clients that are not.
- `analyzer_test.go` lists every fixture package. **A package left out of the list is not run**, and its `want` comments are never checked. It runs the default flags, `-outer-handle=false` and `-outer-context=false`. It runs a package again after restoring a flag, to prove that flags are read on each run. No test may call `t.Parallel`, since the flags are global.
- A directive without a reason cannot be pinned in a fixture: the expectation comment on its line would be read as the reason. `internal/directive` tests it.
- **Coverage is held at 99.5% of statements** (`coverage.sh`). The only statements left are in `main`, which hands over to the driver. Code no input reaches is deleted, not excluded.

### Formal specs

There are none yet. The region analysis reasons about paths, so it is the first candidate for one, as molint's specs model its path rules.
