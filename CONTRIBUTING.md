# Contributing

Thanks for looking at this. The library is small, has no dependencies, and is
tested from three languages (Go, JavaScript and Python through the C library),
so most changes are quick to make and quick to verify. This page is the checklist.

## Ground rules

- **Zero dependencies.** The Go library uses the standard library only. A
  change that adds a `require` to `go.mod` will not be merged.
- **The grammar is shared.** The JavaScript client runs the same engine
  through WASM and every other language through the C library in
  `clients/ffi`. The same corpus in `conformance/corpus.json` runs against all
  of them. A grammar change is a change to that corpus too.
- **The grammar is pinned.** `internal/bridgejson/testdata/golden.jsonl`
  records what several hundred queries parse to. If your change alters a line
  of it, a query somebody already uses now means something else: regenerate
  with `go test ./internal/bridgejson -run TestGolden -update`, read the diff,
  and say in the pull request why each changed line is right.
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
make wasm       # the WASM build; then `npm test` in clients/npm
make -C clients/ffi test   # the C library, driven from Python
```

CI runs the same steps, then the tests again on macOS, Windows, the newest Go
and a 32-bit build, and fails if coverage of the library drops below 90%. A
fuzz failure writes its input under `testdata/fuzz`; add it to the seed list in
the fuzz test, with the fix, rather than committing the generated file.

## What a change needs

| Change | Also update |
|---|---|
| Grammar or lexer behaviour | a case in `conformance/corpus.json`, the syntax table in `README.md`, and a seed in `parser/fuzz_test.go` if the shape was ever wrong |
| A built-in function | its `Arity`, a row in the README's function table, and a nil-argument case in `eval/coerce_test.go` |
| Evaluation semantics | a case in `eval/review_regress_test.go` or `conformance/corpus.json`, and one in `conformance/sql/testdata/cases.json` when SQL should agree |
| The JSON AST shape | `internal/bridgejson`, the TypeScript types in `clients/npm/src`, and the golden file |
| A bridge call | `internal/bridgejson/api.go` only; `wasm/bridge.go` and `clients/ffi/cshared.go` stay thin wrappers |
| A new query form worth keeping | a line in `internal/bridgejson/testdata/queries.txt`, then `-update` |
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
