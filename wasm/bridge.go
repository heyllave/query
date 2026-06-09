//go:build wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// jsParse parses a query string and returns the AST as a JSON object.
//
// JS signature: queryParse(query: string, maxLength?: number) => { ast?: object, error?: string }
func jsParse(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return jsResult(nil, "queryParse requires a query string argument")
	}
	q := args[0].String()
	maxLen := 256
	if len(args) > 1 && !args[1].IsUndefined() {
		maxLen = args[1].Int()
	}

	expr, err := parser.Parse(q, maxLen)
	if err != nil {
		return jsResult(nil, err.Error())
	}

	node := astToJSON(expr)
	return jsResult(node, "")
}

// jsValidate validates a JSON AST against field configurations.
//
// JS signature: queryValidate(astJSON: string, fieldsJSON: string) => { valid: boolean, errors?: string[] }
func jsValidate(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return jsResult(nil, "queryValidate requires ast and fields arguments")
	}

	astJSON := args[0].String()
	fieldsJSON := args[1].String()

	expr, err := jsonToAST(astJSON)
	if err != nil {
		return jsResult(nil, "invalid AST: "+err.Error())
	}

	var fields []validate.FieldConfig
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return jsResult(nil, "invalid fields config: "+err.Error())
	}

	v := validate.New(fields)
	if err := v.Validate(expr); err != nil {
		result := map[string]any{
			"valid":  false,
			"errors": []string{err.Error()},
		}
		return toJSValue(result)
	}
	return toJSValue(map[string]any{"valid": true})
}

// jsStringify converts a JSON AST back to a query string.
//
// JS signature: queryStringify(astJSON: string) => { query?: string, error?: string }
func jsStringify(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return jsResult(nil, "queryStringify requires an AST argument")
	}

	expr, err := jsonToAST(args[0].String())
	if err != nil {
		return jsResult(nil, "invalid AST: "+err.Error())
	}

	return jsResult(ast.String(expr), "")
}

// jsParseAndValidate parses and validates in one call.
//
// JS signature: queryParseAndValidate(query: string, fieldsJSON: string) => { ast?: object, error?: string }
func jsParseAndValidate(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return jsResult(nil, "queryParseAndValidate requires query and fields arguments")
	}

	q := args[0].String()
	fieldsJSON := args[1].String()

	expr, err := parser.Parse(q, 256)
	if err != nil {
		return jsResult(nil, err.Error())
	}

	var fields []validate.FieldConfig
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return jsResult(nil, "invalid fields config: "+err.Error())
	}

	v := validate.New(fields)
	if err := v.Validate(expr); err != nil {
		return jsResult(nil, err.Error())
	}

	node := astToJSON(expr)
	return jsResult(node, "")
}

// jsMatch compiles a boolean predicate and evaluates it against a record.
//
// JS signature: queryMatch(query: string, fieldsJSON: string, recordJSON: string)
//
//	=> { result?: boolean, error?: string }
func jsMatch(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return jsResult(nil, "queryMatch requires query, fields, and record arguments")
	}

	fields, err := parseFields(args[1].String())
	if err != nil {
		return jsResult(nil, err.Error())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(args[2].String()), &record); err != nil {
		return jsResult(nil, "invalid record: "+err.Error())
	}

	prog, err := eval.Compile(args[0].String(), fields)
	if err != nil {
		return jsResult(nil, err.Error())
	}
	return jsResult(prog.Match(record), "")
}

// jsEval compiles a value expression and evaluates it against a record,
// returning the computed value.
//
// JS signature: queryEval(query: string, fieldsJSON: string, recordJSON: string)
//
//	=> { result?: any, error?: string }
func jsEval(_ js.Value, args []js.Value) any {
	if len(args) < 3 {
		return jsResult(nil, "queryEval requires query, fields, and record arguments")
	}

	fields, err := parseFields(args[1].String())
	if err != nil {
		return jsResult(nil, err.Error())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(args[2].String()), &record); err != nil {
		return jsResult(nil, "invalid record: "+err.Error())
	}

	prog, err := eval.CompileValue(args[0].String(), fields)
	if err != nil {
		return jsResult(nil, err.Error())
	}
	v, err := prog.Eval(record)
	if err != nil {
		return jsResult(nil, err.Error())
	}
	return jsResult(v, "")
}

// parseFields decodes a field-config JSON array shared by the eval bridges.
func parseFields(fieldsJSON string) ([]validate.FieldConfig, error) {
	var fields []validate.FieldConfig
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return nil, fmt.Errorf("invalid fields config: %w", err)
	}
	return fields, nil
}

// jsResult creates a {result, error} JS object.
func jsResult(result any, errMsg string) any {
	obj := map[string]any{}
	if errMsg != "" {
		obj["error"] = errMsg
	}
	if result != nil {
		switch v := result.(type) {
		case string:
			obj["result"] = v
		default:
			obj["result"] = toJSValue(v)
		}
	}
	return toJSValue(obj)
}

// toJSValue converts a Go value to a js.Value by marshaling through JSON.
func toJSValue(v any) js.Value {
	data, err := json.Marshal(v)
	if err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}
	return js.Global().Get("JSON").Call("parse", string(data))
}
