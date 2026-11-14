// Package examples_test runs every example program the way the README tells
// a reader to run it, so an example that stops building or starts failing
// breaks the build instead of the reader's first five minutes.
package examples_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cases maps each example directory to the argument lists it is run with.
// An example that takes no arguments has one empty list.
var cases = map[string][][]string{
	"colombia":        {{}},
	"customvalidator": {{}},
	"explain":         {{}},
	"filter":          {{}},
	"functions":       {{}},
	"json":            {{"state=draft AND total>50000"}, {"(labels.dev=jane OR labels.env=bar) AND NOT cluster=demo"}},
	"restrictions":    {{}},
	"rules":           {{}},
	"selector":        {{}},
	"sql": {
		{"state=draft AND total>50000"},
		{"(state=draft OR state=issued) AND name=John*"},
		{"created_at:2026-01-01..2026-03-31"},
		{"created_at>=now()-7d"},
		{"orders@(status=shipped)"},
		{"name!=Jo*"},
		{"lower(name)=john*"},
	},
	"struct": {{}},
	"value":  {{}},
}

// rejected are inputs an example must refuse with a non-zero exit, so the
// allowlists it demonstrates are known to hold.
var rejected = map[string][]string{
	"sql": {"pg_sleep(10)", "nope=1", "secret>a"},
}

func TestExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("examples are run with `go run`, skipped in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		args, known := cases[name]
		if !known {
			t.Errorf("examples/%s has no entry in cases; add how it should be run", name)
			continue
		}
		for _, a := range args {
			t.Run(name+"/"+strings.Join(a, " "), func(t *testing.T) {
				run(t, goBin, name, a, true)
			})
		}
		for _, q := range rejected[name] {
			t.Run(name+"/rejects "+q, func(t *testing.T) {
				run(t, goBin, name, []string{q}, false)
			})
		}
	}
}

func run(t *testing.T, goBin, name string, args []string, wantOK bool) {
	t.Helper()
	cmd := exec.Command(goBin, append([]string{"run", "./" + filepath.Clean(name)}, args...)...)
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if wantOK && err != nil {
		t.Fatalf("go run ./examples/%s %v failed: %v\n%s", name, args, err, out)
	}
	if !wantOK && err == nil {
		t.Fatalf("go run ./examples/%s %v succeeded, want a rejection\n%s", name, args, out)
	}
	if strings.Contains(string(out), "panic:") {
		t.Fatalf("go run ./examples/%s %v panicked:\n%s", name, args, out)
	}
}
