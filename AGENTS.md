# Repository instructions

txbypass is a Go analyzer that reports a database handle used inside a transaction it opened. Every rule, message, exemption and directive is specified in [design/rules.md](design/rules.md). The architecture and the rejected designs are in [design/implementation.md](design/implementation.md). They include why handles are compared by name, not followed as values. Read the relevant section before changing a rule, a message, a directive, or how handles are compared.

When a rule changes, change `design/rules.md` first, then its fixtures under `testdata/src/`, then the code. An opener or a begin goes into its table in `internal/opener`. Its library gets a stub under `testdata/src/`, and a row in the tables of the README and `design/rules.md`. A new fixture package goes into the list in `analyzer_test.go`: a package left out is never run, and its `want` comments pass unchecked. Keep the README and `skills/txbypass-authoring/SKILL.md` in sync with the rules. The skill is installed by `txbypass skill install`, so a stale one misleads every agent that reads it. A release bumps the version in the README's install instructions and the skill's "Written against" line together. `skills_test.go` fails when they differ.

Run `go test ./...` for Go changes and `mise x -- ./test_all.sh` before claiming the full repository gate passes. It runs golangci-lint, `declscope shrink`, declscope, and the coverage floor.

This repository runs declscope on itself with `qualify: ondemand` and `exported: true`. Read `.agents/skills/declscope-authoring/SKILL.md` before adding, naming, or moving a declaration.
