# Contributing

Thanks for looking at this. The library is small, has no dependencies, and is
tested from three languages, so most changes are quick to make and quick to
verify. This page is the checklist.

## Ground rules

- **Zero dependencies.** The Go library uses the standard library only. A
  change that adds a `require` to `go.mod` will not be merged.
- **The grammar is shared.** The JavaScript and Dart clients run the same
  engine through WASM and FFI, and the same corpus in `conformance/corpus.json`
  runs against all three. A grammar change is a change to that corpus too.
- **Consumers generate code.** The library parses, validates and evaluates. SQL,
  JSON or React output is the consumer's visitor; `examples/` shows how.
- **Conventions** for imports, tests and commits are in `CLAUDE.md` and apply
  to humans too: table-driven tests with the standard `testing` package, and
  commits in the form `type(scope): description`.

## Before you open a pull request

```bash
make check      # gofmt, vet, lint, tests with -race
make examples   # every example program runs as documented
make sqltest    # every corpus case through the evaluator and a real SQLite
make fuzz       # 30s per fuzz target (set FUZZTIME=5m for a longer run)
make wasm       # the WASM build and its JavaScript tests
```

CI runs the same steps plus the Dart client and an Android build.

## What a change needs

| Change | Also update |
|---|---|
| Grammar or lexer behaviour | a case in `conformance/corpus.json`, the syntax table in `README.md`, and a seed in `parser/fuzz_test.go` if the shape was ever wrong |
| A built-in function | its `Arity`, a row in the README's function table, and a nil-argument case in `eval/coerce_test.go` |
| Evaluation semantics | a case in `eval/review_regress_test.go` or `conformance/corpus.json`, and one in `conformance/sql/testdata/cases.json` when SQL should agree |
| The JSON AST shape | `internal/bridgejson`, the TypeScript types in `clients/npm/src`, and the Dart types in `clients/flutter/lib` |
| Anything user-visible | a line under *Unreleased* in `CHANGELOG.md` |

## The SQL differential corpus

`conformance/sql/` is a separate Go module (so the library keeps zero dependencies)
holding a SQLite reference lowering and a corpus of real-world, odd and edge
cases: `testdata/records.json` is a small invoices table with NULLs, empty
strings, non-ASCII names, LIKE metacharacters, negative and huge numbers and
boundary timestamps; `testdata/cases.json` lists queries with the ids they
must return. Each case runs through the evaluator and through SQLite, and
both must match the hand-written expectation, so a case documents the
semantics twice over. A case the SQL side cannot honour sets `skipSQL` with
the reason. `TestCorpus_IsWellFormed` validates the data itself: unique ids,
expected ids that exist, timestamps in the canonical form. When adding a
case, write the expectation by hand first; if the two engines agree with
each other but not with you, re-read the data before changing the engines.

## Reporting bugs

The most useful report is a query, the record it ran against, the field config,
what you got and what you expected. The `query explain` command prints the
tokens and tree:

```bash
go run ./cmd/query explain 'state=draft AND total>10-3'
```

If a query makes the library panic, that is always a bug. Fuzzing has not found
one in the parser or evaluator; please include the input.

## Adding an example

Examples live in `examples/<name>/main.go` and must run as the comment at the
top of the file says. `examples/examples_test.go` runs every directory, so add
your example's arguments to its `cases` map or the test will tell you to.

## Releasing

Releases are tagged from `main` with a `v0.x.y` tag after moving the
*Unreleased* section of `CHANGELOG.md` under the new version. Consumers should
pin a tag, not a commit.
