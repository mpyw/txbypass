# Implementation notes

This file records design decisions and implementation details. The short, always-loaded instructions are in [AGENTS.md](../AGENTS.md). What txbypass reports is specified in [rules.md](rules.md).

## Project overview

**txbypass** is a Go linter that reports a database handle used inside a transaction it opened. It is built on [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis), the [`ctrlflow`](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/ctrlflow) pass, and an SSA form it builds with debug information.

The rules find uses in the syntax of a transaction's scope. They ask the SSA form what each use holds, and where a begun transaction is open. A fact says which parameters a function ends, so a helper of another package is seen.

## Architecture

```text
analyzer.go              Analyzer, one flag per rule and per kind of declaration, and the fact types
plugin/                  the golangci-lint module plugin: the settings in place of the flags
skills/                  the agent skills, installed by `txbypass skill install`, and Set, which embeds them
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
internal/funcname/       the names -invokes, -begins and -closes take
internal/txcall/         the calls that open a transaction: the tables (known.go), invokers, begins, and ent's shape
internal/program/        the SSA form with debug information, and the ways from syntax to it
internal/handle/         whether two SSA values are the same handle
internal/fact/           the facts a package exports: closes, invokes and begins
internal/declared/       what a directive or a flag declares, the shape it reads from a signature, and which types are transactions
internal/txcloser/       which instructions may end a transaction
internal/txhelper/       the helpers of the project that are invokers or begins: helper.go, invoker.go, begin.go
internal/region/         where a begun transaction is open, on every path
internal/spell/          how a message spells a use, from the syntax
```

The rules share one pass's state and nothing else, so they sit in one flat package with a file each. The file boundaries are checked by declscope.

| Package | Why it stands alone |
| --- | --- |
| `rule` | Names only. Flags, messages and directives all spell them |
| `directive` | Reads comments and source text only, and the types a declaring directive names |
| `funcname` | Reads and spells names of functions only |
| `txcall` | Reads syntax and types only. It knows nothing of rules |
| `program` | Builds the SSA form, and maps syntax to it. It knows nothing of handles |
| `fact` | Data only. The packages that export a fact and those that read it all import it, so it cannot live in one of them |
| `declared` | Reads declarations, flags, facts and types. The packages below ask it, so it knows none of them |
| `handle` | Reads SSA values only. It asks an interface which functions begin and which types are transactions |
| `txcloser` | Reads SSA instructions and facts. It knows nothing of rules |
| `txhelper` | Reads syntax, SSA and facts, and asks `region` where a begin is open. It knows nothing of rules |
| `region` | Reads the SSA control flow, and asks `txcloser` what ends a transaction |
| `spell` | Reads syntax and types only |

`plugin` builds its own copy of the analyzer from the settings, with the rules on that the settings say. Setting the flags of `txbypass.Analyzer` instead would be state shared by every run in the process. The copy carries `Requires` and `FactTypes` too. It lives in the main module, since `plugin-module-register` adds no dependency beyond `golang.org/x/tools`. A nested module would need a second tag per release, and would make `declscope shrink` stand down on `internal/`.

### declscope

