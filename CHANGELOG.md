# Changelog

All notable changes to this library are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
semantic versioning.

## Unreleased

### Fixed

- A value glued to the next term is an error instead of two conditions.
  Under the default grammar `a=now()x` read as `a=now() AND x`, and
  `a=[b]c`, `a=2026-11-16[b]` and `a=(1+2)x` the same way: the lookahead
  judged the whole run one value, and the reader stopped partway and left
  the rest to become a condition. A term after a space inside a group,
  `a=(1 2)`, is refused too. A glued AND or OR still joins two conditions
  (`a=[b]AND c=1`). A test now holds the two to the same span.
- A date with a time can be an operand of arithmetic.
  `createdAt>2026-11-16T10:00:00Z-1h` was text, and refused on a date field
  with a hint that offered the form already typed; it is that instant minus
  an hour now, as `2026-11-16+1d` already was a day later. A zone offset is
  part of the datetime, not a sum: `10:00:00-05:00` is an instant.
- A date that is not written in full is no longer text. Four digits and a
  hyphen used to be kept together on the promise of a date, so `2026-11-5`
  and `2026-1` were text while `10-3` was a subtraction, and `2026-1-1` was
  text minus one. Only a complete `YYYY-MM-DD` is a date now; anything
  shorter is a subtraction, like every other run of digits and hyphens. To
  compare with those characters as text, quote them: `name="2026-11-5"`.
- The validator works out what arithmetic over literals gives and refuses
  it when it cannot fit the field: `createdAt>10*3` (a number where a date
  belongs), `ttl>10-3`, `n>7d-1h`. Arithmetic whose operands have no
  meaning together, such as `10-7d`, is refused as well. Both used to
  validate and then match nothing. Arithmetic that reads a field or calls a
  function is left to the evaluator, as before.
- A date field takes a date written short. `createdAt=2026-11-5` is the
  day 2026-11-05, in a comparison, a range or an `IN` list, and quoted as
  well; validated, it prints as `2026-11-05`. It must be one unbroken run:
  `2026 - 11 - 5` and `(2026)-11-5` are subtractions. `ast.Value.ShortDate`
  marks such a run, set by the parser from the source.
- A number field refuses a date-shaped run: `n=2026-11` would otherwise
  compare with 2015, and the hint names that number in case it was meant.
  Written as a subtraction, `n=2026 - 11`, it is 2015 as before.

- A date written short is refused where no declared type says what it is:
  as a function's argument (`contains(name, 2026-11)`) and compared with a
  function's result (`upper(name)=2026-11-5`, `year(day)=2026-11`). The
  hint gives the date in full, the text in quotes and the subtraction with
  spaces. A run is date-shaped only when its parts could be a month and a
  day, so `n<1500-50` is 1450.
- Arithmetic over literals that has no result is refused instead of
  matching nothing: a divisor written as zero (`n=10/0`, `ttl>7d/0`), text
  as an operand (`ttl>7d-1.5h`, where `1.5h` is not a duration), and a
  fractional modulo. A field of whole numbers refuses a fraction:
  `n=10/4`, like `n=2.5`.
- A bare `2026-11` where a field belongs is reported whole, not as `2026`.

### Breaking

- Under the default grammar, a value glued to the next term (`a=now()x`)
  is refused; it used to be two conditions joined by AND. The strict
  grammar already refused every such query.
- With no schema, `2026-11` and `2026-11-5` evaluate as subtractions:
  `query.Eval("2026-11")` is 2015. They were text.
- `contains(name, 2026-11)` searched for the text `2026-11`; it is refused
  now, and `contains(name, "2026-11")` is how to write it.
- A text field no longer takes `2026-11` or `2026-11-5` unquoted. They are
  arithmetic, like `10-3`, and a text field refuses arithmetic: write
  `period="2026-11"`. Before this release the two were text while `10-3`
  was refused.

### Changed

- The Grammar page states how an unquoted value gets its type as ordered
  steps, each with examples, and a test holds the parser to every one.

- The documentation is in three parts: Language, Clients and Guides. Page
  addresses are unchanged.

### Added

- Completion for a query that is still being typed. `query.Complete(text,
  cursor, fields)` (package `complete`) reports what the grammar expects at
  the cursor and lists what the schema can suggest there: field names, the
  operators a field allows, a list's selectors, `AND` and `OR`, and the
  values a field is known to take. Inside a selector it offers the fields of
  one element. It answers from the schema alone and never fails on an
  unfinished query.
