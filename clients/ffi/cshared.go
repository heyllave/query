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
//	QueryParse            {"query", "maxLength"?, "explicitAnd"?, "strictValues"?} -> {"result": ast}
//	QueryValidate         {"ast", "fields", "maxLength"?} -> {"valid": bool, ...}
//	QueryStringify        {"ast", "maxLength"?}           -> {"result": string}
//	QueryParseAndValidate {"query", "fields", "maxLength"?, "explicitAnd"?, "strictValues"?} -> {"result": ast}
//	QueryMatch            {"query", "fields", "record", "maxLength"?, "explicitAnd"?, "strictValues"?} -> {"result": bool}
//	QueryEval             {"query", "fields", "record", "maxLength"?} -> {"result": value}
//	QueryComplete         {"query", "fields", "cursor"?, "utf16"?, "maxLength"?, "explicitAnd"?, "strictValues"?} -> {"result": completion}
//	QueryOffer            {"completion", "candidates"} -> {"result": items}
//
// "maxLength" is the query length limit in characters (default 256); 0
// disables it.
//
// "explicitAnd" and "strictValues" are booleans, false when absent, that turn
// off lenient forms of the grammar: with "explicitAnd" a space between two
// conditions is an error (code missingAnd), and with "strictValues" an
// unquoted value may not contain =, < or >. A value that is not a boolean is
// an error, and so is a key the call does not list.
//
// QueryComplete reports what can be written at "cursor" in a query that is
// still being typed. The cursor is a byte offset, or a count of UTF-16 code
// units when "utf16" is true, and the end of the query when absent; the
// span in the answer is in the same unit. QueryOffer turns candidates the
// host supplies (values it fetched, the keys of a nested field) into items
// written the way the grammar needs them.
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
	"fmt"
	"slices"
	"strings"
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
// maxLength reads the optional "maxLength" key, falling back to the default
// limit, and fails when it is present but not an integer.
func (r request) maxLength() (int, bridgejson.Envelope) {
	maxLen := bridgejson.DefaultMaxLength
	if v, ok := r["maxLength"]; ok {
		if err := json.Unmarshal(v, &maxLen); err != nil {
			return 0, bridgejson.Fail("maxLength must be an integer")
		}
	}
	return maxLen, nil
}

// options reads the parse options of a call that takes a query: "maxLength"
// and the optional booleans "explicitAnd" and "strictValues".
func (r request) options() (bridgejson.Options, bridgejson.Envelope) {
	maxLen, fail := r.maxLength()
	if fail != nil {
		return bridgejson.Options{}, fail
	}
	o := bridgejson.Options{MaxLength: maxLen}
	// In a fixed order, so two bad members always name the same one first.
	for _, sw := range []struct {
		key string
		dst *bool
	}{{"explicitAnd", &o.ExplicitAnd}, {"strictValues", &o.StrictValues}} {
		v, ok := r[sw.key]
		if !ok {
			continue
		}
		// A JSON null would unmarshal into a bool without complaint and
		// leave it false; it is not a boolean, so it is refused too.
		if string(v) != "true" && string(v) != "false" {
			return bridgejson.Options{}, bridgejson.Fail(sw.key + " must be a boolean")
		}
		*sw.dst = string(v) == "true"
	}
	return o, nil
}

// unknownKey returns a member of the request that is not in keys, the first
// in sorted order so the same request always names the same one. ok is false
// when every member is known; the name alone cannot say so, because "" is a
// key JSON allows.
func (r request) unknownKey(keys []string) (name string, ok bool) {
	var unknown []string
	for k, v := range r {
		if slices.Contains(keys, k) {
			continue
		}
		// A binding that fills one struct for every call sends the members
		// this call has no use for as null. That asks for nothing, so it is
		// read as absent; a strict switch is never excused this way.
		if string(v) == "null" && slices.Contains(inputKeys, k) {
			continue
		}
		unknown = append(unknown, k)
	}
	if len(unknown) == 0 {
		return "", false
	}
	slices.Sort(unknown)
	return unknown[0], true
}

// The members each call takes.
var (
	parseKeys    = []string{"query", "maxLength", "explicitAnd", "strictValues"}
	validateKeys = []string{"ast", "fields", "maxLength"}
	printKeys    = []string{"ast", "maxLength"}
	checkKeys    = []string{"query", "fields", "maxLength", "explicitAnd", "strictValues"}
	matchKeys    = []string{"query", "fields", "record", "maxLength", "explicitAnd", "strictValues"}
	evalKeys     = []string{"query", "fields", "record", "maxLength"}
	completeKeys = []string{"query", "fields", "cursor", "utf16", "maxLength", "explicitAnd", "strictValues"}
	offerKeys    = []string{"completion", "candidates"}
)

