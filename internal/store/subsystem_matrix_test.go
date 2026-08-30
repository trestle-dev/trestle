package store

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

type subsystemMatrix struct {
	MatrixName string `json:"matrixName"`
	Surfaces   []struct {
		Surface  string
		Status   string
		Evidence []struct {
			Package  string
			Test     string
			Behavior string
		}
	}
}

// TestSubsystemMatrixEvidenceCrossCheck validates the subsystem hardening
// matrix: every proven surface names evidence entries that identify the
// package, an existing exact test function and a behavior distinct from the
// test name. Surfaces marked source-inspected or limitation must not claim
// proven evidence.
func TestSubsystemMatrixEvidenceCrossCheck(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "docs", "hardening", "subsystem-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m subsystemMatrix
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.MatrixName != "trestle-subsystem-hardening" {
		t.Errorf("matrixName = %q", m.MatrixName)
	}
	// Collect exact test names per package.
	names := map[string]bool{}
	entries, _ := os.ReadDir(filepath.Join(root, "internal"))
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(root, "internal", dir.Name(), "*_test.go"))
		for _, file := range files {
			fset := token.NewFileSet()
			parsed, perr := parser.ParseFile(fset, file, nil, 0)
			if perr != nil {
				continue
			}
			for _, decl := range parsed.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.IsExported() {
					names[dir.Name()+"."+fn.Name.Name] = true
				}
			}
		}
	}
	for _, surface := range m.Surfaces {
		switch surface.Status {
		case "proven":
			if len(surface.Evidence) == 0 {
				t.Errorf("surface %q is proven but has no evidence", surface.Surface)
				continue
			}
		case "source-inspected", "limitation":
			if len(surface.Evidence) > 0 {
				t.Errorf("surface %q is %s but lists evidence (should be empty or marked proven)", surface.Surface, surface.Status)
			}
			continue
		default:
			t.Errorf("surface %q has invalid status %q", surface.Surface, surface.Status)
			continue
		}
		for _, e := range surface.Evidence {
			if e.Package == "" || e.Test == "" || e.Behavior == "" {
				t.Errorf("surface %q: evidence entry missing package/test/behavior: %#v", surface.Surface, e)
				continue
			}
			if e.Behavior == e.Test {
				t.Errorf("surface %q: evidence behavior %q repeats the test name", surface.Surface, e.Behavior)
			}
			if !names[e.Package+"."+e.Test] {
				t.Errorf("surface %q: evidence test %s.%s does not exist", surface.Surface, e.Package, e.Test)
			}
		}
	}
}
