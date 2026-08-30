package store

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parityMatrix mirrors docs/postgres/parity-matrix.json.
type parityMatrix struct {
	MatrixName    string `json:"matrixName"`
	MatrixVersion int    `json:"matrixVersion"`
	Checkpoint    string `json:"checkpoint"`
	Statuses      []string
	Areas         []struct {
		Operations []struct {
			Operation string
			SQLite    string `json:"sqlite"`
			Postgres  string `json:"postgres"`
			Evidence  []struct {
				Package  string `json:"package"`
				Test     string `json:"test"`
				Behavior string `json:"behavior"`
			}
		}
	}
	UnverifiedOrProviderSpecific []struct {
		Operation string
		SQLite    string `json:"sqlite"`
		Postgres  string `json:"postgres"`
		Evidence  []struct {
			Package  string `json:"package"`
			Test     string `json:"test"`
			Behavior string `json:"behavior"`
		}
	}
}

// analyzeTest parses the exact test function in the given package and reports
// whether its own body (not just its file) exercises SQLite and/or PostgreSQL
// via the provider loop (storetest.Providers), an explicit SQLite/Postgres
// path, or a real-PostgreSQL gate (postgresTestURL/ownedURL).
func analyzeTest(t *testing.T, root, pkg, testName string) (coversSQLite, coversPostgres bool, file string, err error) {
	t.Helper()
	files, gerr := filepath.Glob(filepath.Join(root, "internal", pkg, "*_test.go"))
	if gerr != nil {
		return false, false, "", gerr
	}
	for _, f := range files {
		fset := token.NewFileSet()
		parsed, perr := parser.ParseFile(fset, f, nil, 0)
		if perr != nil {
			continue
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != testName {
				continue
			}
			idents := map[string]bool{}
			hasStoretestProviders := false
			hasStoretestPostgresURL := false
			hasStoretestLock := false
			hasStoretestOpen := false
			hasSQLiteLiteral := false
			ast.Inspect(fn, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.Ident:
					idents[v.Name] = true
				case *ast.BasicLit:
					if v.Kind == token.STRING && v.Value == `"sqlite"` {
						hasSQLiteLiteral = true
					}
				case *ast.CallExpr:
					if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
						if x, ok := sel.X.(*ast.Ident); ok && x.Name == "storetest" {
							switch sel.Sel.Name {
							case "Providers":
								hasStoretestProviders = true
							case "PostgresURL":
								hasStoretestPostgresURL = true
							case "Lock":
								hasStoretestLock = true
							case "Open":
								hasStoretestOpen = true
							}
						}
					}
				}
				return true
			})
			hasPgToken := hasStoretestProviders || hasStoretestPostgresURL || hasStoretestLock || idents["postgresTestURL"] || idents["ownedURL"] || idents["Postgres"]
			hasSQLiteToken := hasStoretestProviders || hasStoretestOpen || hasSQLiteLiteral || idents["SQLite"]
			coversSQLite = hasSQLiteToken && hasPgToken
			coversPostgres = hasPgToken
			return coversSQLite, coversPostgres, f, nil
		}
	}
	return false, false, "", os.ErrNotExist
}

// TestParityMatrixEvidenceCrossCheck validates the parity inventory with Go
// AST: every verified row must cite an exact test function whose own body
// exercises the claimed providers, and the package must match the file
// location. File-level heuristics are rejected: a provider-aware file does not
// make an unrelated test evidence.
func TestParityMatrixEvidenceCrossCheck(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "docs", "postgres", "parity-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m parityMatrix
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.MatrixName != "trestle-sqlite-postgres-parity" {
		t.Errorf("matrixName = %q", m.MatrixName)
	}
	validStatus := map[string]bool{}
	for _, s := range m.Statuses {
		validStatus[s] = true
	}

	checkRow := func(operation, sqlite, postgres string, evidence []struct {
		Package  string `json:"package"`
		Test     string `json:"test"`
		Behavior string `json:"behavior"`
	}) {
		for _, status := range []string{sqlite, postgres} {
			if status != "n/a" && !validStatus[status] {
				t.Errorf("%q: invalid status %q", operation, status)
			}
		}
		if len(evidence) == 0 {
			if sqlite == "verified" || postgres == "verified" {
				t.Errorf("%q is verified but has no evidence tests", operation)
			}
			return
		}
		haveSQLite := false
		havePostgres := false
		for _, e := range evidence {
			if e.Package == "" || e.Test == "" || e.Behavior == "" {
				t.Errorf("%q: evidence entry missing package/test/behavior: %#v", operation, e)
				continue
			}
			if e.Behavior == e.Test {
				t.Errorf("%q: evidence behavior %q merely repeats the test name; describe the asserted behavior", operation, e.Behavior)
			}
			covSQLite, covPostgres, file, aerr := analyzeTest(t, root, e.Package, e.Test)
			if aerr != nil {
				t.Errorf("%q: evidence test %s.%s does not exist or could not be parsed: %v", operation, e.Package, e.Test, aerr)
				continue
			}
			if !strings.Contains(file, "/"+e.Package+"/") {
				t.Errorf("%q: evidence package %q does not match file %q", operation, e.Package, file)
			}
			if !covSQLite && !covPostgres {
				t.Errorf("%q: evidence test %s.%s does not itself exercise a provider path (file-level presence is not evidence)", operation, e.Package, e.Test)
			}
			if covSQLite {
				haveSQLite = true
			}
			if covPostgres {
				havePostgres = true
			}
		}
		if sqlite == "verified" && !haveSQLite {
			t.Errorf("%q claims sqlite=verified but no cited test exercises SQLite in its own body", operation)
		}
		if postgres == "verified" && !havePostgres {
			t.Errorf("%q claims postgres=verified but no cited test exercises PostgreSQL in its own body", operation)
		}
	}

	for _, area := range m.Areas {
		for _, op := range area.Operations {
			checkRow(op.Operation, op.SQLite, op.Postgres, op.Evidence)
		}
	}
	for _, op := range m.UnverifiedOrProviderSpecific {
		checkRow(op.Operation, op.SQLite, op.Postgres, op.Evidence)
	}
}
