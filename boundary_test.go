package query_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The lightweight core — the packages a host embeds to parse and validate a
// query. A consumer that imports any of these must never, through the import
// graph, pull in the evaluator, the CLI, the WASM or C bridges, or any
// third-party dependency. These tests pin that boundary so it cannot erode by
// accident: a new import from the core into the heavier surface fails here
// before it reaches a release.
var corePackages = []string{
	"github.com/heyllave/query/token",
	"github.com/heyllave/query/ast",
	"github.com/heyllave/query/parser",
	"github.com/heyllave/query/validate",
	"github.com/heyllave/query/complete",
}

// Packages outside the core. The core importing any of these would drag the
// evaluator, its ~40 functions, the command line, or a build-tagged bridge
// into a host that only wanted to parse.
var nonCorePrefixes = []string{
	"github.com/heyllave/query/eval",
	"github.com/heyllave/query/cmd",
	"github.com/heyllave/query/clients",
	"github.com/heyllave/query/wasm",
	"github.com/heyllave/query/output",
	"github.com/heyllave/query/internal/bridgejson",
	"github.com/heyllave/query/conformance",
}

// TestCore_DoesNotImportTheHeavySurface walks the full transitive import graph
// of every core package and fails if it reaches the evaluator or any other
// non-core package. This is the guarantee behind "import the parser, never get
// the evaluator".
func TestCore_DoesNotImportTheHeavySurface(t *testing.T) {
	for _, pkg := range corePackages {
		pkg := pkg
		t.Run(short(pkg), func(t *testing.T) {
			for _, dep := range deps(t, pkg) {
				for _, bad := range nonCorePrefixes {
					if dep == bad || strings.HasPrefix(dep, bad+"/") {
						t.Errorf("core package %s imports non-core %s through its graph", pkg, dep)
					}
				}
			}
		})
	}
}

// TestCore_HasNoThirdPartyDependency fails if any core package reaches a
// package outside the standard library and this module. The library's whole
// premise is a zero-dependency parser; a host's module graph must stay clean.
func TestCore_HasNoThirdPartyDependency(t *testing.T) {
	for _, pkg := range corePackages {
		pkg := pkg
		t.Run(short(pkg), func(t *testing.T) {
			for _, dep := range deps(t, pkg) {
				if isThirdParty(dep) {
					t.Errorf("core package %s depends on third-party %s", pkg, dep)
				}
			}
		})
	}
}

// TestModule_IsZeroDependency reads go.mod and fails on any require directive.
// The module ships no third-party code; a new dependency has to be a deliberate
// change to this test, not a silent one in go.mod.
func TestModule_IsZeroDependency(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	// Match a require line or the body of a require block. Standard-library
	// only, so a small matcher rather than golang.org/x/mod.
	requireLine := regexp.MustCompile(`(?m)^\s*require\s`)
	if requireLine.Match(data) {
		t.Errorf("go.mod declares a require directive; the module must stay zero-dependency:\n%s", data)
	}
}

// deps returns the transitive dependencies of a package as reported by the go
// tool. It skips the test when the go binary is unavailable (for instance in a
// sandbox without a toolchain) rather than failing.
func deps(t *testing.T, pkg string) []string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go tool not on PATH: %v", err)
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatalf("locate module root: %v", err)
	}
	cmd := exec.Command(goBin, "list", "-deps", pkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

// isThirdParty reports whether an import path is neither standard library nor
// part of this module. The standard library's first path segment has no dot;
// everything else with a dot is an external module.
func isThirdParty(importPath string) bool {
	if strings.HasPrefix(importPath, "github.com/heyllave/query") {
		return false
	}
	first := importPath
	if i := strings.IndexByte(importPath, '/'); i >= 0 {
		first = importPath[:i]
	}
	return strings.Contains(first, ".")
}

// moduleRoot walks up from the test's working directory to the directory that
// holds go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func short(pkg string) string {
	return strings.TrimPrefix(pkg, "github.com/heyllave/query/")
}