- `Completion.Offer` for candidates that live outside the schema, such as
  values a host fetches or the keys of a nested field. It writes each one
  the way the grammar needs it: a value that would not read back as itself
  is quoted, and a key is joined to its field, in the bracket form when it is
  not a plain identifier. `complete.ValueText` and `complete.FieldText` are
  the two writers.
- `FieldConfig.Values`: values a field is known to take, offered by
  completion. Validation does not hold a query to them.
- `parser.CursorAt`, the layer under completion: the slot at a position, the
  word under the cursor, the field it belongs to and the list selectors
  around it, with no schema.
- The TypeScript client has `complete`, `offer` and `applyCompletion`, with
  the cursor counted as `selectionStart` counts it. The C library has
  `QueryComplete` and `QueryOffer`, counting bytes, or UTF-16 code units
  with `"utf16": true`.
- Completion says when a value is compared with a function's result
  (`Completion.Function`) or is an operand of arithmetic
  (`Completion.Arithmetic`, `Completion.Sign`), and `Offer` writes
  candidates to suit: a number stays a number after `len(name)>`, `812`
  is quoted after `lower(name)=`, and after `createdAt>now()-` only a
  duration is offered.
- Nothing is offered that the field would refuse. A value from `Values` or
  from `Offer` is checked against the field's type the way the validator
  checks it, and one that does not fit is left out. Inside a quote opened
  on a field that is not text, the item replaces the quote.
- Completion cases in the shared corpus, run by every client.
- A "Completing a query as it is typed" guide.
- A "query and jq" guide: the two tools on the same records, with the cases
  where they return different answers.

## v0.4.0 - 2026-11-18

### Added

- List fields in a schema. A field of the new `validate.TypeList` declares
  the fields of one element in `FieldConfig.Elements`, and the validator
  checks a selector against it: the condition inside `orders@(...)` may name
  only an element's fields, with their types and operators, and an element
  field may itself be a list. `FieldConfig.Selectors` names the selectors a
  list allows; the default, `validate.DefaultSelectors`, is `@first`,
  `@(...)`, `@all(...)` and `@none(...)`, so `@last` and `@any(...)` are
  refused on a declared list unless it lists them. The TypeScript client and
  the C library take the same config as `"type": "list"`, `"elements"` and
  `"selectors"`.
- `ast.SelectorFirst`, `SelectorLast`, `SelectorExists`, `SelectorAny`,
  `SelectorAll` and `SelectorNone` name the selectors, `ast.Selectors()`
  lists them, and `ast.SelectorForm` gives the written form (`@all(...)`).
  `FieldConfig.AllowsSelector` and `FieldConfig.SelectorForms` answer for
  one field, for a host that describes its schema to a client.

### Changed

- Once a schema declares a list field, a selector on a field that is not a
  list is a validation error (`"name" is not a list field`). A schema with
  no list field validates as before.

## v0.3.0 - 2026-11-18

### Changed

- Under `ExplicitAnd`, a `NOT` with nothing after it to negate is an error:
  `a=1 AND NOT`, `NOT` alone, `(NOT)` and `NOT OR b=2`. It was read as the
  bare term `not`, so a query cut short searched for that word. A dangling
  `AND` or `OR` was already an error. `not=1`, `not.x=1`, `not@first` and
  `"not"` are unchanged, and so is the default grammar.

### Fixed

- A word that cannot stand alone as a term now gets a hint that shows it in
  quotes: `812`, `2026-11-16`, `v1.2`, `foo@bar.com` and `checkout*`. The
  last one was given the advice for arithmetic.
- The grammar page gave `NOT a@first` two derivations. `NOT` now has its own
  rule, which is how the parser has always read it. The page also states
  when a keyword is a name and how an unquoted value gets its type.

## v0.2.0 - 2026-11-17

### Added

- Strict parsing options, for a host whose queries come from scripts and
  saved links rather than a search box. `query.WithExplicitAnd()` requires
  `AND` or `OR` between conditions, so `a=1 b=2`, `frontend checkout` and
  `labels[team]=x` are errors with the new code `missingAnd`.
  `query.WithStrictValues()` refuses `=`, `<` and `>` inside an unquoted
  value, so `a=1+b=2` is an error instead of a comparison with the text
  `1+b=2`. `query.WithStrict()` turns both on. The same switches are
  `parser.ParseWith` with a `parser.Config`, and `eval.WithExplicitAnd()`,
  `eval.WithStrictValues()` and `eval.WithStrict()`. `query.MatchWith` takes the same options as `query.Parse`, so a host that
  parses strictly matches strictly. `query.EvalWith` accepts the query length
  option for value expressions. The existing `query.Match` and `query.Eval`
  keep their original function signatures and default grammar. Both are off by default and only refuse
  queries: one accepted under the strict options parses to the same tree
  under the default grammar. The TypeScript client takes them as an object
  in place of the `maxLength` number (`parse(q, { explicitAnd: true })`), and
  the C library as the `"explicitAnd"` and `"strictValues"` keys of a
  request.
