package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/parser"
)

func parse(t *testing.T, q string) ast.Expression {
	t.Helper()
	expr, err := parser.Parse(q, 0)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	return expr
}

func TestAsJSON_QualifierExpr(t *testing.T) {
	expr := parse(t, "status=active")
	data, err := AsJSON(expr)
	if err != nil {
		t.Fatal(err)
	}

	var node jsonNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if node.Type != "QualifierExpr" {
		t.Errorf("expected QualifierExpr, got %s", node.Type)
	}
	if node.Field != "status" {
		t.Errorf("expected field 'status', got %s", node.Field)
	}
	if node.Value != "active" {
		t.Errorf("expected value 'active', got %s", node.Value)
	}
	if node.ValueT != "string" {
		t.Errorf("expected value_type 'string', got %s", node.ValueT)
	}
}

func TestAsJSON_BinaryExpr(t *testing.T) {
	expr := parse(t, "a=1 AND b=2")
	data, err := AsJSON(expr)
	if err != nil {
		t.Fatal(err)
	}

	var node jsonNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if node.Type != "BinaryExpr" {
		t.Errorf("expected BinaryExpr, got %s", node.Type)
	}
	if node.Op != "AND" {
		t.Errorf("expected AND, got %s", node.Op)
	}
	if len(node.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(node.Children))
	}
	if node.Children[0].Field != "a" {
		t.Errorf("expected field 'a', got %s", node.Children[0].Field)
	}
}

func TestAsJSON_WithPositions(t *testing.T) {
	expr := parse(t, "status=active")
	data, err := AsJSON(expr, WithPositions())
	if err != nil {
		t.Fatal(err)
	}

	var node jsonNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if node.Position == nil {
		t.Fatal("expected position to be set with WithPositions()")
	}
}

func TestAsJSON_WithoutPositions(t *testing.T) {
	expr := parse(t, "status=active")
	data, err := AsJSON(expr)
	if err != nil {
		t.Fatal(err)
	}

	var node jsonNode
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if node.Position != nil {
		t.Error("expected no position without WithPositions()")
	}
}

func TestAsTree_BinaryExpr(t *testing.T) {
	expr := parse(t, "status=active AND priority>3")
	data, err := AsTree(expr)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	expected := []string{
		"AndExpr",
		"QualifierExpr (=)",
		"Field: status",
		"Value: active (string)",
		"QualifierExpr (>)",
		"Field: priority",
		"Value: 3 (integer)",
	}
	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("tree missing %q, got:\n%s", s, out)
		}
	}
}

func TestAsTree_NestedGroups(t *testing.T) {
	expr := parse(t, "(a=1 OR b=2) AND (c=3 OR d=4)")
	data, err := AsTree(expr)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	expected := []string{"AndExpr", "GroupExpr", "OrExpr", "Field: a", "Field: d"}
	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("tree missing %q, got:\n%s", s, out)
		}
	}
}

func TestAsTree_SelectorExpr(t *testing.T) {
	expr := parse(t, "items@first")
	data, err := AsTree(expr)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "SelectorExpr (@first)") {
		t.Errorf("expected SelectorExpr (@first), got:\n%s", out)
	}
	if !strings.Contains(out, "PresenceExpr") {
		t.Errorf("expected PresenceExpr, got:\n%s", out)
	}
}

func TestAsTree_NotExpr(t *testing.T) {
	expr := parse(t, "NOT status=active")
	data, err := AsTree(expr)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "NotExpr") {
		t.Errorf("expected NotExpr, got:\n%s", out)
	}
}

func TestAsTree_RangeExpr(t *testing.T) {
	expr := parse(t, "price:10..50")
	data, err := AsTree(expr)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	expected := []string{"QualifierExpr (..)", "Field: price", "Value: 10", "EndValue: 50"}
	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("tree missing %q, got:\n%s", s, out)
		}
	}
}

func TestAsTree_WithPositions(t *testing.T) {
	expr := parse(t, "status=active")
	data, err := AsTree(expr, WithPositions())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "[") {
		t.Errorf("expected position brackets with WithPositions(), got:\n%s", out)
	}
}

func TestAsTree_BoxDrawing(t *testing.T) {
	expr := parse(t, "a=1 AND b=2")
	data, err := AsTree(expr)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	if !strings.Contains(out, "├── ") {
		t.Errorf("expected ├── connector, got:\n%s", out)
	}
	if !strings.Contains(out, "└── ") {
		t.Errorf("expected └── connector, got:\n%s", out)
	}
}

func TestFormat_WritesToWriter(t *testing.T) {
	expr := parse(t, "status=active")
	var buf bytes.Buffer
	if err := Format(&buf, expr, JSONOutput); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("expected output, got empty buffer")
	}

	var node jsonNode
	if err := json.Unmarshal(buf.Bytes(), &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if node.Type != "QualifierExpr" {
		t.Errorf("expected QualifierExpr, got %s", node.Type)
	}
}

func TestFormat_CustomFormatter(t *testing.T) {
	expr := parse(t, "status=active")
	custom := &countFormatter{}

	var buf bytes.Buffer
	if err := Format(&buf, expr, custom); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "1" {
		t.Errorf("expected '1', got %q", buf.String())
	}
}

func TestAsJSON_AllNodeTypes(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{"binary", "a=1 AND b=2", "BinaryExpr"},
		{"unary", "NOT a=1", "UnaryExpr"},
		{"qualifier", "status=active", "QualifierExpr"},
		{"presence", "status", "PresenceExpr"},
		{"group", "(a=1)", "GroupExpr"},
		{"selector", "items@first", "SelectorExpr"},
		{"func_call", "contains(tags, urgent)", "FuncCallExpr"},
		{"or", "a=1 OR b=2", "BinaryExpr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := parse(t, tt.query)
			data, err := AsJSON(expr)
			if err != nil {
				t.Fatal(err)
			}

			var node jsonNode
			if err := json.Unmarshal(data, &node); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if node.Type != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, node.Type)
			}
		})
	}
}

// countFormatter is a test Formatter that writes the node count.
type countFormatter struct{}

func (f *countFormatter) Format(w io.Writer, expr ast.Expression, _ Options) error {
	count := 0
	ast.Walk(expr, func(_ ast.Expression) bool {
		count++
		return true
	})
	_, err := fmt.Fprintf(w, "%d", count)
	return err
}
