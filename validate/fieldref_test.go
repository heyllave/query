package validate

import (
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

func fieldRefFields() []FieldConfig {
	return []FieldConfig{
		{Name: "total", Type: TypeDecimal, AllowedOps: NumericOps},
		{Name: "base", Type: TypeDecimal, AllowedOps: NumericOps},
		{Name: "cpu", Type: TypeInteger, AllowedOps: NumericOps},
		{Name: "cpu_limit", Type: TypeInteger, AllowedOps: NumericOps},
		{Name: "name", Type: TypeText, AllowedOps: TextOps},
	}
}

// qualifierWithFieldRef builds `field >= [ref]`.
func qualifierWithFieldRef(field, ref string) ast.Expression {
	return &ast.QualifierExpr{
		Field:    ast.FieldPath{field},
		Operator: token.Gte,
		Value:    ast.Value{Type: ast.ValueFieldRef, Field: ast.FieldPath{ref}},
	}
}

func TestValidate_FieldRefOperand(t *testing.T) {
	v := New(fieldRefFields())

	t.Run("declared operand field passes", func(t *testing.T) {
		expr := qualifierWithFieldRef("total", "base")
		if err := v.Validate(expr); err != nil {
			t.Errorf("validate declared field ref: %v", err)
		}
	})

	t.Run("unknown operand field fails", func(t *testing.T) {
		expr := qualifierWithFieldRef("total", "nope")
		if err := v.Validate(expr); err == nil {
			t.Error("validate unknown field ref: error = nil, want ErrFieldNotFound")
		}
	})

	t.Run("field ref operand skips strict type check", func(t *testing.T) {
		// name is text; [name] > number-ish op must still validate (dynamic
		// stance — type agreement is deferred to eval).
		expr := &ast.QualifierExpr{
			Field:    ast.FieldPath{"cpu"},
			Operator: token.Gte,
			Value:    ast.Value{Type: ast.ValueFieldRef, Field: ast.FieldPath{"name"}},
		}
		if err := v.Validate(expr); err != nil {
			t.Errorf("validate cross-type field ref: %v (want nil — dynamic)", err)
		}
	})
}

func TestValidate_FieldRefInArithmetic(t *testing.T) {
	v := New(fieldRefFields())
	// total >= [base] * 1.1
	expr := &ast.QualifierExpr{
		Field:    ast.FieldPath{"total"},
		Operator: token.Gte,
		Value: ast.Value{Type: ast.ValueArith, Arith: &ast.ArithExpr{
			Op:    ast.ArithMul,
			Left:  &ast.Value{Type: ast.ValueFieldRef, Field: ast.FieldPath{"base"}},
			Right: &ast.Value{Type: ast.ValueFloat, Float: 1.1},
		}},
	}
	if err := v.Validate(expr); err != nil {
		t.Errorf("validate [base]*1.1: %v", err)
	}

	// unknown operand inside arithmetic fails
	bad := &ast.QualifierExpr{
		Field:    ast.FieldPath{"total"},
		Operator: token.Gte,
		Value: ast.Value{Type: ast.ValueArith, Arith: &ast.ArithExpr{
			Op:    ast.ArithMul,
			Left:  &ast.Value{Type: ast.ValueFieldRef, Field: ast.FieldPath{"missing"}},
			Right: &ast.Value{Type: ast.ValueFloat, Float: 1.1},
		}},
	}
	if err := v.Validate(bad); err == nil {
		t.Error("validate [missing]*1.1: error = nil, want ErrFieldNotFound")
	}
}
