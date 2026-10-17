# Implementation notes

This file records design decisions and implementation details. The short, always-loaded instructions are in [AGENTS.md](../AGENTS.md). What txbypass reports is specified in [rules.md](rules.md).

## Project overview

**txbypass** is a Go linter that reports a database handle used inside a transaction it opened. It is built on [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis), the [`ctrlflow`](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/ctrlflow) pass, and an SSA form it builds with debug information.

The rules find uses in the syntax of a transaction's scope. They ask the SSA form what each use holds, and where a begun transaction is open. A fact says which parameters a function ends, so a helper of another package is seen.

## Architecture

```text
analyzer.go              Analyzer, one flag per rule, and the fact type
skills.go                Skills: the embedded skills/ directory
plugin/                  the golangci-lint module plugin: the settings in place of the flags
skills/                  the agent skills, installed by `txbypass skill install`
cmd/txbypass/            singlechecker entry point, the skill subcommand, and -V=full
internal/                the rules: one flat package, one namespace per file
  run.go                 Run: build the SSA form when needed, every function and call, then the reports in order
  checker.go             the per-pass state, and reporting through the directives
  config.go              Config: which rules are on
  scope.go               the code inside one transaction, its uses of a handle, and WithTx
  outer_handle.go        outer-handle, for callbacks and for begins
  outer_context.go       outer-context
internal/rule/           the rule names, shared by flags, messages and directives
internal/directive/      //txbypass: comments
internal/opener/         the calls that open a transaction: the tables, and ent's shape
internal/program/        the SSA form with debug information, and the ways from syntax to it
internal/handle/         whether two SSA values are the same handle
internal/closer/         which instructions may end a transaction, and the fact of helpers
internal/region/         where a begun transaction is open, on every path
internal/spell/          how a message spells a use, from the syntax
```

The rules share one pass's state and nothing else, so they sit in one flat package with a file each. The file boundaries are checked by declscope.

| Package | Why it stands alone |
| --- | --- |
| `rule` | Names only. Flags, messages and directives all spell them |
| `directive` | Reads comments and source text only |
| `opener` | Reads syntax and types only. It knows nothing of rules |
| `program` | Builds the SSA form, and maps syntax to it. It knows nothing of handles |
| `handle` | Reads SSA values only |
| `closer` | Reads SSA instructions and facts. It knows nothing of rules |
| `region` | Reads the SSA control flow, and asks `closer` what ends a transaction |
| `spell` | Reads syntax and types only |

`plugin` builds its own copy of the analyzer from the settings, with the rules on that the settings say. Setting the flags of `txbypass.Analyzer` instead would be state shared by every run in the process. The copy carries `Requires` and `FactTypes` too. It lives in the main module, since `plugin-module-register` adds no dependency beyond `golang.org/x/tools`. A nested module would need a second tag per release, and would make `declscope shrink` stand down on `internal/`.

### declscope

