package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_NoArgs(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run(nil, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := readTemp(t, stderr)
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected usage in stderr, got %q", out)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"bogus"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := readTemp(t, stderr)
	if !strings.Contains(out, "unknown command: bogus") {
		t.Errorf("expected unknown command error, got %q", out)
	}
}

func TestRun_Help(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	out := readTemp(t, stdout)
	if !strings.Contains(out, "explain") {
		t.Errorf("expected 'explain' in help output, got %q", out)
	}
	_ = stderr
}

func TestExplain_MissingExpression(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := readTemp(t, stderr)
	if !strings.Contains(out, "missing query expression") {
		t.Errorf("expected missing expression error, got %q", out)
	}
}

func TestExplain_BinaryExpr(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "status=active AND priority>3"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

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

func TestExplain_SelectorChain(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "items@first"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

	expected := []string{
		"SelectorExpr (@first)",
		"PresenceExpr",
		"Field: items",
	}
	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("tree missing %q, got:\n%s", s, out)
		}
	}
}

func TestExplain_NestedGroups(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "(a=1 OR b=2) AND (c=3 OR d=4)"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

	expected := []string{
		"AndExpr",
		"GroupExpr",
		"OrExpr",
		"Field: a",
		"Field: b",
		"Field: c",
		"Field: d",
	}
	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("tree missing %q, got:\n%s", s, out)
		}
	}
}

func TestExplain_LexerError(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "$invalid"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := readTemp(t, stderr)
	if !strings.Contains(out, "error [") {
		t.Errorf("expected a coded error, got %q", out)
	}
	if !strings.Contains(out, "^") {
		t.Errorf("expected source pointer (^), got %q", out)
	}
}

func TestExplain_ParserError(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "AND foo"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := readTemp(t, stderr)
	if !strings.Contains(out, "error [") {
		t.Errorf("expected a coded error, got %q", out)
	}
	if !strings.Contains(out, "^") {
		t.Errorf("expected source pointer (^), got %q", out)
	}
}

func TestExplain_JSON(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "--json", "status=active"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

	var node map[string]any
	if err := json.Unmarshal([]byte(out), &node); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if node["type"] != "QualifierExpr" {
		t.Errorf("expected QualifierExpr, got %v", node["type"])
	}
	if node["field"] != "status" {
		t.Errorf("expected field 'status', got %v", node["field"])
	}
	if node["value"] != "active" {
		t.Errorf("expected value 'active', got %v", node["value"])
	}
}

func TestExplain_JSONPositions(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "--json", "--positions", "status=active"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

	var node map[string]any
	if err := json.Unmarshal([]byte(out), &node); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if node["position"] == nil {
		t.Fatal("expected position to be set with --positions flag")
	}
}

func TestExplain_Tokens(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "--tokens", "status=active"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

	expected := []string{"IDENT", "status", "=", "STRING", "active", "EOF"}
	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("tokens output missing %q, got:\n%s", s, out)
		}
	}
}

func TestExplain_TokensPositions(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "--tokens", "--positions", "status=active"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)
	// Positions appear as [offset:length]
	if !strings.Contains(out, "[") {
		t.Errorf("expected position brackets with --positions, got:\n%s", out)
	}
}

func TestExplain_Positions(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "--positions", "status=active"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)
	if !strings.Contains(out, "[") {
		t.Errorf("expected position brackets with --positions, got:\n%s", out)
	}
}

func TestExplain_Schema(t *testing.T) {
	// Create a temp schema file
	schema := `{
  "fields": [
    {"name": "status", "type": "text", "allowed_ops": ["=", "!=", "*"], "searchable": true},
    {"name": "priority", "type": "integer", "allowed_ops": ["=", "!=", ">", ">=", "<", "<="], "searchable": false}
  ]
}`
	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(schemaPath, []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("valid query", func(t *testing.T) {
		stdout, stderr := tempFiles(t)
		code := run([]string{"explain", "--schema", schemaPath, "status=active"}, stdout, stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
		}
	})

	t.Run("invalid field", func(t *testing.T) {
		stdout, stderr := tempFiles(t)
		code := run([]string{"explain", "--schema", schemaPath, "unknown=value"}, stdout, stderr)
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d; stdout: %s", code, readTemp(t, stdout))
		}
	})
}

