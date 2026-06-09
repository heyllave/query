package query_test

import (
	"errors"
	"testing"

	"github.com/heyllave/query"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

func evalFields() []validate.FieldConfig {
	return []validate.FieldConfig{
		{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
		{Name: "base", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	}
}

func TestMatch(t *testing.T) {
	fields := evalFields()
	rec := map[string]any{"state": "draft", "total": 60000.0}

	t.Run("true predicate", func(t *testing.T) {
		ok, err := query.Match("state=draft AND total>50000", fields, rec)
		if err != nil {
			t.Fatalf("Match: %v", err)
		}
		if !ok {
			t.Error("Match = false, want true")
		}
	})

	t.Run("false predicate", func(t *testing.T) {
		ok, err := query.Match("total>100000", fields, rec)
		if err != nil {
			t.Fatalf("Match: %v", err)
		}
		if ok {
			t.Error("Match = true, want false")
		}
	})

	t.Run("compile error surfaces", func(t *testing.T) {
		if _, err := query.Match("unknown_field=x", fields, rec); err == nil {
			t.Error("Match error = nil, want compile error for unknown field")
		}
	})
}

func TestEval(t *testing.T) {
	fields := evalFields()

	t.Run("value expression", func(t *testing.T) {
		v, err := query.Eval("[base]*2", fields, map[string]any{"base": 21.0})
		if err != nil {
			t.Fatalf("Eval: %v", err)
		}
		if v != 42.0 {
			t.Errorf("Eval([base]*2) = %v, want 42", v)
		}
	})

	t.Run("missing field is ErrNoValue", func(t *testing.T) {
		if _, err := query.Eval("[base]", fields, map[string]any{}); !errors.Is(err, eval.ErrNoValue) {
			t.Errorf("Eval missing field err = %v, want ErrNoValue", err)
		}
	})

	t.Run("compile error surfaces", func(t *testing.T) {
		if _, err := query.Eval("[nope]*2", fields, map[string]any{}); err == nil {
			t.Error("Eval error = nil, want validate error for unknown field")
		}
	})
}
