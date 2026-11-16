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

// optionsArg reads the optional last argument of a call that parses a query.
// It is either the maxLength number, as before, or an object
// {maxLength?, explicitAnd?, strictValues?}; a member that is absent keeps
// its default. A member of the wrong type, or one the call does not know, is
// an error and not a default: a host that passes explicitAnd: 1 or
// strict: true believes strict parsing is on, and must not be answered as if
// it were.
func optionsArg(args []js.Value, i int) (bridgejson.Options, bridgejson.Envelope) {
	o := bridgejson.Options{MaxLength: maxLengthArg(args, i)}
	if len(args) <= i || args[i].Type() != js.TypeObject {
		return o, nil
	}
	obj := args[i]
	// A member this call does not know is an error for the same reason a
	// member of the wrong type is: {strict: true} must not be answered as if
	// strict parsing were on.
	names := js.Global().Get("Object").Call("keys", obj)
	for k := 0; k < names.Length(); k++ {
		switch name := names.Index(k).String(); name {
		case "maxLength", "explicitAnd", "strictValues":
		default:
			return o, bridgejson.Fail("unknown option \"" + name + "\"; the options are maxLength, explicitAnd, strictValues")
		}
	}
	if v := obj.Get("maxLength"); !absent(v) {
		if v.Type() != js.TypeNumber {
			return o, bridgejson.Fail("maxLength must be a number")
		}
		o.MaxLength = v.Int()
	}
	for _, sw := range []struct {
		key string
		dst *bool
	}{{"explicitAnd", &o.ExplicitAnd}, {"strictValues", &o.StrictValues}} {
		v := obj.Get(sw.key)
		if absent(v) {
			continue
		}
		if v.Type() != js.TypeBoolean {
			return o, bridgejson.Fail(sw.key + " must be a boolean")
		}
		*sw.dst = v.Bool()
	}
	return o, nil
}

func absent(v js.Value) bool { return v.Type() == js.TypeUndefined }

// jsParse is queryParse(query, maxLength? | options?).
func jsParse(args []js.Value) bridgejson.Envelope {
	o, fail := optionsArg(args, 1)
	if fail != nil {
		return fail
	}
	return bridgejson.ParseWith(args[0].String(), o)
}

// jsValidate is queryValidate(astJSON, fieldsJSON, maxLength?).
func jsValidate(args []js.Value) bridgejson.Envelope {
	return bridgejson.Validate(args[0].String(), args[1].String(), maxLengthArg(args, 2))
}

// jsStringify is queryStringify(astJSON, maxLength?).
func jsStringify(args []js.Value) bridgejson.Envelope {
	return bridgejson.Stringify(args[0].String(), maxLengthArg(args, 1))
}

// jsParseAndValidate is queryParseAndValidate(query, fieldsJSON, maxLength? | options?).
func jsParseAndValidate(args []js.Value) bridgejson.Envelope {
	o, fail := optionsArg(args, 2)
	if fail != nil {
		return fail
	}
	return bridgejson.ParseAndValidateWith(args[0].String(), args[1].String(), o)
}

// jsMatch is queryMatch(query, fieldsJSON, recordJSON, maxLength? | options?).
func jsMatch(args []js.Value) bridgejson.Envelope {
	o, fail := optionsArg(args, 3)
	if fail != nil {
		return fail
	}
	return bridgejson.MatchWith(args[0].String(), args[1].String(), args[2].String(), o)
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
