//go:build wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/heyllave/query/internal/bridgejson"
)

// bridge adapts a call to a JavaScript function. need is how many leading
// arguments are required, all strings; the envelope comes from
// internal/bridgejson, which the C bridge shares, so every client sees the
// same shapes. A Go panic becomes an error envelope instead of tearing down
// the WASM instance (and with it every later call from the page).
func bridge(name string, need int, call func(args []js.Value) bridgejson.Envelope) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		return toJSValue(bridgejson.Guard(func() bridgejson.Envelope {
			if len(args) < need {
				return bridgejson.Fail(name + ": missing arguments")
			}
			for _, a := range args[:need] {
				if a.Type() != js.TypeString {
					return bridgejson.Fail(name + ": arguments must be strings")
				}
			}
			return call(args)
		}))
	})
}

// maxLengthArg reads the optional maxLength argument at index i, falling back
// to the default limit when it is absent or not a number.
func maxLengthArg(args []js.Value, i int) int {
	if len(args) > i && args[i].Type() == js.TypeNumber {
		return args[i].Int()
	}
	return bridgejson.DefaultMaxLength
}

// jsParse is queryParse(query, maxLength?).
func jsParse(args []js.Value) bridgejson.Envelope {
	return bridgejson.Parse(args[0].String(), maxLengthArg(args, 1))
}

// jsValidate is queryValidate(astJSON, fieldsJSON, maxLength?).
func jsValidate(args []js.Value) bridgejson.Envelope {
	return bridgejson.Validate(args[0].String(), args[1].String(), maxLengthArg(args, 2))
}

// jsStringify is queryStringify(astJSON, maxLength?).
func jsStringify(args []js.Value) bridgejson.Envelope {
	return bridgejson.Stringify(args[0].String(), maxLengthArg(args, 1))
}

// jsParseAndValidate is queryParseAndValidate(query, fieldsJSON, maxLength?).
func jsParseAndValidate(args []js.Value) bridgejson.Envelope {
	return bridgejson.ParseAndValidate(args[0].String(), args[1].String(), maxLengthArg(args, 2))
}

// jsMatch is queryMatch(query, fieldsJSON, recordJSON, maxLength?).
func jsMatch(args []js.Value) bridgejson.Envelope {
	return bridgejson.Match(args[0].String(), args[1].String(), args[2].String(), maxLengthArg(args, 3))
}

// jsEval is queryEval(query, fieldsJSON, recordJSON, maxLength?).
func jsEval(args []js.Value) bridgejson.Envelope {
	return bridgejson.Eval(args[0].String(), args[1].String(), args[2].String(), maxLengthArg(args, 3))
}

// toJSValue converts an envelope to a JavaScript object by way of JSON, so a
// value JSON cannot carry (NaN, an infinity) becomes an error rather than a
// malformed object.
func toJSValue(env bridgejson.Envelope) js.Value {
	data, err := json.Marshal(env)
	if err != nil {
		data, _ = json.Marshal(bridgejson.Fail("result is not representable in JSON: " + err.Error()))
	}
	return js.Global().Get("JSON").Call("parse", string(data))
}
