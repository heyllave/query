package eval

import (
	"fmt"
	"reflect"
)

// Limits on a record value the evaluator will render as text. A value past
// either one has no text form and compares as a missing value.
const (
	maxSprintDepth = 32
	maxSprintNodes = 4096
)

// sprint renders a record value as text for comparison. It is fmt.Sprint for
// anything fmt can print in bounded time. A record is host data the library
// does not control, and fmt recurses without limit, so a map or pointer that
// reaches itself would overflow the stack — which no recover can catch — and
// a huge structure would be formatted in full for every record. Such a value
// renders as "" instead.
func sprint(v any) string {
	switch v.(type) {
	case nil:
		return ""
	case string, bool, int, int64, float64, fmt.Stringer, error:
		return fmt.Sprint(v)
	}
	budget := maxSprintNodes
	if !bounded(reflect.ValueOf(v), 0, &budget) {
		return ""
	}
	return fmt.Sprint(v)
}

// bounded reports whether v is a tree fmt can walk within the limits. Depth
// alone catches a cycle, since a cycle has no bottom.
func bounded(v reflect.Value, depth int, budget *int) bool {
	*budget--
	if depth > maxSprintDepth || *budget < 0 {
		return false
	}
	switch v.Kind() { //nolint:exhaustive // scalar kinds have no children
	case reflect.Pointer, reflect.Interface:
		return v.IsNil() || bounded(v.Elem(), depth+1, budget)
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if !bounded(iter.Key(), depth+1, budget) || !bounded(iter.Value(), depth+1, budget) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return true
		}
		for i := 0; i < v.Len(); i++ {
			if !bounded(v.Index(i), depth+1, budget) {
				return false
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !bounded(v.Field(i), depth+1, budget) {
				return false
			}
		}
	}
	return true
}
