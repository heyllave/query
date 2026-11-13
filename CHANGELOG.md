# Changelog

All notable changes to this library are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
semantic versioning once the first tag is cut.

## Unreleased

### Fixed

- Numeric comparisons are exact: an integer literal no longer truncates a
  decimal record value (`price>3` is true for 3.5, `price=3` is false for 3.9).
- Every Go integer and float kind is read (uint, int16, named types). A record
  value that cannot be read as the literal's type never compares as zero or as
  the zero time; `n=0`, `n!=0` and `n<1` are all false for `"abc"`.
- A missing or nil field inside a function resolves to no value instead of the
  text `<nil>`, so `len(name)>3` no longer matches records without a name.
  Errors from nested calls propagate.
- Negated operators (`len(name)!>5`) and ranges (`len(name):1..5`) work on
  function-wrapped qualifiers.
- `WithAllowedOps` and `WithAllowedFields` are enforced inside the validator,
  so they also bind a custom validator, function arguments and bracketed field
  references. A field's own operators apply through a function for `=`, `!=`,
  wildcard and presence.
- `daysAgo`, `addDays` and `addBusinessDays` accept at most
  ±`eval.MaxCalendarDays` days, so a large literal cannot stall evaluation.
- Dotted fields descend nested maps (`labels.dev` reads `data["labels"]["dev"]`).
- Presence (`name`) is false for nil, `""` and empty lists or maps.
- `name!=jo*` negates the wildcard; `ast.SQLOperator` renders `NOT LIKE`.
- Struct binding dereferences pointer fields and ignores `query` tags on
  unexported fields instead of panicking.
- Integer overflow in arithmetic promotes to float; `round()` with an absurd
  number of places resolves to no value instead of NaN.
- `ast.Walk` visits function calls in field, value and argument position;
  `ast.Fields` no longer reports a function name as a field.
- `ast.String` output re-parses to the same tree: escaped characters are kept,
  values that would not lex back bare are quoted, names outside the identifier
  alphabet print as `["name"]`, and a function-wrapped qualifier prints its
  function.
- The WASM and FFI bridges turn a Go panic into an error envelope instead of
  tearing down the host.
- `examples/sql` validates against a schema and allowlists columns and
  functions; unknown names are errors, never interpolated.

### Changed

- Unquoted values that contain letters are always strings, so UUIDs, versions
  and e-mail addresses are one value. `10-3`, `10 - 3` and `10+3` all compute.
- Literals on a text field compare as text (`zip=90210`). Arithmetic on a text
  or boolean field is a validation error.
- Unknown functions and wrong argument counts are compile errors
  (`eval.ErrUnknownFunction`, `eval.ErrArity`). `eval.Func` gained `Arity`.
- `QualifierExpr.Field` is empty when `FieldFunc` is set.
- `ast.Depth` counts nesting; a chain of one connective (or an `IN` list) is
  one level, so `WithMaxDepth` no longer rejects long flat searches.
- Identifiers may contain `/` (`labels.example.com/team`).

### Added

- Datetime literals (`created_at>=2026-01-01T10:00:00Z`); quoted timestamps on
  date fields are read as dates.
- Signed literals and wildcards in function arguments and `IN` lists.
- Reserved words as field names where unambiguous (`true=1`, `not=1`); quoted
  and bracket-quoted field names (`"foo bar"=1`, `["a.b/c"]=1`).
- `\*` as a literal star inside wildcard patterns.
- `ast.Calls`, `eval.MapAccessor`, `eval.Exactly`/`AtLeast`/`Between`,
  `validate.WithAllowedFields`/`WithAllowedOps`, `parser.ParseDateLiteral`.
- Fuzz targets for the parser, value grammar and evaluator (`make fuzz`), run
  briefly in CI; a test that runs every example program (`make examples`).
- `CONTRIBUTING.md` and this changelog.
