//go:build cshared

// Command libquery is the cgo c-shared bridge for the query language, consumed
// by the Dart/Flutter client over dart:ffi on native platforms. It mirrors the
// WASM/JS bridge function-for-function and shares the JSON contract via
// internal/bridgejson, so the native and web clients behave identically.
//
// Build (clang because the CI runner has no gcc):
//
//	CGO_ENABLED=1 CC=clang go build -buildmode=c-shared \
//	    -tags cshared -o libquery.so ./clients/ffi
//
// The build tag keeps this package — which uses cgo and package main — out of
// `go build ./...` even when CGO_ENABLED=1, so the pure-Go library and the WASM
// target are unaffected.
//
// # Memory ownership
//
// Each exported function takes a NUL-terminated UTF-8 JSON request string that
// the caller owns (the caller frees its own input). Each returns a malloc'd
// NUL-terminated UTF-8 JSON response that the CALLER must free exactly once with
// QueryFree. Never free an input with QueryFree; never free a response twice.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/internal/bridgejson"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

func main() {}

// respond marshals an envelope to a freshly-malloc'd C string owned by the caller.
func respond(obj map[string]any) *C.char {
	b, err := json.Marshal(obj)
	if err != nil {
		// Fall back to a minimal hand-built error so a marshal failure still
		// yields a parseable envelope rather than a null pointer.
		return C.CString(`{"error":"internal: failed to encode response"}`)
	}
	return C.CString(string(b))
}

func errResp(msg string) *C.char { return respond(map[string]any{"error": msg}) }

// parseReq decodes a request JSON object into a generic map.
func parseReq(req *C.char) (map[string]any, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(C.GoString(req)), &m); err != nil {
		return nil, false
	}
	return m, true
}

func reqString(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// reqJSON re-encodes a sub-object of the request (e.g. fields, record, ast) back
// to a JSON string so it can flow through the same decoders the WASM bridge uses.
func reqJSON(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

//export QueryParse
func QueryParse(req *C.char) *C.char {
	m, ok := parseReq(req)
	if !ok {
		return errResp("invalid request JSON")
	}
	maxLen := 256
	if f, ok := m["maxLength"].(float64); ok {
		maxLen = int(f)
	}
	expr, err := parser.Parse(reqString(m, "query"), maxLen)
	if err != nil {
		return errResp(err.Error())
	}
	return respond(map[string]any{"result": bridgejson.AstToJSON(expr)})
}

//export QueryValidate
func QueryValidate(req *C.char) *C.char {
	m, ok := parseReq(req)
	if !ok {
		return errResp("invalid request JSON")
	}
	expr, err := bridgejson.JSONToAST(reqJSON(m, "ast"))
	if err != nil {
		return errResp("invalid AST: " + err.Error())
	}
	fields, err := bridgejson.ParseFields(reqJSON(m, "fields"))
	if err != nil {
		return errResp(err.Error())
	}
	v := validate.New(fields)
	if err := v.Validate(expr); err != nil {
		return respond(map[string]any{"valid": false, "errors": []string{err.Error()}})
	}
	return respond(map[string]any{"valid": true})
}

//export QueryStringify
func QueryStringify(req *C.char) *C.char {
	m, ok := parseReq(req)
	if !ok {
		return errResp("invalid request JSON")
	}
	expr, err := bridgejson.JSONToAST(reqJSON(m, "ast"))
	if err != nil {
		return errResp("invalid AST: " + err.Error())
	}
	return respond(map[string]any{"result": ast.String(expr)})
}

//export QueryParseAndValidate
func QueryParseAndValidate(req *C.char) *C.char {
	m, ok := parseReq(req)
	if !ok {
		return errResp("invalid request JSON")
	}
	expr, err := parser.Parse(reqString(m, "query"), 256)
	if err != nil {
		return errResp(err.Error())
	}
	fields, err := bridgejson.ParseFields(reqJSON(m, "fields"))
	if err != nil {
		return errResp(err.Error())
	}
	if err := validate.New(fields).Validate(expr); err != nil {
		return errResp(err.Error())
	}
	return respond(map[string]any{"result": bridgejson.AstToJSON(expr)})
}

//export QueryMatch
func QueryMatch(req *C.char) *C.char {
	m, ok := parseReq(req)
	if !ok {
		return errResp("invalid request JSON")
	}
	fields, err := bridgejson.ParseFields(reqJSON(m, "fields"))
	if err != nil {
		return errResp(err.Error())
	}
	record, ok := m["record"].(map[string]any)
	if !ok {
		record = map[string]any{}
	}
	prog, err := eval.Compile(reqString(m, "query"), fields)
	if err != nil {
		return errResp(err.Error())
	}
	return respond(map[string]any{"result": prog.Match(record)})
}

//export QueryEval
func QueryEval(req *C.char) *C.char {
	m, ok := parseReq(req)
	if !ok {
		return errResp("invalid request JSON")
	}
	fields, err := bridgejson.ParseFields(reqJSON(m, "fields"))
	if err != nil {
		return errResp(err.Error())
	}
	record, ok := m["record"].(map[string]any)
	if !ok {
		record = map[string]any{}
	}
	prog, err := eval.CompileValue(reqString(m, "query"), fields)
	if err != nil {
		return errResp(err.Error())
	}
	v, err := prog.Eval(record)
	if err != nil {
		return errResp(err.Error())
	}
	return respond(map[string]any{"result": v})
}

//export QueryFree
func QueryFree(p *C.char) {
	C.free(unsafe.Pointer(p))
}