- Under a strict option, a hint that rewrites the query is checked against
  that option, so it is never a query the host would refuse. A keyed lookup
  such as `labels["team"]=x` is told to use a dot (`labels.team=x`), and a
  comparison inside a value is told both ways out: join two conditions with
  `AND`, or quote the value. A missing connective is reported together with a
  lexical error that follows it, instead of only after that error is fixed.
- The TypeScript client and the C library refuse a strict switch that is
  not a boolean (`explicitAnd: 1`), instead of parsing leniently.
- Under `ExplicitAnd`, an SQL word is reported once for each time it is
  used. `name LIKE john` gave the "there is no LIKE" advice twice, once for
  each word left without a connective. A second SQL word, and a connective
  that is really missing after one (`a LIKE b c=1`), are still their own
  errors.
- A Grammar page in the documentation: the EBNF the parser implements, how
  each ambiguous form is read, and what the language leaves out on purpose.

- `query filter`, `query eval` and `query map` run a query over JSON records
  from stdin, one object per line or one array, and write the matching
  records, a value per record, or a reshaped object per record, the way `jq`
  does. Without `--schema` the fields a query names are typed from the
  records. `map` takes `name=<value expression>` entries or bare field paths;
  `eval --raw` writes strings bare.
- Hints. A parse or validation error now says what the writer most likely
  meant and how to write it, in a new `Hint` field on `parser.Error` and
  `validate.Error` (`hint` in the WASM and C envelopes, a `hint:` line in
  `query explain`, and under each error in the playground). Where the fix is
  mechanical the hint is the writer's own query corrected, and it is only
  offered if it parses: `status==active` gives `status=active`,
  `[base]*1.1 > 3` gives `float([base]*1.1)>3`, `3<total` gives `total>3`,
  `a=1 && b=2` gives `a=1 AND b=2`, `total BETWEEN 1 AND 5` gives
  `total:1..5`. Validation suggests the nearest field or function
  (`did you mean "total"?`), says which operators a field allows, and
  explains a value that does not fit its type. A hint names only fields and
  functions the caller may use, so `WithAllowedFields` and a custom validator
  are not revealed by it. Only the first eight errors of a query get a hint.
- `eval.Compile` suggests the nearest registered function for an unknown one
  (`unknown function "uper" (did you mean "upper"?)`).
- `query explain --schema` prints validation errors like syntax errors: one
  per failure, with its code, a marker under the text and its hint.
- Under `ExplicitAnd`, a query that fails for want of an `AND` because of a
  word borrowed from SQL gets that word's hint: `x BETWEEN 1 AND 5` is told
  `x:1..5`, and `name LIKE john`, `name IS NULL` and `SELECT a FROM t` get
  the same advice as under the default grammar, where the general advice to
  add the `AND` would only make the query wrong.

### Security

- A tree sent to `validate` or `stringify` through the WASM and C bridges is
  held to what the parser could have built. A selector name outside `first`,
  `last`, `any`, `all`, `none` and the anonymous form, an unknown operator and
  a `list` value are errors; before, such text was printed as written, so a
  tree could validate and then print as a wider query naming fields it was
  never checked against (`"selector": "first OR secret=a*"`).
- The same trees are bound in size. A string value or field name longer
  than the query length limit, or a tree whose JSON is over 1 MiB, fails with
  the `queryTooLong` code. These used to be parsed with no limit. `validate`
  and `stringify` take the optional `maxLength` the other calls take (WASM:
  a trailing number; C: the `"maxLength"` key), so a tree parsed under a
  larger limit is checked under the same one; 256 is the default and 0
  disables it.
- `ast.WildcardToLike` documents that its result needs `ESCAPE '\'` in the
  `LIKE` clause, and `docs/sql.md`, `examples/sql` and the npm example now
  write it. Without the clause, SQLite, Oracle and SQL Server read an escaped
  `%` or `_` in the pattern as a wildcard again. If you copied the earlier
  example, add the clause.

### Changed

