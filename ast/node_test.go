package ast

import (
	"testing"
	"time"

	"github.com/heyllave/query/token"
)

func TestNodePos(t *testing.T) {
	pos := token.Position{Offset: 5, Length: 3}

	tests := []struct {
		name string
		node Node
	}{
		{"binary", &BinaryExpr{Position: pos}},
		{"unary", &UnaryExpr{Position: pos}},
		{"qualifier", &QualifierExpr{Position: pos}},
		{"presence", &PresenceExpr{Position: pos}},
		{"group", &GroupExpr{Position: pos}},
		{"selector", &SelectorExpr{Position: pos}},
		{"funcCall", &FuncCallExpr{Position: pos}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.node.Pos() != pos {
				t.Errorf("Pos(): got %v, want %v", tt.node.Pos(), pos)
			}
		})
	}
}

func TestFuncArgString(t *testing.T) {
	fp := FieldPath{"name"}
	v := Value{Raw: "42"}
	inner := &FuncCallExpr{Name: "inner"}

	tests := []struct {
		name string
		arg  FuncArg
		want string
	}{
		{"field", FuncArg{Field: &fp}, "name"},
		{"value", FuncArg{Value: &v}, "42"},
		{"call", FuncArg{Call: inner}, "inner(...)"},
		{"empty", FuncArg{}, "<empty>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.arg.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValueTypeString(t *testing.T) {
	tests := []struct {
		name string
		t    ValueType
		want string
	}{
		{"string", ValueString, "string"},
		{"integer", ValueInteger, "integer"},
		{"float", ValueFloat, "float"},
		{"boolean", ValueBoolean, "boolean"},
		{"date", ValueDate, "date"},
		{"duration", ValueDuration, "duration"},
		{"unknown", ValueType(9999), "ValueType(9999)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValueAnyAllTypes(t *testing.T) {
	tm := time.Now()
	tests := []struct {
		name string
		v    Value
		want any
	}{
		{"string", Value{Type: ValueString, Str: "x"}, "x"},
		{"integer", Value{Type: ValueInteger, Int: 42}, int64(42)},
		{"float", Value{Type: ValueFloat, Float: 3.14}, 3.14},
		{"boolean", Value{Type: ValueBoolean, Bool: true}, true},
		{"date", Value{Type: ValueDate, Date: tm}, tm},
		{"duration", Value{Type: ValueDuration, Duration: time.Hour}, time.Hour},
		{"wildcard", Value{Type: ValueString, Str: "pat*", Wildcard: true}, "pat*"},
		{"default", Value{Raw: "raw", Type: 9999}, "raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.Any(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQualifierHelpers(t *testing.T) {
	tests := []struct {
		name        string
		q           *QualifierExpr
		isRange     bool
		isWildcard  bool
		hasFuncCall bool
	}{
		{
			name:    "plain",
			q:       &QualifierExpr{Field: FieldPath{"x"}},
			isRange: false, isWildcard: false, hasFuncCall: false,
		},
		{
			name:    "range",
			q:       &QualifierExpr{EndValue: &Value{}},
			isRange: true, isWildcard: false, hasFuncCall: false,
		},
		{
			name:       "wildcard",
			q:          &QualifierExpr{Value: Value{Wildcard: true}},
			isWildcard: true,
		},
		{
			name:        "fieldFunc",
			q:           &QualifierExpr{FieldFunc: &FuncCallExpr{}},
			hasFuncCall: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.IsRange(); got != tt.isRange {
				t.Errorf("IsRange: got %v, want %v", got, tt.isRange)
			}
			if got := tt.q.IsWildcard(); got != tt.isWildcard {
				t.Errorf("IsWildcard: got %v, want %v", got, tt.isWildcard)
			}
			if got := tt.q.HasFieldFunc(); got != tt.hasFuncCall {
				t.Errorf("HasFieldFunc: got %v, want %v", got, tt.hasFuncCall)
			}
		})
	}
}
