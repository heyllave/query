package eval

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/heyllave/query/validate"
)

// FieldsFromStruct extracts [validate.FieldConfig] from a struct type using
// `query` tags. This enables compile-time type safety — field names and types
// are inferred from the Go struct rather than declared manually.
//
// Tag format: `query:"field_name,ops"` where ops is optional.
//
// Supported Go types → FieldValueType mapping:
//
//	string          → TypeText (TextOps)
//	int, int64, ... → TypeInteger (NumericOps)
//	float32, float64 → TypeDecimal (NumericOps)
//	bool            → TypeBoolean (BoolOps)
//	time.Time       → TypeDate (DateOps)
//	time.Duration   → TypeDuration (DurationOps)
//
// Example:
//
//	type Invoice struct {
//	    State     string    `query:"state"`
//	    Total     float64   `query:"total"`
//	    CreatedAt time.Time `query:"created_at"`
//	    Active    bool      `query:"active"`
//	}
//
//	fields := eval.FieldsFromStruct(Invoice{})
func FieldsFromStruct(v any) []validate.FieldConfig {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var configs []validate.FieldConfig
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("query")
		if tag == "" || tag == "-" {
			continue
		}

		name, opts := parseTag(tag)
		fvt, ops := goTypeToFieldConfig(field.Type)

		if len(opts) > 0 {
			ops = parseOps(opts)
		}

		configs = append(configs, validate.FieldConfig{
			Name:       name,
			Type:       fvt,
			AllowedOps: ops,
		})
	}
	return configs
}

// StructAccessor returns a field accessor function for a struct value,
// resolving field names via `query` tags.
func StructAccessor(v any) func(string) (any, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return func(string) (any, bool) { return nil, false }
	}

	// Build tag→index map
	t := rv.Type()
	tagMap := make(map[string]int, t.NumField())
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("query")
		if tag == "" || tag == "-" {
			continue
		}
		name, _ := parseTag(tag)
		tagMap[name] = i
	}

	return func(field string) (any, bool) {
		idx, ok := tagMap[field]
		if !ok {
			return nil, false
		}
		fv := rv.Field(idx)
		if !fv.IsValid() {
			return nil, false
		}
		return fv.Interface(), true
	}
}

var timeType = reflect.TypeOf(time.Time{})
var durationType = reflect.TypeOf(time.Duration(0))

func goTypeToFieldConfig(t reflect.Type) (validate.FieldValueType, []validate.Op) {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Special types
	if t == timeType {
		return validate.TypeDate, validate.DateOps
	}
	if t == durationType {
		return validate.TypeDuration, validate.DurationOps
	}

	switch t.Kind() {
	case reflect.String:
		return validate.TypeText, validate.TextOps
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return validate.TypeInteger, validate.NumericOps
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return validate.TypeInteger, validate.NumericOps
	case reflect.Float32, reflect.Float64:
		return validate.TypeDecimal, validate.NumericOps
	case reflect.Bool:
		return validate.TypeBoolean, validate.BoolOps
	default:
		return validate.TypeText, validate.TextOps
	}
}

func parseTag(tag string) (string, string) {
	parts := strings.SplitN(tag, ",", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func parseOps(s string) []validate.Op {
	parts := strings.Split(s, "|")
	ops := make([]validate.Op, 0, len(parts))
	for _, p := range parts {
		op := validate.Op(strings.TrimSpace(p))
		if op != "" {
			ops = append(ops, op)
		}
	}
	return ops
}

// CompileFor compiles a query against a Go struct type, inferring field
// configs from `query` struct tags.
//
//	type Invoice struct {
//	    State string    `query:"state"`
//	    Total float64   `query:"total"`
//	}
//
//	prog, err := eval.CompileFor[Invoice]("state=draft AND total>50000")
//	prog.MatchStruct(myInvoice)
func CompileFor[T any](q string, opts ...Option) (*TypedProgram[T], error) {
	var zero T
	fields := FieldsFromStruct(zero)
	if len(fields) == 0 {
		return nil, fmt.Errorf("no query-tagged fields found in %T", zero)
	}

	prog, err := Compile(q, fields, opts...)
	if err != nil {
		return nil, err
	}

	return &TypedProgram[T]{Program: prog}, nil
}

// TypedProgram is a compiled query bound to a specific Go struct type.
type TypedProgram[T any] struct {
	*Program
}

// MatchStruct evaluates the query against a typed struct instance.
func (p *TypedProgram[T]) MatchStruct(v T) bool {
	return p.MatchFunc(StructAccessor(v))
}