- **Breaking for the C library.** A request key the call does not take is an
  error, so `"strict": true` is no longer a lenient parse. A caller that
  sends extra keys has to remove them; `clients/ffi/README.md` lists the keys
  of each call. An input of another call sent as `null` (`"fields": null` to
  `QueryParse`) is still read as absent, so a binding that fills one struct
  for every call keeps working.
- **Breaking for the TypeScript client.** A call refuses an option it does
  not know, an options argument that is neither a number nor an object, and
  an argument after the options. These were ignored before. `validate`,
  `stringify` and `eval` take `{maxLength}` only: they have no condition
  grammar to be strict about.
- Under `ExplicitAnd`, a missing connective no longer hides the token the
  parser stops at after it: `a=1 x 5` reports the missing `AND` and the stray
  `5`, where it reported only the first.
- Error messages name a token by its text (`unexpected "*"`) instead of its
  lexer form (`unexpected token *("*")`), and `Error()` appends the hint in
  parentheses. Codes and positions are unchanged, with one exception below.
  The type-mismatch message quotes the value: `value "abc" (string) does not
  fit field "total" (type decimal)`.
- A run of the same unexpected character is one error that spans the run, so
  `a && b` reports `unexpectedChar` once, at the `&&`, where it used to report
  it twice.

- A `"` inside an unquoted value (`A=x"y`, `A=*0%"`) is a parse error with
  the `unexpectedChar` code. Such a value could not be printed back: a
  pattern may not be quoted, and left bare the quote started a string for
  whatever read the query next, so `A=0%"* "A"` parsed while its printed
  form `A=0%"* AND A` did not. Quote the whole value and write the quote as
  `\"`.

### Fixed

- Hints no longer give advice that does not apply. A word inside a quoted
  value or a bracketed name is not read as SQL (`name="a select" AND total>`
  now says to add a value), and an SQL word only explains the clause it is
  in. A rewrite keeps the whole query and the field as written: `3<total.x`
  gives `total.x>3` where it gave `total>3`, `[total]:1-5` gives
  `[total]:1..5` where it gave `:1..5`, and `3<total AND 4<base` states the
  rule where it dropped the second condition. A rewrite longer than the
  caller's length limit is not offered. Every word of `name IS NULL` gets the
  missing-value advice, whether or not `name` is a declared field.
- A validation hint is built once per error. With a custom validator it used
  to ask about every declared field twice.

- An unquoted value that holds an unclosed call (`0%f(*`) is a string whether
  it is read inside a query or on its own, as with an unclosed group. The
  arithmetic look-ahead used to treat the open call as arithmetic when the
  value stood alone, so the bridge quoted the pattern away.
- A wildcard pattern that starts with `*` and holds an arithmetic character
  (`A=*0%"`) is read as a pattern. The lexer's arithmetic look-ahead could
  claim such a value and then refuse it, so a query like `A=*0%" "A"`
  parsed while its printed form `A=*0%" AND A` did not.

## v0.1.0 - 2026-11-16

The first tagged release. Everything below is relative to the untagged
commits consumers pinned before it.

### Removed

- The Dart and Flutter clients (`clients/flutter`, `clients/flutter_plugin`)
  and their Android and iOS builds. `clients/ffi` stays, as a plain C shared
  library for any language; it is built and tested on its own.

### Fixed

- The JSON tree the WASM and C bridges hand to clients carries a function used
  as a predicate (`contains(tags, "x")`) as a `call` node with its source
  form and the fields it reads. It used to be dropped: `parse` answered a null
  result, and `contains(tags, "x") AND b>1` arrived as a binary node with no
  left side, while `match` evaluated the same query correctly.
- The bridges carry an arithmetic, function or field-reference value in its
  printed form, which always reads back; the raw source with its spaces
  removed did not always.
- The query length limit counts characters, as its message says, not bytes.
  Every bridge entry point (`parseAndValidate`, `match`, `eval`) takes an
  optional `maxLength`; `0` disables the limit. They used to apply the default
  with no way to change it.
- A query that is not valid UTF-8 is a parse error (`unexpectedChar`) at the
  offending byte. It used to lex as a term that could not be carried to a
  client as JSON.
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

- A `fields` key on `call` nodes and on qualifiers with a `fieldFunc`, listing
  the fields the function reads, so a client's `fields()` sees them without
  parsing the source form. `ast.StringValue` prints one value as `ast.String`
  would.
- `FuzzBridgeRoundTrip`, which checks that every query the parser accepts
  survives the JSON bridge unchanged; it runs with `make fuzz` and in CI.
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
