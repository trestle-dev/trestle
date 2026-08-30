package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parityMatrix mirrors docs/postgres/parity-matrix.json for cross-checking.
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
			Evidence  []string
		}
	}
	UnverifiedOrProviderSpecific []struct {
		Operation string
		SQLite    string `json:"sqlite"`
		Postgres  string `json:"postgres"`
		Evidence  []string
	}
}

// TestParityMatrixEvidenceCrossCheck proves the machine-readable parity
// inventory is backed by executable, provider-parameterized evidence: every
// row marked verified must name at least one test that exists in the codebase
// and whose file iterates storetest.Providers, so parity is never claimed from
// shared handler source alone. The executable gate itself is the
// provider-parameterized suite running on SQLite and real PostgreSQL.
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
	if m.MatrixVersion < 1 {
		t.Errorf("matrixVersion = %d", m.MatrixVersion)
	}
	if m.Checkpoint != "CP9" {
		t.Errorf("checkpoint = %q", m.Checkpoint)
	}
	validStatus := map[string]bool{}
	for _, s := range m.Statuses {
		validStatus[s] = true
	}

	// Collect provider-parameterized test names and their files.
	testFiles := map[string]string{}   // test name -> file path
	providerFiles := map[string]bool{} // file paths that iterate storetest.Providers
	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		files, err := filepath.Glob(filepath.Join(root, "internal", dir.Name(), "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			// Provider-parameterized tests iterate storetest.Providers (handler
			// packages) or are gated on real PostgreSQL via postgresTestURL/
			// ownedURL (the store package, which cannot import storetest).
			if strings.Contains(text, "storetest.Providers") || strings.Contains(text, "postgresTestURL(") || strings.Contains(text, "ownedURL(") {
				providerFiles[file] = true
			}
			for _, line := range strings.Split(text, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "func Test") {
					name := strings.Fields(strings.TrimSpace(line))[1]
					name = strings.SplitN(name, "(", 2)[0]
					testFiles[name] = file
				}
			}
		}
	}

	checkRow := func(kind, operation, sqlite, postgres string, evidence []string) {
		for _, status := range []string{sqlite, postgres} {
			if status != "n/a" && !validStatus[status] {
				t.Errorf("%s %q: invalid status %q", kind, operation, status)
			}
		}
		if sqlite == "verified" || postgres == "verified" {
			if len(evidence) == 0 {
				t.Errorf("%s %q is verified but has no evidence tests", kind, operation)
			}
		}
		for _, name := range evidence {
			file, ok := testFiles[name]
			if !ok {
				t.Errorf("%s %q: evidence test %q does not exist", kind, operation, name)
				continue
			}
			if !providerFiles[file] {
				t.Errorf("%s %q: evidence test %q (%s) is not provider-parameterized (does not iterate storetest.Providers)", kind, operation, name, file)
			}
		}
	}

	count := 0
	for _, area := range m.Areas {
		for _, op := range area.Operations {
			checkRow("area", op.Operation, op.SQLite, op.Postgres, op.Evidence)
			count++
		}
	}
	for _, op := range m.UnverifiedOrProviderSpecific {
		checkRow("unverified", op.Operation, op.SQLite, op.Postgres, op.Evidence)
	}
	if count == 0 {
		t.Fatal("parity matrix has no verified operations")
	}
}
