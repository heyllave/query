# Changelog

All notable changes to this library are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
semantic versioning once the first tag is cut.

## Unreleased

### Removed

- The Dart and Flutter clients (`clients/flutter`, `clients/flutter_plugin`)
  and their Android and iOS builds. `clients/ffi` stays, as a plain C shared
  library for any language; it is built and tested on its own.

### Fixed

- `state==draft`, `total=>5` and `a<>b` are parse errors. They used to compare
  against the text `=draft` or `>5` without complaint. A value that really
  starts with `=`, `<` or `>` is written in quotes.
- A record value that reaches itself (a map stored inside itself) no longer
  overflows the stack when compared as text; a value too deep or too large to
  print compares as missing.
- A panic inside a function registered with `WithFunctions`, or one registered
  without an implementation, is an error from that call instead of a crash.
- Dotted field paths work in function calls on the value side
  (`x=lower(labels.team)`), and `x=lower([labels.team])` prints in a form that
  parses back.
- Arithmetic in a function argument may open with a bracketed field or a call
  (`abs([a]-[b])`, `min(len(name)+1, 9)`), on either side of a comparison.
- Only ASCII digits make a number; a digit from another script is text.
- Arithmetic that opens with a number and continues with a bracketed field
  (`total>2*[unit-price]`) parses without spaces or a paren group; the number
  and operator used to be read as text.
- A paren group around a lone field or call in a function argument
  (`abs(([a]))`) is that field or call, so it prints in a form that parses
  back to the same text.
- `Match` and `Eval` through the WASM and C bridges report the same coded
  `errors` and `validationErrors` as `parseAndValidate`, and both bridges
  reject a field config with a nameless or repeated field or an unknown
  operator instead of accepting it silently.

- Numeric comparisons are exact: an integer literal no longer truncates a
  decimal record value (`price>3` is true for 3.5, `price=3` is false for 3.9).
- Every Go integer and float kind is read (uint, int16, named types). A record
  value that cannot be read as the literal's type never compares as zero or as
  the zero time; `n=0`, `n!=0` and `n<1` are all false for `"abc"`.
- A missing or nil field inside a function resolves to no value instead of the
  text `<nil>`, so `len(name)>3` no longer matches records without a name
  (#22). Errors from nested calls propagate.
- Negated operators (`len(name)!>5`) and ranges (`len(name):1..5`) work on
  function-wrapped qualifiers.
- `WithAllowedOps` and `WithAllowedFields` are enforced inside the validator,
  so they also bind a custom validator, function arguments and bracketed field
  references.
- A field's own operators bound every way a query can read it: through a
  function (ordering a function's result over a text field needs the wildcard
  operator), from the value side of a comparison (`name!=[secret]`,
  `n>len(secret)`), and through a predicate function (`startsWith(secret, "a")`
  needs the wildcard operator, also under `WithAllowedOps`). A field that
  allows only `=` can no longer be read prefix by prefix.
- `daysAgo`, `addDays` and `addBusinessDays` accept at most
  ±`eval.MaxCalendarDays` days. `addBusinessDays` skips whole weeks outside the
  holiday calendar, so the largest count costs the same as the smallest (it
  used to walk a third of a second per record at the cap).
- Dotted fields descend nested maps (`labels.dev` reads `data["labels"]["dev"]`),
  including keys that contain dots (`labels.example.com/team` reads
  `data["labels"]["example.com/team"]`) and any map with string keys.
- `x NOT IN (a, b)` no longer parses as `x AND NOT=a ...`; it is `x!=a AND x!=b`.
  `IN` and `NOT IN` accept a function on the left (`lower(name) IN (a, b)`).
- A term followed by a group (`urgent (a OR b)`) is an implicit AND, not a call
  to `urgent`; a call is a name directly followed by `(`.
- Unicode spaces separate terms; a no-break space no longer glues two words
  into one field name.
- `ip=10.0.0.1/24` is one string instead of the string `10.0.0.1` divided by 24.
- A duration literal too large for `time.Duration` is a parse error instead of
  a wrapped value; `abs` of the smallest integer no longer returns it negative.
- The parser refuses more than `parser.MaxNesting` levels of nesting
  (`nestingTooDeep`), so a query cannot exhaust the stack when the length limit
  is disabled.
- `ast.String` prints a quoted field argument (`f(["cpu limit"])`) and a
  grouped string argument (`f((draft))`) in a form that parses back.
- A failed term is reported once rather than twice.
- Presence (`name`) is false for nil, `""` and empty lists or maps.
- `name!=jo*` negates the wildcard; `ast.SQLOperator` renders `NOT LIKE` (#21).
- A date literal without a time denotes the whole UTC day, so
  `createdAt=2026-09-28` matches everything created that day instead of only
  midnight; `!=`, `<=`, `>` and range ends follow the day's bounds, and
  `examples/sql` lowers them to half-open intervals (#24).
- Non-ASCII bare words (`café`, `日本`) lex as one term in field and value
  position; an unexpected character is reported as the whole rune (#23).
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
- The WASM and C bridges turn a Go panic into an error envelope instead of
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

- A documentation site with a live playground, built from `docs/` and
  published to GitHub Pages (<https://heyllave.github.io/query/>).
- `validate.WithAllowedFunctions`, for consumers that translate the tree and
  must refuse functions they cannot translate (`functionNotAllowed`).
- `validate.DefaultOps`, and field configs in JSON may name their type
  (`"type": "decimal"`) and omit `allowedOps` to get the defaults for it.
- A golden file of several hundred queries that pins the grammar, the printed
  form, the JSON tree and the error codes; a resource-listing scenario set in
  the shared corpus; a Python runner that drives the C library through the
  corpus; CI on macOS, Windows, the newest Go and a 32-bit build, with a
  coverage floor and a weekly long fuzz run. See *Stability* in the README.
- Datetime literals (`created_at>=2026-01-01T10:00:00Z`); quoted timestamps on
  date fields are read as dates.
- Signed literals and wildcards in function arguments and `IN` lists; arithmetic
  and bracketed field names in arguments (`addDays(d, 1-8)`, `abs([cpu-limit])`);
  signed operands (`n>10*-3`).
- `NOT IN`, and `IN` on a function.
- Reserved words as field names where unambiguous (`true=1`, `not=1`); quoted
  and bracket-quoted field names (`"foo bar"=1`, `["a.b/c"]=1`).
- `\*` as a literal star inside wildcard patterns.
- `ast.Value.DayOnly`, `parser.IsDayLiteral`, `ast.Calls`, `eval.MapAccessor`, `eval.Exactly`/`AtLeast`/`Between`,
  `validate.WithAllowedFields`/`WithAllowedOps`, `parser.ParseDateLiteral`.
- Fuzz targets for the parser, value grammar and evaluator (`make fuzz`), run
  briefly in CI; a test that runs every example program (`make examples`).
- `conformance/sql/`, a separate module with a SQLite reference lowering and a corpus
  of real, odd and edge cases that the evaluator and a real database must
  both satisfy (`make sqltest`, run in CI).
- `len()` counts characters, not bytes.
- A wildcard value's `Str` is in pattern form (`\*` literal star, `\\`
  backslash); `ast.DecodePattern` reads it and `WildcardToLike` honours it.
- A reserved word cannot name a function (`in(x)` is an error); a dotted
  path may contain one as a later segment (`labels.in`).
- `CONTRIBUTING.md` and this changelog.
