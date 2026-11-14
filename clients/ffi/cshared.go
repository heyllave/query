//go:build cshared

// Command libquery is the C shared-library build of the query language: the
// way in for any language that can call C (Python, Ruby, Java, C#, Rust,
// Swift, ...). It mirrors the WASM bridge function for function and answers
// with the same JSON envelopes, both produced by internal/bridgejson, so a
// native client and a JavaScript one cannot drift apart.
//
// Build:
//
//	make -C clients/ffi build      # libquery.so / .dylib and libquery.h
//
// The build tag keeps this package — which uses cgo and package main — out of
// `go build ./...`, so the pure-Go library and the WASM target are unaffected.
//
// # Calling convention
//
// Every function takes one NUL-terminated UTF-8 JSON object and returns one:
//
//	QueryParse            {"query", "maxLength"?}       -> {"result": ast}
//	QueryValidate         {"ast", "fields"}             -> {"valid": bool, ...}
//	QueryStringify        {"ast"}                       -> {"result": string}
//	QueryParseAndValidate {"query", "fields"}           -> {"result": ast}
//	QueryMatch            {"query", "fields", "record"} -> {"result": bool}
//	QueryEval             {"query", "fields", "record"} -> {"result": value}
//
// A failure answers {"error": message} with "errors" (parse) or
// "validationErrors" (validation) listing each failure's stable code and span.
// No call returns NULL and no call aborts the process.
//
// # Memory ownership
//
// The caller owns the request string and frees it its own way. Each response
// is malloc'd here and must be freed exactly once with QueryFree. Never pass a
// request to QueryFree; never free a response twice.
//
// # Threads
//
// Every function is safe to call from any number of threads at once; the
// library keeps no state between calls.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/heyllave/query/internal/bridgejson"
)

func main() {}

// request is the decoded JSON object a call was made with.
type request map[string]json.RawMessage

// str reads a string member; anything else reads as "".
func (r request) str(key string) string {
	var s string
	_ = json.Unmarshal(r[key], &s)
	return s
}

// raw hands a member on as JSON text, or fallback when it is absent.
func (r request) raw(key, fallback string) string {
	if v, ok := r[key]; ok {
		return string(v)
	}
	return fallback
}

// call decodes the request, runs fn and returns its envelope as a malloc'd C
// string the caller frees with QueryFree.
func call(req *C.char, fn func(request) bridgejson.Envelope) *C.char {
	env := bridgejson.Guard(func() bridgejson.Envelope {
		if req == nil {
			return bridgejson.Fail("invalid request JSON")
		}
		var r request
		if err := json.Unmarshal([]byte(C.GoString(req)), &r); err != nil || r == nil {
			return bridgejson.Fail("invalid request JSON")
		}
		return fn(r)
	})
	b, err := json.Marshal(env)
	if err != nil {
		b, _ = json.Marshal(bridgejson.Fail("result is not representable in JSON: " + err.Error()))
	}
	return C.CString(string(b))
}

//export QueryParse
func QueryParse(req *C.char) *C.char {
	return call(req, func(r request) bridgejson.Envelope {
		maxLen := bridgejson.DefaultMaxLength
		if v, ok := r["maxLength"]; ok {
			if err := json.Unmarshal(v, &maxLen); err != nil {
				return bridgejson.Fail("maxLength must be an integer")
			}
		}
		return bridgejson.Parse(r.str("query"), maxLen)
	})
}

//export QueryValidate
func QueryValidate(req *C.char) *C.char {
	return call(req, func(r request) bridgejson.Envelope {
		return bridgejson.Validate(r.raw("ast", ""), r.raw("fields", "[]"))
	})
}

//export QueryStringify
func QueryStringify(req *C.char) *C.char {
	return call(req, func(r request) bridgejson.Envelope {
		return bridgejson.Stringify(r.raw("ast", ""))
	})
}

//export QueryParseAndValidate
func QueryParseAndValidate(req *C.char) *C.char {
	return call(req, func(r request) bridgejson.Envelope {
		return bridgejson.ParseAndValidate(r.str("query"), r.raw("fields", "[]"))
	})
}

//export QueryMatch
func QueryMatch(req *C.char) *C.char {
	return call(req, func(r request) bridgejson.Envelope {
		return bridgejson.Match(r.str("query"), r.raw("fields", "[]"), r.raw("record", "{}"))
	})
}

//export QueryEval
func QueryEval(req *C.char) *C.char {
	return call(req, func(r request) bridgejson.Envelope {
		return bridgejson.Eval(r.str("query"), r.raw("fields", "[]"), r.raw("record", "{}"))
	})
}

//export QueryFree
func QueryFree(p *C.char) {
	C.free(unsafe.Pointer(p))
}