The repository is checked by [declscope](https://github.com/mpyw/declscope) with `qualify: ondemand` and `exported: true` (`.declscope.yaml`). `declscope shrink` runs before the analyzer.

| Rule | Why |
| --- | --- |
| No `//declscope:core` | A core file hides its names from the naming rule |
| A rule's entry point is `//declscope:shared`, with the file that calls it | `run.go` calls into each rule's file |
| `scope.go` holds what both rules read a scope with | Each name carries `scope`: `txScope`, `scopeOuter`, `reportScopeUses`, `scopeRuns` |

## The SSA form

`buildssa` builds no debug information, so `program` builds its own program with `ssa.GlobalDebug`. Then each expression has a `DebugRef`: the value it evaluates to, and the instruction where it does.

| Decision | Why |
| --- | --- |
| Find uses in the syntax, and ask the SSA form what they hold | Reports point at the expression the reader wrote, and messages spell it. The SSA form alone has no positions for most operands |
| Give the program `ctrlflow`'s `NoReturn` | A call of `log.Fatal` or `os.Exit` then ends its block, so a path through it takes no part in a region |
| Build the SSA form only for a package that calls an opener or a begin, or takes a value with an ending method | The analyzer runs on every dependency for its facts. Most have nothing to read, and building the SSA form of each would make a run slow |
| Find a begin's call by the position of its parenthesis | A call whose results are assigned to several variables gets no `DebugRef` |
| Read a variable on the left of an assignment as written, not used | Its `DebugRef` is the value assigned. `create := db.Create` would report `create` |

## Comparing handles

`handle.Of` follows a value back to its root, through the fields it selects. It is exact: two values are one handle only when they come from one root. It does not ask whether two pointers may alias.

| Decision | Why |
| --- | --- |
| A variable in memory written by one store is the value stored | A variable that a literal captures is not lifted. The callback's `db` is a load of it, so the parameter has to be found through the store |
| Count the stores inside the literals that capture the variable too | A literal can assign it, through its free variable |
| A phi is a handle only when every edge gives it | Two handles meeting is neither. A loop edge back to the phi adds nothing |
| One `handle.Paths` per pass remembers every answer | The SSA form does not change during a pass. A handle in memory is read through every use of it, and a rule asks about every use in a region. Without the memory, a region of 3000 statements took 35 s |
| An address given to a function of the package gives no handle | A repository's `Bind(tx)` writes the field the transaction was opened on. Reading that it does would mean reading the callee, and a miss is better than a false report |
| A variable already on the way is itself | `q = q.Where(c)` stores a call on `q`, which gives `q` back. Pointers copied back and forth lead back to one variable too. Without the guard, the analysis recursed until the stack overflowed |
| A free variable of a synthetic function is a root | The wrapper of a bound method value has no parent to find its closure in |
| `WithTx` does not give its receiver back | `qtx := q.WithTx(tx)` is another handle, bound to the transaction |
| Leave the embedded fields at the end out of a comparison | `db.Begin()` on a `*sqlx.DB` is database/sql's, on the embedded `db.DB`. `db.Get` is sqlx's own, on `db`. Both are one handle |
| Report an expression once, then skip what is inside it | `r.db.Create(u)` gives the handle as the call, the selector and `r.db`. One report is enough |
| Deduplicate by position and rule | Two openers on one handle, one inside the other, read the inner callback twice |

## Regions

A begin's region is a must-analysis over the SSA blocks of its function. A block's entry state is the join of what its predecessors leave on their edges.

| State | Joins as |
| --- | --- |
| Dead, a point no returning path reaches | The other state |
| Open | Open, with open or dead |
| Closed | Closed, with anything |

The blocks start dead, and the entry starts closed. The states only fall, so the analysis ends at what is open on every path.

| Decision | Why |
| --- | --- |
| On every path, not on some | A transaction that may have ended gives false reports. A miss is better than a false report |
| The begin's error is its own SSA value | `err` is reused. A later `if err != nil` checks another call's error, and the SSA form says so without a rule about where the check is |
| An error in memory counts when every path to its load last stored the begin's error | A named result read by a deferred literal is not lifted. The walk back through the blocks stops at the first store on each path |
| The state before each instruction is computed once | A rule asks about every use in the region. Stepping from the block's entry for each made the time grow with the square of the block |
| A deferred statement is outside | It runs at the return, after the commit that usually precedes it |
| A use in a function literal is placed where the literal is made | That is where a goroutine starts, or where a literal called on the spot runs |
| A transaction stored, sent, or given away inside a literal ends the region | Code this analysis does not read may end it. A miss is better than a false report |
| A transaction given to a function value or an interface method does not end it | Repository interfaces take the transaction all the time. Ending the region there would end most regions early |

## Helpers

`closer` reads a function of the package for whether it ends a parameter, and remembers the answer. It exports a fact for each function that does, so that the analysis of an importer reads it.

| Decision | Why |
| --- | --- |
| Only a parameter whose type has an ending method gets a fact | A fact for every parameter that is stored somewhere marked half of the standard library |
| A store or a send inside a helper does not count as an end | `(*sql.Tx).ExecContext` stores its receiver in closures. With escapes counted, every statement would seem to end the transaction |
| The answers are a least fixpoint over every function and root asked about | Helpers may call each other. Memoizing a provisional false made the answer depend on which was read first. Not memoizing it re-read every path through a ring of helpers, which took factorial time. A fixpoint reads each key once a pass, and the passes are at most the keys. A completed fixpoint is final, so a later question reads only the keys it adds |
| The functions are read in the order of their positions | Whatever order dependence is left gives the same answer on each run |
| `opener.Holds` looks up each ending method by name | A handle such as gorm's `*DB` has well over a hundred methods. Building the method set for each argument took most of a run |
| A `Rollback` that takes a function is not an end | pop's `Rollback(fn)` opens a transaction of its own, and rolls that back |
| A generic instance is read through its origin | Instances are not built without `InstantiateGenerics` |

## Scopes that build or rebind

| Decision | Why |
| --- | --- |
| Firestore's client counts only when a call in its chain takes a context | `tx.Get(client.Doc(p))` is how a Firestore transaction reads. Every builder takes no context, and every call that runs takes one |
| A callback does not read a literal it keeps in a variable declared outside it | The after-commit hook is the pattern: the callback collects work to run once the transaction is durable. The literal runs after the callback returns, and a use of the handle in it is on purpose. The syntax says where the variable is declared, which the SSA form loses for a captured variable |
| `WithTx` rebinds only when the scope calls it with its own transaction | Then the code says that `q` belongs to the transaction. Without the call, `q` may be built on another database |
| The `WithTx` call, and a chain on it, is not a use | `q.WithTx(tx)` returns the type of `q`, so it would otherwise read as `q` |

## Rejected designs

| Design | Why not |
| --- | --- |
| Compare handles by the names in the syntax, and read regions from `go/cfg` | The first version did. A commit through a copy, a pointer, a closure or a helper was not seen, so the code after it was reported falsely. A path through `log.Fatal` came back as a path, so a report was missed. A copy of the handle was missed too. The SSA form gives all of these, and the positions stay the syntax's |
| Use `buildssa`'s result | It has no debug information, so an expression cannot be mapped to its value |
| Report every value of the handle's type inside the scope that is not the transaction | A second database, such as a log store, is a handle of the same type. It would be reported on every use |
| Follow the callback when it is a named function or a method value | The outer handle is in the caller. Inside the function, the same field is selected from another value, the receiver |
| End a region at any call the transaction is given to | Passing the transaction to repository helpers is how it is used. Only a helper that ends it ends the region |
| Report every use of Firestore's client inside the transaction | `tx.Get(client.Doc(p))` is the canonical read, and every transaction has one |
| Treat any value with a `WithTx` method as the outer handle | A scope that never calls it may hold one built on another database |
| Make `outer-context` read bun, bob, Spanner and Firestore callbacks too | Their transaction is a value. The outer context there only carries deadlines and values |

## The command

| Decision | Why |
| --- | --- |
| `-V=full` is registered before the driver | x/tools registers a `-V` that prints `devel` for every binary, but only when no `-V` is registered yet. `go vet -vettool` reads the line to identify the tool |
| A release stamps its version with `-X main.version` | goreleaser builds every platform into `dist/` in one checkout. Each build after the first sees untracked files, so the module version the go command records reads `+dirty` |
| `versionFlag` takes its output, its exit and the executable as fields | The tests reach every branch in one process. The coverage gate leaves no room for code that only a subprocess runs |
| `init` intercepts `txbypass skill` and registers `-V` | The test binary runs `init` too, so both are covered. `main` only hands over to the driver |
| No exported error for a pass without `ctrlflow` | A driver runs `Requires` first, so only a hand-built `analysis.Pass` meets it. `internal.errRunWithoutCtrlflow` stays as a guard |

## Testing

```bash
go test ./...            # analysistest over testdata/src, and unit tests
mise x -- ./test_all.sh  # tests, golangci-lint, declscope shrink, declscope, coverage
```

- `testdata/src/*` are analysistest packages. Each library `internal/opener` names has a stub there, with the signatures of the version its table names. `enttx/ent` is shaped like ent's generated client, and `enttx/value`, `enttx/noback` and `enttx/twoback` are clients that are not. `txhelper` holds helpers whose facts another fixture reads.
- `analyzer_test.go` lists every fixture package. **A package left out of the list is not run**, and its `want` comments are never checked. It runs the default flags, `-outer-handle=false` and `-outer-context=false`.
- A fact is checked as a `want` too: `// want finish:"closes\\[0\\]"` on the function that exports it.
- A directive without a reason cannot be pinned in a fixture: the expectation comment on its line would be read as the reason. `internal/directive` tests it.
- **Every statement is covered** (`coverage.sh`), except a block no input can reach. Such a block says why, in a comment above its first statement that starts with `Unreachable:`. The gate lists the uncovered blocks, and fails on any without that comment. Code that is reachable gets a test, or is deleted.

### Formal specs

There are none yet. The region analysis reasons about paths, so it is the first candidate for one, as molint's specs model its path rules.
