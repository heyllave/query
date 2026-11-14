# libquery — the query language for any language

A C shared library around the Go engine, for languages other than Go (use the
module) and JavaScript (use `clients/npm`). It answers with the same JSON as
the WASM build, from the same code (`internal/bridgejson`).

```bash
make build   # libquery.so or libquery.dylib, and libquery.h
make test    # runs conformance/corpus.json through the library from Python
```

## Calling it

Each function takes one NUL-terminated UTF-8 JSON object and returns one.

| Function | Request | Success |
|---|---|---|
| `QueryParse` | `{"query", "maxLength"?}` | `{"result": ast}` |
| `QueryValidate` | `{"ast", "fields"}` | `{"valid": true}` or `{"valid": false, "errors", "validationErrors"}` |
| `QueryStringify` | `{"ast"}` | `{"result": "query text"}` |
| `QueryParseAndValidate` | `{"query", "fields"}` | `{"result": ast}` |
| `QueryMatch` | `{"query", "fields", "record"}` | `{"result": true}` |
| `QueryEval` | `{"query", "fields", "record"}` | `{"result": value}` |

A failure is `{"error": "message"}` with `errors` (syntax) or
`validationErrors` (validation): lists of `{code, message, offset, length}`.
Show your own wording from `code`; the codes do not change, the messages may.

A field is `{"name": "total", "type": "decimal"}`, optionally with
`"allowedOps": ["=", ">"]`, `"nested": true` for label-style maps.

## Rules

- Free every returned string with `QueryFree`, exactly once. Never pass a
  request to `QueryFree`.
- Any function may be called from any thread at any time.
- No call returns `NULL` and no call aborts the process, whatever it is given.

`conformance_test.py` is a complete binding using only Python's standard
library; a binding for another language is the same forty lines.
