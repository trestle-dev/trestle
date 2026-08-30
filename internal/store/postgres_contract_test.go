package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestPostgresReadinessContract enforces that the machine-readable readiness
// contract matches the compiled behaviour. It always runs (it is a governance
// check). When TRESTLE_TEST_POSTGRES_URL is configured it also probes a real
// PostgreSQL server and asserts the running major version is inside the
// declared supported window, proving the gate exercises a real server rather
// than a mock.
func TestPostgresReadinessContract(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "docs", "postgres", "postgres-readiness.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ContractName         string `json:"contractName"`
		ContractVersion      int    `json:"contractVersion"`
		Checkpoint           string `json:"checkpoint"`
		Campaign             string `json:"campaign"`
		Provider             string `json:"provider"`
		Status               string `json:"status"`
		CurrentSchemaVersion int    `json:"currentSchemaVersion"`
		Driver               string `json:"driver"`
		SupportedVersions    []int  `json:"supportedServerVersions"`
		SchemaOwnership      struct {
			MigrationTable string `json:"migrationTable"`
			SourceOfTruth  string `json:"sourceOfTruth"`
		} `json:"schemaOwnership"`
		Deployment struct {
			Supported   []string `json:"supported"`
			Unsupported []string `json:"unsupported"`
		} `json:"deploymentShapes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("readiness contract is not valid JSON: %v", err)
	}

	if doc.ContractName != "trestle-postgres-readiness" {
		t.Errorf("contractName = %q", doc.ContractName)
	}
	if doc.ContractVersion < 1 {
		t.Errorf("contractVersion = %d, want >= 1", doc.ContractVersion)
	}
	if doc.Checkpoint != "CP1" {
		t.Errorf("checkpoint = %q, want CP1", doc.Checkpoint)
	}
	if doc.Campaign != "public-preview" {
		t.Errorf("campaign = %q, want public-preview", doc.Campaign)
	}
	if doc.Provider != "postgres" {
		t.Errorf("provider = %q, want postgres", doc.Provider)
	}
	if doc.CurrentSchemaVersion != CurrentVersion {
		t.Errorf("currentSchemaVersion = %d, compiled CurrentVersion = %d", doc.CurrentSchemaVersion, CurrentVersion)
	}
	if doc.Driver != "github.com/lib/pq" {
		t.Errorf("driver = %q", doc.Driver)
	}

	if len(doc.SupportedVersions) == 0 {
		t.Error("supportedServerVersions must declare at least one PostgreSQL major")
	}
	seen := map[int]bool{}
	for _, major := range doc.SupportedVersions {
		if major < 10 {
			t.Errorf("supportedServerVersions contains implausible major %d", major)
		}
		seen[major] = true
	}
	for _, want := range []int{16, 17, 18} {
		if !seen[want] {
			t.Errorf("supportedServerVersions missing the declared CI-window major %d", want)
		}
	}

	if doc.SchemaOwnership.MigrationTable != "_trestle_schema_migrations" {
		t.Errorf("schemaOwnership.migrationTable = %q, want _trestle_schema_migrations", doc.SchemaOwnership.MigrationTable)
	}
	if !strings.Contains(doc.SchemaOwnership.SourceOfTruth, "migration history") {
		t.Errorf("schemaOwnership.sourceOfTruth must name validated migration history, got %q", doc.SchemaOwnership.SourceOfTruth)
	}

	if len(doc.Deployment.Supported) == 0 || len(doc.Deployment.Unsupported) == 0 {
		t.Error("deploymentShapes must declare both supported and unsupported shapes")
	}

	// Real-server probe: assert the running server is inside the supported
	// window. This is the "real PostgreSQL, not a mock" part of the gate.
	if url := os.Getenv("TRESTLE_TEST_POSTGRES_URL"); url != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		version, err := Probe(ctx, Postgres, url, 10*time.Second)
		if err != nil {
			t.Fatalf("real PostgreSQL probe failed: %v", err)
		}
		t.Logf("real PostgreSQL server version: %s", version)
		major, err := parseMajorVersion(version)
		if err != nil {
			t.Fatalf("cannot parse server version %q: %v", version, err)
		}
		if !seen[major] {
			t.Errorf("running PostgreSQL major %d is outside the declared supported window %v", major, doc.SupportedVersions)
		}
	}
}

// parseMajorVersion extracts the leading major version from a PostgreSQL
// SHOW server_version value such as "18.6 (Debian 18.6-1.pgdg120+1)".
func parseMajorVersion(value string) (int, error) {
	first := strings.Fields(value)
	if len(first) == 0 {
		return 0, errors.New("empty version string")
	}
	major := strings.Split(first[0], ".")[0]
	n, err := strconv.Atoi(major)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// findRepoRoot walks up from the package directory until it finds go.mod,
// matching the repository layout used by docs/postgres/.
func findRepoRoot() (string, error) {
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
			return "", errors.New("go.mod not found above package directory")
		}
		dir = parent
	}
}
