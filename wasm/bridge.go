//go:build wasm

package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"syscall/js"

	"github.com/heyllave/query/internal/bridgejson"
)

// bridge adapts a call to a JavaScript function. need is how many leading
// arguments are required, all strings, and one optional argument may follow
// them; the envelope comes from internal/bridgejson, which the C bridge
// shares, so every client sees the same shapes. A Go panic becomes an error
// envelope instead of tearing down the WASM instance (and with it every later
// call from the page).
func bridge(name string, need int, call func(args []js.Value) bridgejson.Envelope) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		return toJSValue(bridgejson.Guard(func() bridgejson.Envelope {
			// A wrapper that forwards an optional argument passes undefined.
			for len(args) > need && absent(args[len(args)-1]) {
				args = args[:len(args)-1]
			}
			if len(args) < need {
				return bridgejson.Fail(name + ": missing arguments")
			}
			// An argument past the last one would be ignored, and the caller
			// who passed options there would not know.
			if len(args) > need+1 {
				return bridgejson.Fail(name + ": too many arguments")
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

// The options each kind of call takes. Only a call that parses a condition
// has a grammar to be strict about.
var (
	lengthOption    = []string{"maxLength"}
	parseOptions    = []string{"maxLength", "explicitAnd", "strictValues"}
	completeOptions = []string{"cursor", "maxLength", "explicitAnd", "strictValues"}
)

// maxLengthArg reads the optional last argument of a call that takes no
// grammar switches: the maxLength number, or an object {maxLength?}.
func maxLengthArg(args []js.Value, i int) (int, bridgejson.Envelope) {
	o, fail := optionsArg(args, i, lengthOption)
	return o.MaxLength, fail
}

// optionsArg reads the optional last argument of a call. It is either the
// maxLength number, as before, or an object with the members named in known;
// a member that is absent keeps its default.
//
// Anything else is an error and not a default: a member of the wrong type, a
// member the call does not know, or an argument that is neither a number nor
// an object. A host that passes explicitAnd: 1, strict: true or a bare true
// believes strict parsing is on, and must not be answered as if it were.
func optionsArg(args []js.Value, i int, known []string) (bridgejson.Options, bridgejson.Envelope) {
	o := bridgejson.Options{MaxLength: bridgejson.DefaultMaxLength}
	if len(args) <= i || absent(args[i]) || args[i].Type() == js.TypeNull {
		return o, nil
	}
	arg := args[i]
	if arg.Type() == js.TypeNumber {
		o.MaxLength = arg.Int()
		return o, nil
	}
	list := strings.Join(known, ", ")
	if arg.Type() != js.TypeObject || js.Global().Get("Array").Call("isArray", arg).Bool() {
		return o, bridgejson.Fail("options must be a number, the maxLength, or an object with " + list)
	}
	// Sorted, as the C library does, so both name the same member when
	// several are unknown. A member set to undefined is an absent one.
	names := js.Global().Get("Object").Call("keys", arg)
	var unknown []string
	for k := 0; k < names.Length(); k++ {
		if name := names.Index(k).String(); !slices.Contains(known, name) && !absent(arg.Get(name)) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return o, bridgejson.Fail(fmt.Sprintf("unknown option %q; this call takes %s", unknown[0], list))
	}
	if v := arg.Get("maxLength"); !absent(v) {
		if v.Type() != js.TypeNumber {
			return o, bridgejson.Fail("maxLength must be a number")
		}
		o.MaxLength = v.Int()
	}
	for _, sw := range []struct {
		key string
		dst *bool
	}{{"explicitAnd", &o.ExplicitAnd}, {"strictValues", &o.StrictValues}} {
		v := arg.Get(sw.key)
		if absent(v) || !slices.Contains(known, sw.key) {
			continue
		}
		if v.Type() != js.TypeBoolean {
			return o, bridgejson.Fail(sw.key + " must be a boolean")
		}
		*sw.dst = v.Bool()
	}
	return o, nil
}

// jsComplete is queryComplete(query, fieldsJSON, maxLength? | options?). The
// last argument is what every other call takes, a number for maxLength or an
// options object, and the object also carries cursor. The cursor counts
// UTF-16 code units, as a JavaScript string is indexed, and so does the span
// in the answer. Absent or null, as in the C library, it is the end of the
// query.
func jsComplete(args []js.Value) bridgejson.Envelope {
	query := args[0].String()
	cursor := bridgejson.UTF16Len(query)
	o, fail := optionsArg(args, 2, completeOptions)
	if fail != nil {
		return fail
	}
	if len(args) > 2 && args[2].Type() == js.TypeObject {
		if at := args[2].Get("cursor"); !absent(at) && at.Type() != js.TypeNull {
			if at.Type() != js.TypeNumber || at.Float() != float64(at.Int()) {
				return bridgejson.Fail("cursor must be an integer")
			}
			cursor = at.Int()
		}
	}
	return bridgejson.Complete(query, args[1].String(), cursor, true, o)
}

// jsOffer is queryOffer(completionJSON, candidatesJSON).
func jsOffer(args []js.Value) bridgejson.Envelope {
	if len(args) > 2 {
		return bridgejson.Fail("queryOffer: too many arguments")
	}
	return bridgejson.Offer(args[0].String(), args[1].String())
}

func absent(v js.Value) bool { return v.Type() == js.TypeUndefined }

// jsParse is queryParse(query, maxLength? | options?).
func jsParse(args []js.Value) bridgejson.Envelope {
	o, fail := optionsArg(args, 1, parseOptions)
	if fail != nil {
		return fail
	}
	return bridgejson.ParseWith(args[0].String(), o)
}

// jsValidate is queryValidate(astJSON, fieldsJSON, maxLength?).
func jsValidate(args []js.Value) bridgejson.Envelope {
	maxLen, fail := maxLengthArg(args, 2)
	if fail != nil {
		return fail
	}
	return bridgejson.Validate(args[0].String(), args[1].String(), maxLen)
}

// jsStringify is queryStringify(astJSON, maxLength?).
func jsStringify(args []js.Value) bridgejson.Envelope {
	maxLen, fail := maxLengthArg(args, 1)
	if fail != nil {
		return fail
	}
	return bridgejson.Stringify(args[0].String(), maxLen)
}

// jsParseAndValidate is queryParseAndValidate(query, fieldsJSON, maxLength? | options?).
func jsParseAndValidate(args []js.Value) bridgejson.Envelope {
	o, fail := optionsArg(args, 2, parseOptions)
	if fail != nil {
		return fail
	}
	return bridgejson.ParseAndValidateWith(args[0].String(), args[1].String(), o)
}

// jsMatch is queryMatch(query, fieldsJSON, recordJSON, maxLength? | options?).
func jsMatch(args []js.Value) bridgejson.Envelope {
	o, fail := optionsArg(args, 3, parseOptions)
	if fail != nil {
		return fail
	}
	return bridgejson.MatchWith(args[0].String(), args[1].String(), args[2].String(), o)
}

// jsEval is queryEval(query, fieldsJSON, recordJSON, maxLength?).
func jsEval(args []js.Value) bridgejson.Envelope {
	maxLen, fail := maxLengthArg(args, 3)
	if fail != nil {
		return fail
	}
	return bridgejson.Eval(args[0].String(), args[1].String(), args[2].String(), maxLen)
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
