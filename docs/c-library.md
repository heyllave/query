---
title: C library
nav_order: 3
parent: Clients
permalink: /c-library/
---

# C library, for every other language
{: .no_toc }

1. TOC
{:toc}

`libquery` is a C shared library around the Go engine. Use it from Python, Ruby, Rust, Java, C#, Swift or anything else that can call C. It answers with the same JSON as the WebAssembly build, from the same code.

## Build

```bash
make -C clients/ffi build   # libquery.so or libquery.dylib, and libquery.h
make -C clients/ffi test    # runs the shared corpus through the library from Python
```

The build needs Go 1.25 or newer and a C compiler.

## Functions

Each function takes one NUL-terminated UTF-8 JSON object and returns one.

| Function | Request | Success |
|---|---|---|
| `QueryParse` | `{"query", "maxLength"?, "explicitAnd"?, "strictValues"?}` | `{"result": tree}` |
| `QueryValidate` | `{"ast", "fields", "maxLength"?}` | `{"valid": true}` |
| `QueryStringify` | `{"ast", "maxLength"?}` | `{"result": "query text"}` |
| `QueryParseAndValidate` | `{"query", "fields", "maxLength"?, "explicitAnd"?, "strictValues"?}` | `{"result": tree}` |
| `QueryMatch` | `{"query", "fields", "record", "maxLength"?, "explicitAnd"?, "strictValues"?}` | `{"result": true}` |
| `QueryEval` | `{"query", "fields", "record", "maxLength"?}` | `{"result": value}` |

`maxLength` is the query length limit in characters; it defaults to 256 and `0` disables it. For a tree, it bounds the text the tree carries as written (string values, field names), so a tree parsed under a larger limit is validated or printed under the same one.

`QueryParse`, `QueryParseAndValidate` and `QueryMatch` also take two optional booleans, `"explicitAnd"` and `"strictValues"`, which turn off lenient forms of the grammar: `{"query": "a=1 b=2", "explicitAnd": true}` fails with the code `missingAnd`. A key a call does not take is an error, so a misspelt option cannot pass for one. An input of another call sent as `null` (`"fields": null` to `QueryParse`) is read as absent. See [Grammar](../grammar/#strict-options).

A field in `"fields"` may be a list: `{"name": "orders", "type": "list", "elements": [{"name": "status", "type": "text"}]}`. The condition inside a selector is then checked against the element's fields. See [List fields](../go/#list-fields).

A failure is `{"error": "message"}` with a list of coded entries: `errors` for syntax, `validationErrors` for validation. Each entry is `{code, message, offset, length}`, plus `hint` when the engine can tell what was meant. Show your own wording from `code`; the codes do not change, the messages may.

A field is `{"name": "total", "type": "decimal"}`. Add `"allowedOps": ["=", ">"]` to narrow the operators, or `"nested": true` for label-style maps.

## Rules

- Free every returned string with `QueryFree`, exactly once. Never pass a request to `QueryFree`.
- Any function may be called from any thread at any time.
- No call returns `NULL` and no call aborts the process, whatever it is given.

## A complete binding in Python

```python
import ctypes, json

lib = ctypes.CDLL("./libquery.so")
for name in ("QueryParse", "QueryValidate", "QueryStringify",
             "QueryParseAndValidate", "QueryMatch", "QueryEval"):
    fn = getattr(lib, name)
    fn.argtypes = [ctypes.c_char_p]
    fn.restype = ctypes.c_void_p
lib.QueryFree.argtypes = [ctypes.c_void_p]

def call(name, **request):
    ptr = getattr(lib, name)(json.dumps(request).encode())
    try:
        return json.loads(ctypes.string_at(ptr).decode())
    finally:
        lib.QueryFree(ptr)

call("QueryMatch",
     query="labels.team=payments AND replicas>=2",
     fields=[{"name": "labels", "type": "text", "nested": True},
             {"name": "replicas", "type": "integer"}],
     record={"labels": {"team": "payments"}, "replicas": 3})
# {"result": True}
```

A binding for another language follows the same steps: load, send JSON, read JSON, free. The test runner in [`clients/ffi/conformance_test.py`](https://github.com/heyllave/query/blob/main/clients/ffi/conformance_test.py) is a fuller version of this code.
