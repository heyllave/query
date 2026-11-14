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

// jsParse is queryParse(query, maxLength?).
func jsParse(args []js.Value) bridgejson.Envelope {
	maxLen := bridgejson.DefaultMaxLength
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		maxLen = args[1].Int()
	}
	return bridgejson.Parse(args[0].String(), maxLen)
}

// jsValidate is queryValidate(astJSON, fieldsJSON).
func jsValidate(args []js.Value) bridgejson.Envelope {
	return bridgejson.Validate(args[0].String(), args[1].String())
}

// jsStringify is queryStringify(astJSON).
func jsStringify(args []js.Value) bridgejson.Envelope {
	return bridgejson.Stringify(args[0].String())
}

// jsParseAndValidate is queryParseAndValidate(query, fieldsJSON).
func jsParseAndValidate(args []js.Value) bridgejson.Envelope {
	return bridgejson.ParseAndValidate(args[0].String(), args[1].String())
}

// jsMatch is queryMatch(query, fieldsJSON, recordJSON).
func jsMatch(args []js.Value) bridgejson.Envelope {
	return bridgejson.Match(args[0].String(), args[1].String(), args[2].String())
}

// jsEval is queryEval(query, fieldsJSON, recordJSON).
func jsEval(args []js.Value) bridgejson.Envelope {
	return bridgejson.Eval(args[0].String(), args[1].String(), args[2].String())
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