func TestExplain_SchemaInvalidFile(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "--schema", "/nonexistent/schema.json", "status=active"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	out := readTemp(t, stderr)
	if !strings.Contains(out, "reading schema file") {
		t.Errorf("expected schema file error, got %q", out)
	}
}

func TestExplain_NotExpr(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "NOT status=active"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)
	if !strings.Contains(out, "NotExpr") {
		t.Errorf("expected NotExpr in output, got:\n%s", out)
	}
}

func TestExplain_RangeExpr(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "price:10..50"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

	expected := []string{
		"QualifierExpr (..)",
		"Field: price",
		"Value: 10",
		"EndValue: 50",
	}
	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("tree missing %q, got:\n%s", s, out)
		}
	}
}

func TestExplain_OrExpr(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "a=1 OR b=2"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)
	if !strings.Contains(out, "OrExpr") {
		t.Errorf("expected OrExpr in output, got:\n%s", out)
	}
}

func TestExplain_FuncCall(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "contains(tags, urgent)"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)
	if !strings.Contains(out, "FuncCallExpr (contains)") {
		t.Errorf("expected FuncCallExpr in output, got:\n%s", out)
	}
}

func TestExplain_JSONBinaryExpr(t *testing.T) {
	stdout, stderr := tempFiles(t)
	code := run([]string{"explain", "--json", "a=1 AND b=2"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, readTemp(t, stderr))
	}
	out := readTemp(t, stdout)

	var node map[string]any
	if err := json.Unmarshal([]byte(out), &node); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if node["type"] != "BinaryExpr" {
		t.Errorf("expected BinaryExpr, got %v", node["type"])
	}
	if node["op"] != "AND" {
		t.Errorf("expected AND, got %v", node["op"])
	}
	children, ok := node["children"].([]any)
	if !ok || len(children) != 2 {
		t.Errorf("expected 2 children, got %v", node["children"])
	}
}

// tempFiles creates temporary stdout and stderr files for capturing output.
func tempFiles(t *testing.T) (stdout, stderr *os.File) {
	t.Helper()
	dir := t.TempDir()
	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdout.Close() })

	stderr, err = os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stderr.Close() })
	return stdout, stderr
}

// readTemp reads the full contents of a temp file.
func readTemp(t *testing.T, f *os.File) string {
	t.Helper()
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRunExplain_ErrorShowsCodeAndSpan pins the two things a person writing a
// client needs from the CLI: the stable identifier to switch on, and a pointer
// that covers the whole offending token rather than only its first column.
func TestRunExplain_ErrorShowsCodeAndSpan(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantCode  string
		wantPoint string
	}{
		{
			name:      "multi-character token underlines its whole span",
			query:     "items@nope(x=1)",
			wantCode:  "[expectedSelector]",
			wantPoint: "      ^~~~",
		},
		{
			name:      "single-character token gets a bare caret",
			query:     "[unclosed",
			wantCode:  "[unclosedFieldRef]",
			wantPoint: "  ^",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			stdout, stderr := tempFiles(t)

			// Act
			if code := run([]string{"explain", tt.query}, stdout, stderr); code != 1 {
				t.Fatalf("run(explain %q) exit = %d, want 1", tt.query, code)
			}
			out := readTemp(t, stderr)

			// Assert
			if !strings.Contains(out, tt.wantCode) {
				t.Errorf("stderr = %q, want it to contain the code %s", out, tt.wantCode)
			}
			if !strings.Contains(out, tt.wantPoint) {
				t.Errorf("stderr = %q, want it to contain the pointer %q", out, tt.wantPoint)
			}
		})
	}
}
