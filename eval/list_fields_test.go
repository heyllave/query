package eval

import (
	"testing"

	"github.com/heyllave/query/validate"
)

// A query validated against declared list elements evaluates in memory,
// including a list inside a list.
func TestCompile_DeclaredListFields(t *testing.T) {
	fields := []validate.FieldConfig{
		{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
		{Name: "forks", Type: validate.TypeList, AllowedOps: validate.ListOps, Elements: []validate.FieldConfig{
			{Name: "name", Type: validate.TypeText, AllowedOps: validate.TextOps},
			{Name: "replicas", Type: validate.TypeInteger, AllowedOps: validate.NumericOps},
			{Name: "env", Type: validate.TypeList, AllowedOps: validate.ListOps, Elements: []validate.FieldConfig{
				{Name: "key", Type: validate.TypeText, AllowedOps: validate.TextOps},
			}},
		}},
	}
	record := map[string]any{
		"name": "x",
		"forks": []any{
			map[string]any{"name": "a", "replicas": 3, "env": []any{map[string]any{"key": "DEBUG"}}},
			map[string]any{"name": "b", "replicas": 1, "env": []any{}},
		},
	}
	tests := map[string]bool{
		"forks@first":                        true,
		"forks@(name=a AND replicas>2)":      true,
		"forks@all(replicas>2)":              false,
		"forks@none(name=c)":                 true,
		"forks@(name=a AND env@(key=DEBUG))": true,
		"forks@(name=b AND env@(key=DEBUG))": false,
		"forks@(name=b AND NOT env@first)":   true,
	}
	for q, want := range tests {
		// Act
		prog, err := Compile(q, fields)

		// Assert
		if err != nil {
			t.Errorf("%q: %v", q, err)
			continue
		}
		if got := prog.Match(record); got != want {
			t.Errorf("%q: got %v, want %v", q, got, want)
		}
	}

	// A refused selector does not compile.
	if _, err := Compile("forks@last", fields); err == nil {
		t.Error("forks@last compiled against a list that does not allow @last")
	}
}
