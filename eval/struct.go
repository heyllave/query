package eval

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
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
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var configs []validate.FieldConfig
	walkTaggedFields(t, func(name string, _ []int, field reflect.StructField) {
		_, opts := parseTag(field.Tag.Get("query"))
		fvt, ops := goTypeToFieldConfig(field.Type)
		if len(opts) > 0 {
			ops = parseOps(opts)
		}
		configs = append(configs, validate.FieldConfig{
			Name:       name,
			Type:       fvt,
			AllowedOps: ops,
		})
	})
	return configs
}

// walkTaggedFields visits every readable, `query`-tagged field of a struct type,
// including fields promoted through embedded (anonymous) structs. It walks
// breadth-first so a shallower field shadows a deeper one of the same name, as
// Go's own field promotion does, and reports each field's index path for
// [reflect.Value.FieldByIndex]. An unexported field cannot be read through
// reflection, so a tag on one is ignored rather than left to panic at match
// time.
func walkTaggedFields(t reflect.Type, visit func(name string, path []int, f reflect.StructField)) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	type frame struct {
		t      reflect.Type
		prefix []int
	}
	seen := map[string]bool{}
	queue := []frame{{t: t}}
	for len(queue) > 0 {
		fr := queue[0]
		queue = queue[1:]
		for i := range fr.t.NumField() {
			f := fr.t.Field(i)
			path := append(append([]int(nil), fr.prefix...), i)
			if f.Anonymous && f.Tag.Get("query") == "" {
				et := f.Type
				if et.Kind() == reflect.Pointer {
					et = et.Elem()
				}
				// An embedded struct with no tag of its own promotes its fields;
				// time.Time is a struct but is read as a whole, never descended.
				if et.Kind() == reflect.Struct && et != timeType {
					queue = append(queue, frame{t: et, prefix: path})
					continue
				}
			}
			tag := f.Tag.Get("query")
			if tag == "" || tag == "-" || !f.IsExported() {
				continue
			}
			name, _ := parseTag(tag)
			if seen[name] {
				continue
			}
			seen[name] = true
			visit(name, path, f)
		}
	}
}

// tagIndexCache memoizes the query-tag → field-index map per struct type. The
// map depends only on the type, so it is computed once and reused across every
// value of that type — the per-record hot path then does a map lookup and a
// reflect field read, with no tag parsing.
var tagIndexCache sync.Map // map[reflect.Type]map[string][]int

// tagIndex returns the query-tag → field-index-path map for struct type t,
// building and caching it on first use. A path has more than one element when
// the field is promoted through an embedded struct.
func tagIndex(t reflect.Type) map[string][]int {
	if cached, ok := tagIndexCache.Load(t); ok {
		return cached.(map[string][]int)
	}
	m := make(map[string][]int)
	walkTaggedFields(t, func(name string, path []int, _ reflect.StructField) {
		m[name] = path
	})
	actual, _ := tagIndexCache.LoadOrStore(t, m)
	return actual.(map[string][]int)
}

// StructAccessor returns a field accessor function for a struct value,
// resolving field names via `query` tags. The tag→index map is cached per
// struct type, so repeated calls for the same type allocate nothing beyond the
// returned closure. A pointer field is dereferenced; a nil pointer is a
// missing field.
func StructAccessor(v any) func(string) (any, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return func(string) (any, bool) { return nil, false }
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return func(string) (any, bool) { return nil, false }
	}

	tagMap := tagIndex(rv.Type())

	return func(field string) (any, bool) {
		path, ok := tagMap[field]
		if !ok {
			return nil, false
		}
		// FieldByIndexErr returns an error instead of panicking when the path
		// crosses a nil embedded pointer; that field is simply missing.
		fv, err := rv.FieldByIndexErr(path)
		if err != nil {
			return nil, false
		}
		return fieldValue(fv)
	}
}

// fieldValue reads a struct field for comparison, dereferencing pointers so a
// *float64 compares as its float64. A nil pointer or interface is reported
// as a missing field.
func fieldValue(fv reflect.Value) (any, bool) {
	if !fv.IsValid() {
		return nil, false
	}
	for fv.Kind() == reflect.Pointer || fv.Kind() == reflect.Interface {
		if fv.IsNil() {
			return nil, false
		}
		fv = fv.Elem()
	}
	return fv.Interface(), true
}

var timeType = reflect.TypeOf(time.Time{})
var durationType = reflect.TypeOf(time.Duration(0))

func goTypeToFieldConfig(t reflect.Type) (validate.FieldValueType, []validate.Op) {
	// Handle pointer types
	if t.Kind() == reflect.Pointer {
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
