// Example: Custom validation rules via the AstValidator interface.
//
// Demonstrates WithCustomValidator to extend validation with domain-specific
// rules:
//
//   - Per-tenant field access control (tenant A can query "revenue",
//     tenant B cannot — even though the field is declared in the static config).
//   - Cross-field business rules (start_date must be before end_date when
//     both are present).
//   - Value range enforcement (total must be positive).
//
// Run:
//
//	go run ./examples/customvalidator
package main

import (
	"errors"
	"fmt"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/eval"
	"github.com/heyllave/query/validate"
)

var allFields = []validate.FieldConfig{
	{Name: "state", Type: validate.TypeText, AllowedOps: validate.TextOps},
	{Name: "revenue", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "total", Type: validate.TypeDecimal, AllowedOps: validate.NumericOps},
	{Name: "start_date", Type: validate.TypeDate, AllowedOps: validate.DateOps},
	{Name: "end_date", Type: validate.TypeDate, AllowedOps: validate.DateOps},
}

// tenantValidator enforces:
//   - per-tenant field denylists (override semantics via GetFieldConfig)
//   - start_date < end_date when both are present
//   - total must be positive
type tenantValidator struct {
	tenantID string
	fields   map[string]validate.FieldConfig
	denied   map[string]bool
}

func newTenantValidator(tenantID string, denied ...string) *tenantValidator {
	index := make(map[string]validate.FieldConfig, len(allFields))
	for _, f := range allFields {
		index[f.Name] = f
	}
	deniedSet := make(map[string]bool, len(denied))
	for _, d := range denied {
		deniedSet[d] = true
	}
	return &tenantValidator{tenantID: tenantID, fields: index, denied: deniedSet}
}

// GetFieldConfig is authoritative when a custom validator is installed.
// Returning (_, false) causes the field to be treated as unknown — even if
// it is declared in the static config passed to eval.Compile.
func (t *tenantValidator) GetFieldConfig(name string) (validate.FieldConfig, bool) {
	if t.denied[name] {
		return validate.FieldConfig{}, false
	}
	cfg, ok := t.fields[name]
	return cfg, ok
}

// ValidateCustomRules runs once on the root expression after built-in checks.
// Walk the AST with ast.Walk to implement cross-field or value-shape rules.
func (t *tenantValidator) ValidateCustomRules(node ast.Expression) error {
	var start, end *ast.QualifierExpr
	var errs []error

	ast.Walk(node, func(e ast.Expression) bool {
		q, ok := e.(*ast.QualifierExpr)
		if !ok {
			return true
		}
		switch q.Field.String() {
		case "start_date":
			start = q
		case "end_date":
			end = q
		case "total":
			if q.Value.Type == ast.ValueInteger && q.Value.Int < 0 {
				errs = append(errs, fmt.Errorf("total must be positive (got %d)", q.Value.Int))
			}
			if q.Value.Type == ast.ValueFloat && q.Value.Float < 0 {
				errs = append(errs, fmt.Errorf("total must be positive (got %v)", q.Value.Float))
			}
		}
		return true
	})

	if start != nil && end != nil {
		if !start.Value.Date.Before(end.Value.Date) {
			errs = append(errs, fmt.Errorf(
				"start_date (%s) must be before end_date (%s)",
				start.Value.Raw, end.Value.Raw))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func main() {
	fmt.Println("=== Per-tenant field access ===")
	fmt.Println("  Tenant A: full access (no denylist)")
	fmt.Println("  Tenant B: cannot query 'revenue'")
	fmt.Println()

	tenantA := newTenantValidator("A")
	tenantB := newTenantValidator("B", "revenue")

	queries := []string{
		"state=draft",
		"revenue>1000000",
		"state=draft AND revenue>1000000",
	}
	for _, q := range queries {
		_, errA := eval.Compile(q, allFields, eval.WithCustomValidator(tenantA))
		_, errB := eval.Compile(q, allFields, eval.WithCustomValidator(tenantB))
		fmt.Printf("  %-45s  A=%s  B=%s\n", q, status(errA), status(errB))
	}

	fmt.Println()
	fmt.Println("=== Cross-field rule: start_date < end_date ===")
	fmt.Println()

	pairs := []string{
		"start_date>=2026-01-01 AND end_date<=2026-03-31",
		"start_date>=2026-04-01 AND end_date<=2026-03-31",
	}
	for _, q := range pairs {
		_, err := eval.Compile(q, allFields, eval.WithCustomValidator(tenantA))
		fmt.Printf("  %-65s  %s\n", q, status(err))
	}

	fmt.Println()
	fmt.Println("=== Value range rule: total must be positive ===")
	fmt.Println()

	values := []string{
		"total>0",
		"total<-1",
		"state=draft AND total<-100",
	}
	for _, q := range values {
		_, err := eval.Compile(q, allFields, eval.WithCustomValidator(tenantA))
		fmt.Printf("  %-45s  %s\n", q, status(err))
	}
}

func status(err error) string {
	if err == nil {
		return "OK"
	}
	return "BLOCKED: " + err.Error()
}