// inputKeys are the members that carry a call's input, as opposed to the
// switches that change how it is read.
var inputKeys = []string{"query", "ast", "fields", "record", "maxLength", "cursor", "completion", "candidates"}

func (r request) raw(key, fallback string) string {
	if v, ok := r[key]; ok {
		return string(v)
	}
	return fallback
}

// call decodes the request, runs fn and returns its envelope as a malloc'd C
// string the caller frees with QueryFree.
//
// keys are the members the call understands. Any other member is an error
// and not ignored: a host that sends "strict": true and gets an answer
// believes strict parsing is on.
func call(req *C.char, keys []string, fn func(request) bridgejson.Envelope) *C.char {
	env := bridgejson.Guard(func() bridgejson.Envelope {
		if req == nil {
			return bridgejson.Fail("invalid request JSON")
		}
		var r request
		if err := json.Unmarshal([]byte(C.GoString(req)), &r); err != nil || r == nil {
			return bridgejson.Fail("invalid request JSON")
		}
		if unknown, ok := r.unknownKey(keys); ok {
			return bridgejson.Fail(fmt.Sprintf("unknown key %q; this call takes %s", unknown, strings.Join(keys, ", ")))
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
	return call(req, parseKeys, func(r request) bridgejson.Envelope {
		opts, fail := r.options()
		if fail != nil {
			return fail
		}
		return bridgejson.ParseWith(r.str("query"), opts)
	})
}

//export QueryValidate
func QueryValidate(req *C.char) *C.char {
	return call(req, validateKeys, func(r request) bridgejson.Envelope {
		maxLen, fail := r.maxLength()
		if fail != nil {
			return fail
		}
		return bridgejson.Validate(r.raw("ast", ""), r.raw("fields", "[]"), maxLen)
	})
}

//export QueryStringify
func QueryStringify(req *C.char) *C.char {
	return call(req, printKeys, func(r request) bridgejson.Envelope {
		maxLen, fail := r.maxLength()
		if fail != nil {
			return fail
		}
		return bridgejson.Stringify(r.raw("ast", ""), maxLen)
	})
}

//export QueryParseAndValidate
func QueryParseAndValidate(req *C.char) *C.char {
	return call(req, checkKeys, func(r request) bridgejson.Envelope {
		opts, fail := r.options()
		if fail != nil {
			return fail
		}
		return bridgejson.ParseAndValidateWith(r.str("query"), r.raw("fields", "[]"), opts)
	})
}

//export QueryMatch
func QueryMatch(req *C.char) *C.char {
	return call(req, matchKeys, func(r request) bridgejson.Envelope {
		opts, fail := r.options()
		if fail != nil {
			return fail
		}
		return bridgejson.MatchWith(r.str("query"), r.raw("fields", "[]"), r.raw("record", "{}"), opts)
	})
}

//export QueryEval
func QueryEval(req *C.char) *C.char {
	return call(req, evalKeys, func(r request) bridgejson.Envelope {
		maxLen, fail := r.maxLength()
		if fail != nil {
			return fail
		}
		return bridgejson.Eval(r.str("query"), r.raw("fields", "[]"), r.raw("record", "{}"), maxLen)
	})
}

// QueryComplete reports what can be written at "cursor" in "query", a byte
// offset unless "utf16" is true, and the end of the query when absent.
//
//export QueryComplete
func QueryComplete(req *C.char) *C.char {
	return call(req, completeKeys, func(r request) bridgejson.Envelope {
		opts, fail := r.options()
		if fail != nil {
			return fail
		}
		query := r.str("query")
		inUTF16 := false
		if v, ok := r["utf16"]; ok {
			if string(v) != "true" && string(v) != "false" {
				return bridgejson.Fail("utf16 must be a boolean")
			}
			inUTF16 = string(v) == "true"
		}
		cursor := len(query)
		if inUTF16 {
			cursor = bridgejson.UTF16Len(query)
		}
		if v, ok := r["cursor"]; ok && string(v) != "null" {
			if err := json.Unmarshal(v, &cursor); err != nil {
				return bridgejson.Fail("cursor must be an integer")
			}
		}
		return bridgejson.Complete(query, r.raw("fields", "[]"), cursor, inUTF16, opts)
	})
}

// QueryOffer writes the host's "candidates" as items for a "completion"
// that QueryComplete returned.
//
//export QueryOffer
func QueryOffer(req *C.char) *C.char {
	return call(req, offerKeys, func(r request) bridgejson.Envelope {
		return bridgejson.Offer(r.raw("completion", "null"), r.raw("candidates", "null"))
	})
}

//export QueryFree
func QueryFree(p *C.char) {
	C.free(unsafe.Pointer(p))
}
