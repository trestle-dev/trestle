package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// MigrationLineageManifest freezes the pre-preview migration lineage so an old
// migration can never be silently redefined after it has been exercised.
type MigrationLineageManifest struct {
	LineageName    string `json:"lineageName"`
	LineageVersion int    `json:"lineageVersion"`
	Description    string `json:"description"`
	Normalization  string `json:"normalization"`
	FinalVersion   int    `json:"finalVersion"`
	Migrations     []struct {
		Version        int    `json:"version"`
		Name           string `json:"name"`
		SQLiteSHA256   string `json:"sqliteSha256"`
		PostgresSHA256 string `json:"postgresSha256"`
	} `json:"migrations"`
}

const migrationLineageFile = "migrations_manifest.json"
const migrationLineageNormalization = "strip CR; split into lines; trim each line; drop blank lines; join with a single newline; append a trailing newline"

// normalizeDDL reduces a DDL string to the canonical form hashed by the
// lineage manifest. It is intentionally whitespace-only normalization so
// formatting edits never look like schema changes while real DDL edits do.
func normalizeDDL(sql string) string {
	sql = strings.ReplaceAll(sql, "\r\n", "\n")
	sql = strings.ReplaceAll(sql, "\r", "\n")
	var lines []string
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// TestMigrationLineageFrozen is the append-only migration lineage gate. It
// requires every retained migration to have both a SQLite and a PostgreSQL DDL
// whose normalized SHA-256 matches the committed manifest exactly, names to
// match, versions to be contiguous 1..CurrentVersion, and CurrentVersion to
// equal the manifest's final version. Editing or removing a retained migration
// changes its digest and fails here; only appending a new migration with the
// manifest regenerated passes.
func TestMigrationLineageFrozen(t *testing.T) {
	raw, err := os.ReadFile(migrationLineageFile)
	if err != nil {
		t.Fatalf("migration lineage manifest is missing: %v", err)
	}
	var manifest MigrationLineageManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("migration lineage manifest is not valid JSON: %v", err)
	}
	if manifest.LineageName != "trestle-migration-lineage" {
		t.Errorf("lineageName = %q", manifest.LineageName)
	}
	if manifest.LineageVersion < 1 {
		t.Errorf("lineageVersion = %d", manifest.LineageVersion)
	}
	if manifest.FinalVersion != CurrentVersion {
		t.Errorf("manifest finalVersion=%d, compiled CurrentVersion=%d", manifest.FinalVersion, CurrentVersion)
	}
	if len(manifest.Migrations) != CurrentVersion {
		t.Fatalf("manifest has %d migrations, want %d", len(manifest.Migrations), CurrentVersion)
	}
	for i, m := range manifest.Migrations {
		wantVersion := i + 1
		if m.Version != wantVersion {
			t.Errorf("manifest migration %d has version %d (must be contiguous)", i, m.Version)
		}
		if m.Name != migrations[i].name {
			t.Errorf("migration %d name=%q, manifest name=%q", wantVersion, migrations[i].name, m.Name)
		}
		if m.SQLiteSHA256 == "" || m.PostgresSHA256 == "" {
			t.Errorf("migration %d must have both SQLite and PostgreSQL digests", wantVersion)
		}
		wantSQLite := sha256Hex(normalizeDDL(migrations[i].sql))
		if m.SQLiteSHA256 != wantSQLite {
			t.Errorf("migration %d (%s) SQLite DDL changed: manifest %s, compiled %s",
				wantVersion, m.Name, m.SQLiteSHA256, wantSQLite)
		}
		pgDDL := postgresMigrations[wantVersion]
		if pgDDL == "" {
			t.Errorf("migration %d has no PostgreSQL DDL", wantVersion)
			continue
		}
		wantPostgres := sha256Hex(normalizeDDL(pgDDL))
		if m.PostgresSHA256 != wantPostgres {
			t.Errorf("migration %d (%s) PostgreSQL DDL changed: manifest %s, compiled %s",
				wantVersion, m.Name, m.PostgresSHA256, wantPostgres)
		}
	}
}

// TestWriteMigrationLineageManifest regenerates the manifest from the compiled
// migrations. It is a generator, not a gate: it writes only when
// TRESTLE_LINEAGE_WRITE=1, so an append of a new migration can update the
// frozen baseline deliberately. The resulting file must be committed and must
// then satisfy TestMigrationLineageFrozen.
func TestWriteMigrationLineageManifest(t *testing.T) {
	if os.Getenv("TRESTLE_LINEAGE_WRITE") != "1" {
		t.Skip("TRESTLE_LINEAGE_WRITE=1 required to regenerate the migration lineage manifest")
	}
	manifest := MigrationLineageManifest{
		LineageName:    "trestle-migration-lineage",
		LineageVersion: 1,
		Description:    "Frozen pre-preview migration lineage. Append-only: retained versions must never be edited; new migrations may only be appended and the manifest regenerated.",
		Normalization:  migrationLineageNormalization,
		FinalVersion:   CurrentVersion,
	}
	for i, m := range migrations {
		entry := struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{Version: i + 1, Name: m.name, SQLiteSHA256: sha256Hex(normalizeDDL(m.sql)), PostgresSHA256: sha256Hex(normalizeDDL(postgresMigrations[i+1]))}
		manifest.Migrations = append(manifest.Migrations, entry)
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(migrationLineageFile, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
