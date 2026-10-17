# Implementation notes

This file records design decisions and implementation details. The short, always-loaded instructions are in [AGENTS.md](../AGENTS.md). What txbypass reports is specified in [rules.md](rules.md).

## Project overview

**txbypass** is a Go linter that reports a database handle used inside a transaction it opened. It is built on [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis) and the [`inspect`](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/inspect) pass.

It reads names and types inside the callback of a known opener. It has no analysis facts, so it does not analyze dependencies.

## Architecture

```text
analyzer.go              Analyzer, one flag per rule
skills.go                Skills: the embedded skills/ directory
plugin/                  the golangci-lint module plugin: the settings in place of the flags
skills/                  the agent skills, installed by `txbypass skill install`
cmd/txbypass/            singlechecker entry point, the skill subcommand, and -V=full
internal/                the rules: one flat package, one namespace per file
  run.go                 Run: every call in the package, then the reports in order
  checker.go             the per-pass state, and reporting through the directives
  config.go              Config: which rules are on
  outer_handle.go        outer-handle
internal/rule/           the rule names, shared by flags, messages and directives
internal/directive/      //txbypass: comments
internal/opener/         the calls that open a transaction, and their callbacks
internal/handle/         whether two expressions name the same handle
```

The rules share one pass's state and nothing else, so they sit in one flat package with a file each. The file boundaries are checked by declscope.

| Package | Why it stands alone |
| --- | --- |
| `rule` | Names only. Flags, messages and directives all spell them |
| `directive` | Reads comments and source text only |
| `opener` | Reads types only. It knows nothing of rules |
| `handle` | Reads expressions and types only |

`plugin` builds its own copy of the analyzer from the settings, with the rules on that the settings say. Setting the flags of `txbypass.Analyzer` instead would be state shared by every run in the process. It lives in the main module, since `plugin-module-register` adds no dependency beyond `golang.org/x/tools`. A nested module would need a second tag per release, and would make `declscope shrink` stand down on `internal/`.

### declscope

The repository is checked by [declscope](https://github.com/mpyw/declscope) with `qualify: ondemand` and `exported: true` (`.declscope.yaml`). `declscope shrink` runs before the analyzer.

| Rule | Why |
| --- | --- |
| No `//declscope:core` | A core file hides its names from the naming rule |
| A rule's entry point is `//declscope:shared`, with the file that calls it | `run.go` calls into each rule's file |

## Comparing handles

A handle is compared by name: a base `types.Object`, then the field `*types.Var`s selected from it. This is what Coder's `intxcheck` does too. It is the line between a report that is always a use of the outer handle and one that might be.

| Decision | Why |
| --- | --- |
| Compare objects, not source text | A parameter that shadows the handle has the same text and another object |
| Keep the embedded fields a promotion passes | `s.Create` and `s.DB.Create` are one handle when `s` embeds `*gorm.DB` |
| A method that returns its receiver's type gives the handle back | gorm chains every call: `db.WithContext(ctx).Debug().Create(u)` |
| Report an expression once, then skip what is inside it | `r.db.Create(u)` names the handle as the call, the selector and `r.db`. One report is enough |
| Deduplicate by position and rule | Two openers on one handle, one inside the other, read the inner callback twice |

## Rejected designs

| Design | Why not |
| --- | --- |
| Follow values with SSA, so that `d := db` is the same handle | A copy before the transaction is rare. A value that flows through phi nodes, fields and calls makes "the same handle" a may-alias question. Names give no false reports |
| Report every value of the handle's type inside the callback that is not the transaction | A second database, such as a log store, is a handle of the same type. It would be reported on every use |
| Follow the callback when it is a named function or a method value | The outer handle is named in the caller. Inside the function, the same field is selected from another variable, the receiver |
| Read `Begin` ... `Commit` regions | It needs the control flow between them, with early returns and deferred rollbacks. It is the next step, not the first |

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

- `testdata/src/*` are analysistest packages. `gorm.io/gorm`, `github.com/uptrace/bun` and `github.com/jackc/pgx/v5` there are stubs, with the signatures of the versions `internal/opener` names.
- `analyzer_test.go` runs the default flags and `-outer-handle=false`. It runs a package again after restoring a flag, to prove that flags are read on each run. No test may call `t.Parallel`, since the flags are global.
- A directive without a reason cannot be pinned in a fixture: the expectation comment on its line would be read as the reason. `internal/directive` tests it.
- **Coverage is held at 99.5% of statements** (`coverage.sh`). The only statements left are in `main`, which hands over to the driver. Code no input reaches is deleted, not excluded.

### Formal specs

There are none. The rules compare names inside one function literal, and follow no paths. molint's specs model rules that reason about paths, which this one does not.
