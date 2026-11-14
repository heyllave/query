# Security

This library exists to take a query string from an untrusted person and turn
it into a tree, a yes-or-no answer, or a value. Treat the query as hostile; the
library does.

## What the library guarantees

- **No input makes it panic, hang or run code.** The language has no loops, no
  assignment and no way to define a function. Parsing and evaluation are
  bounded by the query length (256 bytes by default), by `parser.MaxNesting`
  levels of nesting and by `eval.MaxCalendarDays` for day arithmetic.
- **Restrictions hold.** A field outside `WithAllowedFields`, an operator
  outside a field's `AllowedOps` or `WithAllowedOps`, and a function outside
  `validate.WithAllowedFunctions` are refused however the query reaches for
  them: directly, through a function, or from the value side of a comparison.
- **It produces no SQL.** Turning a tree into SQL is the caller's code. Bind
  every literal as a parameter and map every field and function through an
  allowlist; `examples/sql` shows one way. A tree is not safe to interpolate
  because it validated.

A way to break any of these is a vulnerability.

## What is the caller's responsibility

- Functions registered with `WithFunctions` run with whatever arguments the
  query supplies. A panic inside one is contained, but its running time and
  side effects are its own.
- With the length limit disabled (`WithMaxLength(0)`), cost grows with the
  query. Keep a limit on anything a stranger can send.
- Records are host data. The evaluator reads them and never writes to them.

## Reporting

Please report a vulnerability privately through GitHub's "Report a
vulnerability" button on the repository's Security tab rather than in a public
issue. Include the query, the field config and what you expected. Fixes land on
the main branch and, once releases are tagged, in the latest minor version.
