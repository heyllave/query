package bridgejson

import (
	"encoding/json"
	"fmt"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/parser"
	"github.com/heyllave/query/validate"
)

// DefaultMaxLength is the query length limit a bridge applies when the caller
// gives none.
const DefaultMaxLength = 256

// Envelope is the JSON object every bridge call answers with. A success
// carries "result" (or "valid" for [Validate]); a failure carries "error",
// the joined English message, plus "errors" for parse failures and
// "validationErrors" for validation failures, each a list of coded entries a
// client can render in its own words.
type Envelope map[string]any

// Guard runs a bridge call and turns a Go panic into an error envelope. A
// panic that escaped would tear down the WASM instance or abort the process
// hosting the shared library, which no query is allowed to do.
func Guard(fn func() Envelope) (env Envelope) {
	defer func() {
		if r := recover(); r != nil {
			env = Fail(fmt.Sprintf("internal error: %v", r))
		}
	}()
	return fn()
}

// Fail is the envelope for a failure that has only a message.
func Fail(msg string) Envelope { return Envelope{"error": msg} }

func ok(result any) Envelope { return Envelope{"result": result} }

// Parse parses query and answers with its AST. A maxLength of zero or less
// disables the length limit, as in [parser.Parse].
func Parse(query string, maxLength int) Envelope {
	expr, err := parser.Parse(query, maxLength)
	if err != nil {
		return parseFailure(err)
	}
	return ok(AstToJSON(expr))
}

// Validate checks a JSON AST against field configs and answers with
// {"valid": true} or {"valid": false, "errors", "validationErrors"}.
func Validate(astJSON, fieldsJSON string) Envelope {
	expr, err := JSONToAST(astJSON)
	if err != nil {
		return Fail("invalid AST: " + err.Error())
	}
	fields, err := ParseFields(fieldsJSON)
	if err != nil {
		return Fail(err.Error())
	}
	if err := validate.New(fields).Validate(expr); err != nil {
		env := Envelope{"valid": false, "errors": []string{err.Error()}}
		if list := ValidationErrors(err); len(list) > 0 {
			env["validationErrors"] = list
		}
		return env
	}
	return Envelope{"valid": true}
}

// Stringify prints a JSON AST back as a query string.
func Stringify(astJSON string) Envelope {
	expr, err := JSONToAST(astJSON)
	if err != nil {
		return Fail("invalid AST: " + err.Error())
	}
	return ok(ast.String(expr))
}

// ParseAndValidate parses query under the default length limit, validates it
// against the field configs and answers with its AST.
func ParseAndValidate(query, fieldsJSON string) Envelope {
	expr, err := parser.Parse(query, DefaultMaxLength)
	if err != nil {
		return parseFailure(err)
	}
	fields, err := ParseFields(fieldsJSON)
	if err != nil {
		return Fail(err.Error())
	}
	if err := validate.New(fields).Validate(expr); err != nil {
		return compileFailure(err)
	}
	return ok(AstToJSON(expr))
}

// Match compiles query as a predicate and evaluates it against a JSON record.
func Match(query, fieldsJSON, recordJSON string) Envelope {
	fields, record, fail := decodeInputs(fieldsJSON, recordJSON)
	if fail != nil {
		return fail
	}
	prog, err := eval.Compile(query, fields)
	if err != nil {
		return compileFailure(err)
	}
	return ok(prog.Match(record))
}

// Eval compiles query as a value expression and evaluates it against a JSON
// record.
func Eval(query, fieldsJSON, recordJSON string) Envelope {
	fields, record, fail := decodeInputs(fieldsJSON, recordJSON)
	if fail != nil {
		return fail
	}
	prog, err := eval.CompileValue(query, fields)
	if err != nil {
		return compileFailure(err)
	}
	v, err := prog.Eval(record)
	if err != nil {
		return Fail(err.Error())
	}
	return ok(v)
}

func decodeInputs(fieldsJSON, recordJSON string) ([]validate.FieldConfig, map[string]any, Envelope) {
	fields, err := ParseFields(fieldsJSON)
	if err != nil {
		return nil, nil, Fail(err.Error())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(recordJSON), &record); err != nil {
		return nil, nil, Fail("invalid record: " + err.Error())
	}
	return fields, record, nil
}

// parseFailure reports a parse failure: "error" is the joined message and
// "errors" lists each failure as {code, kind, message, offset, length}. A
// failure that is not a parse error reports its message alone.
func parseFailure(err error) Envelope {
	env := Fail(err.Error())
	parsed := parser.Errors(err)
	if len(parsed) == 0 {
		return env
	}
	list := make([]map[string]any, len(parsed))
	for i, e := range parsed {
		list[i] = map[string]any{
			"code":    string(e.Code),
			"kind":    e.Kind.String(),
			"message": e.Message,
			"offset":  e.Position.Offset,
			"length":  e.Position.Length,
		}
	}
	env["errors"] = list
	return env
}

// compileFailure reports a query that failed to parse or to validate, with
// whichever coded list the failure carries.
func compileFailure(err error) Envelope {
	env := parseFailure(err)
	if list := ValidationErrors(err); len(list) > 0 {
		env["validationErrors"] = list
	}
	return env
}