The repository is checked by [declscope](https://github.com/mpyw/declscope) with `qualify: ondemand` and `exported: true` (`.declscope.yaml`). `declscope shrink` runs before the analyzer.

| Rule | Why |
| --- | --- |
| No `//declscope:core` | A core file hides its names from the naming rule |
| A rule's entry point is `//declscope:shared`, with the file that calls it | `run.go` calls into each rule's file |
| `scope.go` holds what both rules read a scope with | Each name carries `scope`: `txScope`, `reportScopeUses`, `scopeRuns` |

## The SSA form

`buildssa` builds no debug information, so `program` builds its own program with `ssa.GlobalDebug`. Then each expression has a `DebugRef`: the value it evaluates to, and the instruction where it does.

| Decision | Why |
| --- | --- |
| Find uses in the syntax, and ask the SSA form what they hold | Reports point at the expression the reader wrote, and messages spell it. The SSA form alone has no positions for most operands |
| Give the program `ctrlflow`'s `NoReturn` | A call of `log.Fatal` or `os.Exit` then ends its block, so a path through it takes no part in a region |
| Build the SSA form only for a package that calls an invoker or a begin, or takes a value with an ending method | The analyzer runs on every dependency for its facts. Most have nothing to read, and building the SSA form of each would make a run slow |
| Find a begin's call by the position of its parenthesis | A call whose results are assigned to several variables gets no `DebugRef` |
| Read a variable on the left of an assignment as written, not used | Its `DebugRef` is the value assigned. `create := db.Create` would report `create` |
| Read the body of a range over a function in the function the builder makes of it | The builder moves the body into a yield function, which the range statement names. Its uses, invokers and begins live there, not in the enclosing function |

## Comparing handles

`handle.Of` follows a value back to its root, through the fields it selects. It is exact: two values are one handle only when they come from one root. It does not ask whether two pointers may alias.

| Decision | Why |
| --- | --- |
| A variable in memory written by one store is the value stored | A variable that a literal captures is not lifted. The callback's `db` is a load of it, so the parameter has to be found through the store |
| Count the stores inside the literals that capture the variable too | A literal can assign it, through its free variable |
| A phi is a handle only when every edge gives it | Two handles meeting is neither. A loop edge back to the phi adds nothing |
| A phi already read gives its handle again, and only a phi still on the way is a loop | Two optional filters, as `q = q.Where(...)` under two conditions, meet one phi through two paths. Taking the second arrival for a loop made `q` a root of its own |
| An edge back to any phi still on the way adds nothing, and an answer that leans on one is kept only while it stays | In nested loops, the inner phi's edge leads to the outer phi, which is still being read. An answer that leans on it holds only while it is on the way, as Tarjan's lowlink tells. Without keeping it at all, a chain of branches in a loop took exponential time |
| One `handle.Paths` per pass remembers every answer | The SSA form does not change during a pass. A handle in memory is read through every use of it, and a rule asks about every use in a region. Without the memory, a region of 3000 statements took 35 s |
| An address given to a function of the module gives no handle | A repository's `Bind(tx)` writes the field the transaction was opened on. Reading that it does would mean reading the callee, and a miss is better than a false report |
| A variable already on the way is itself | `q = q.Where(c)` stores a call on `q`, which gives `q` back. Pointers copied back and forth lead back to one variable too. Without the guard, the analysis recursed until the stack overflowed |
| A free variable of a synthetic function is a root | The wrapper of a bound method value has no parent to find its closure in |
| `WithTx` does not give its receiver back | `qtx := q.WithTx(tx)` is another handle, bound to the transaction |
| Leave the embedded fields at the end out of a comparison | `db.Begin()` on a `*sqlx.DB` is database/sql's, on the embedded `db.DB`. `db.Get` is sqlx's own, on `db`. Both are one handle |
| Report an expression once, then skip what is inside it | `r.db.Create(u)` gives the handle as the call, the selector and `r.db`. One report is enough |
| Deduplicate by position and rule | Two invokers on one handle, one inside the other, read the inner callback twice |

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
| A use in the body of a range over a function is read in the body | The body runs in the loop. An end earlier in the same iteration closes it there. The body's entry is open on the first iteration, and closed when an iteration may end the transaction and go on |
| A transaction stored, sent, or given away inside a literal ends the region | Code this analysis does not read may end it. A miss is better than a false report |
| A transaction given to a function value or an interface method does not end it | Repository interfaces take the transaction all the time. Ending the region there would end most regions early |

## Helpers

`txcloser` reads a function of the package for whether it ends a parameter, and remembers the answer. It exports a fact for each function that does, so that the analysis of an importer reads it.

| Decision | Why |
| --- | --- |
| Only a parameter whose type has an ending method gets a fact | A fact for every parameter that is stored somewhere marked half of the standard library |
| A store or a send inside a helper does not count as an end | `(*sql.Tx).ExecContext` stores its receiver in closures. With escapes counted, every statement would seem to end the transaction |
| A literal that ends the transaction and is given away counts inside a helper too | `retry(func() error { return tx.Commit() })` commits as surely in a helper as in the begin's function. A literal is code of the helper, not a library's store |
| The body of a range over a function ends the transaction only on a path that goes on | The body is a literal given to the iterator. A path that returns from the function holding the loop leaves the transaction open after it, as in a loop over a slice. The body stores its exit in a jump variable, and the function branches on it: to a return, or past the loop |
| A return in such a body is an exit whose block only loads the results and returns | A labeled break or a goto out of the body jumps to its target, which runs code of its own. The optimizer may merge a target with one way in into the block the builder made for returns, so the block's name says nothing |
| In a helper, every end in such a body counts | A path that rolls back and returns has ended the parameter before the helper returns |
| The answers are a least fixpoint over every function and root asked about | Helpers may call each other. Memoizing a provisional false made the answer depend on which was read first. Not memoizing it re-read every path through a ring of helpers, which took factorial time. A fixpoint reads each key once a pass, and the passes are at most the keys. A completed fixpoint is final, so a later question reads only the keys it adds |
| The functions are read in the order of their positions | Whatever order dependence is left gives the same answer on each run |
| `txcall.HoldsTx` looks up each ending method by name | A handle such as gorm's `*DB` has well over a hundred methods. Building the method set for each argument took most of a run |
| A `Rollback` that takes a function is not an end | pop's `Rollback(fn)` opens a transaction of its own, and rolls that back |
| A generic instance is read through its origin | Instances are not built without `InstantiateGenerics` |
| A literal of a type parameter is read in its core type | `T{id}` with `T ~struct{ ID int }` selects a field of the struct every type of `T` has, as the SSA form builds it. Its underlying type is the constraint, an interface |
| The core type is read from the whole type set | A constraint may embed an interface that holds the terms, as `Entity` embeds `Keyed`. An interface allows what all its embedded types allow, and a union what any term allows |

## Helpers that are invokers or begins

`txhelper` reads each declared function for a parameter of a function type that it runs inside a transaction. It also reads it for a first result that is a transaction it begins. It answers `InvokerOf` for every call, a library's invoker first, so the rules ask it alone.

| Decision | Why |
| --- | --- |
| Read the body, with no directive | A project's `withTx` is written once and called everywhere. Reading it costs one pass. A directive would have to say which field is the handle |
| The outer handle is a parameter, and the fields selected from it, by name | A fact is gob-encoded, so it cannot hold a `*types.Var`. The caller finds the fields again in the type of its argument |
| Every call of the callback must be inside a transaction it is given | A helper that may run the callback outside makes every use inside a false report on that path. A miss is better than a false report |
| A callback given anywhere else is no helper | It may run where this analysis does not follow it |
| A helper that calls itself reads as no helper there, and the answer is kept | A ring of helpers is rare. In a ring of two, the one read second may see the first as no helper, so the answer may depend on the order. A fixpoint was not added, since it would only find more helpers in code that is rare |
| Take the first parameter that qualifies | Two callbacks run in one transaction are rare, and the first is the usual one |
| A helper that returns a transaction is a begin of its own kind, with the fact `begins` | The region is read where the result is kept, as for a library's begin. The helper's error result is the begin's error, so `if err != nil` leaves the region as it does after `BeginTx` |
| Every return of a begin helper gives `nil` or a transaction still open there | A helper that commits before it returns, or hands back one it was given, would open a region on a closed or foreign transaction |
| A begin helper's result may be a load of a named result, followed back through its stores | A named result that a deferred literal reads lives in memory. Its return loads each result, stores it back, runs the deferred calls, and loads it again. The region's error check follows the same loads, and so does the check for a returned `nil` |
| A deferred call given the transaction makes a begin helper no helper | The region ends nothing where a deferred call is written. A rollback deferred in the helper runs before the caller reads the transaction |
| An error whose address is handed to a call or stored makes a begin helper no helper | The walk back to the last store sees only stores of the function. A call given `&err` may set it after the begin succeeded |
| The return in the block that a recovered panic reaches takes no part | It returns whatever the named results hold. A path through a panic takes no part in a region either |
| A literal's parameter is a helper's transaction only when the literal is given as the helper's callback | A helper may take two literals. The other one may be given the handle itself, so a transaction opened on it is no savepoint |
| A savepoint through a helper is known as one through a library | The shape carries `Nests` of the invoker or begin inside. A transaction a helper gives, or a helper's begin, counts as a transaction for the savepoint check. One connection given as the handle makes the helper's call not read, as for a library |
| `Begin` names its handle as `Invoker` does: an expression, whether it is the receiver, and fields | A helper may take its handle as an argument. The library begins keep the selector of their call |

## Declared helpers

| Decision | Why |
| --- | --- |
| Three directives, `invokes`, `begins` and `closes`, with no arguments | A transaction is opened by invoking a callback or by beginning, and ended by a call. The verbs say which. Open and begin read the same, so invoke names the callback kind |
| Read the shape from the signature | An interface method has no body. The defaults fit a transaction manager: the receiver is the handle, and the callback takes the transaction, or a context that carries it |
| A body of the package that opens on one handle names it | A declared method of a struct that holds two handles would report every use of the struct otherwise |
| One `declared.Set` answers what is declared and which types are transactions | `txcloser`, `txhelper` and `handle` all ask both. The checker held the answers once, and handed each of them over as a function value, so a constructor took three of them |
| Flag names are matched where they are called, by `funcname.Key`, as errlogreturn's `-sinks` is | Every pass sees the same flags, so no fact is needed. A directive is seen only where it is written, so it exports a fact |
| A declaration wins over the body | A declaration is for code the reader knows better than the body says |
| A declared end ends any input, the receiver of an interface method included | An interface method's receiver is the transaction, as `tx.Done(err)` is. A name that is not `Commit` or `Rollback` is otherwise not an end |

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
| Report the outer transaction inside a savepoint | The savepoint runs on the transaction's connection, so the statement runs inside it. `ROLLBACK TO SAVEPOINT` undoes it with the rest. The tests of pgx and bun use the outer transaction there on purpose |
| Read a begin on one connection, as `*sql.Conn` or `*pgx.Conn` | A statement on the connection runs inside the transaction. The tests of pgx do so 29 times. A type created in the transaction is loaded through the connection, since the transaction has no method for it |
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

- `testdata/src/*` are analysistest packages. Each library `internal/txcall` names has a stub there, with the signatures of the version its table names. `enttx/ent` is shaped like ent's generated client, and `enttx/value`, `enttx/noback` and `enttx/twoback` are clients that are not. `helperlib` holds helpers whose facts another fixture reads.
- `testdata/mod` is a module, run by `TestModule` in module mode. It holds what depends on the module, as a repository in another package of it. Its `go.mod` replaces `gorm.io/gorm` with the stub, which has a `go.mod` for that.
- `analyzer_test.go` lists every fixture package. **A package left out of the list is not run**, and its `want` comments are never checked. It runs the default flags, `-outer-handle=false` and `-outer-context=false`.
- A fact is checked as a `want` too: `// want finish:"closes\\[0\\]"` on the function that exports it.
- A directive without a reason cannot be pinned in a fixture: the expectation comment on its line would be read as the reason. `internal/directive` tests it.
- **Every statement is covered** (`coverage.sh`), except a block no input can reach. Such a block says why, in a comment above its first statement that starts with `Unreachable:`. The gate lists the uncovered blocks, and fails on any without that comment. Code that is reachable gets a test, or is deleted.

### Formal specs

There are none yet. The region analysis reasons about paths, so it is the first candidate for one, as molint's specs model its path rules.
