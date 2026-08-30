package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
// append-only updater passes.
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

// appendOnlyUpdate validates an existing manifest against the compiled
// migrations and returns it with entries appended only for versions after the
// existing finalVersion. It refuses to write anything when a retained version,
// name or digest differs, when the manifest is truncated, gapped, reordered,
// inconsistent or already ahead of the compiled version, or when the manifest
// metadata (lineage version, normalization contract) is unsupported. The
// manifest metadata is validated before the no-op branch, and a no-op is only
// possible for a complete, internally consistent manifest.
func appendOnlyUpdate(existing MigrationLineageManifest, current []migration, pg map[int]string, currentVersion int) (MigrationLineageManifest, error) {
	if existing.LineageName != "trestle-migration-lineage" {
		return existing, errors.New("manifest is not the trestle migration lineage")
	}
	if existing.LineageVersion != 1 {
		return existing, fmt.Errorf("unsupported manifest lineage version %d", existing.LineageVersion)
	}
	if existing.Normalization != migrationLineageNormalization {
		return existing, errors.New("manifest normalization contract changed")
	}
	if len(existing.Migrations) != existing.FinalVersion {
		return existing, fmt.Errorf("manifest has %d migrations but finalVersion %d (truncated or inconsistent)", len(existing.Migrations), existing.FinalVersion)
	}
	if len(existing.Migrations) > len(current) {
		return existing, fmt.Errorf("manifest has %d migrations, compiled has %d (truncation of compiled history)", len(existing.Migrations), len(current))
	}
	if len(existing.Migrations) > 0 && existing.Migrations[existing.FinalVersion-1].Version != existing.FinalVersion {
		return existing, fmt.Errorf("manifest finalVersion %d does not match its last entry version %d", existing.FinalVersion, existing.Migrations[existing.FinalVersion-1].Version)
	}
	for i, entry := range existing.Migrations {
		wantVersion := i + 1
		if entry.Version != wantVersion {
			return existing, fmt.Errorf("manifest entry %d has version %d (gaps, truncation or reordering)", i, entry.Version)
		}
		compiled := current[i]
		if entry.Name != compiled.name {
			return existing, fmt.Errorf("migration %d name changed from %q to %q; retained lineage is immutable", wantVersion, entry.Name, compiled.name)
		}
		if entry.SQLiteSHA256 != sha256Hex(normalizeDDL(compiled.sql)) {
			return existing, fmt.Errorf("migration %d SQLite DDL changed; retained lineage is immutable", wantVersion)
		}
		pgDDL := pg[wantVersion]
		if pgDDL == "" {
			return existing, fmt.Errorf("migration %d has no PostgreSQL DDL", wantVersion)
		}
		if entry.PostgresSHA256 != sha256Hex(normalizeDDL(pgDDL)) {
			return existing, fmt.Errorf("migration %d PostgreSQL DDL changed; retained lineage is immutable", wantVersion)
		}
	}
	if existing.FinalVersion == currentVersion {
		return existing, nil // genuine no-op only after full metadata and entry validation
	}
	if existing.FinalVersion > currentVersion {
		return existing, fmt.Errorf("manifest finalVersion %d is ahead of compiled CurrentVersion %d", existing.FinalVersion, currentVersion)
	}
	for v := existing.FinalVersion + 1; v <= currentVersion; v++ {
		pgDDL := pg[v]
		if pgDDL == "" {
			return existing, fmt.Errorf("migration %d has no PostgreSQL DDL", v)
		}
		entry := struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{Version: v, Name: current[v-1].name, SQLiteSHA256: sha256Hex(normalizeDDL(current[v-1].sql)), PostgresSHA256: sha256Hex(normalizeDDL(pgDDL))}
		existing.Migrations = append(existing.Migrations, entry)
	}
	existing.FinalVersion = currentVersion
	return existing, nil
}

// TestUpdateMigrationLineageManifest is the append-only manifest updater. It
// reads the committed manifest, validates every retained entry against the
// compiled migrations, appends entries only for versions after the existing
// finalVersion, and writes nothing when validation fails or no new migration
// exists. Gated by TRESTLE_LINEAGE_WRITE=1.
func TestUpdateMigrationLineageManifest(t *testing.T) {
	if os.Getenv("TRESTLE_LINEAGE_WRITE") != "1" {
		t.Skip("TRESTLE_LINEAGE_WRITE=1 required to update the migration lineage manifest")
	}
	raw, err := os.ReadFile(migrationLineageFile)
	if err != nil {
		t.Fatal(err)
	}
	var existing MigrationLineageManifest
	if err := json.Unmarshal(raw, &existing); err != nil {
		t.Fatal(err)
	}
	updated, err := appendOnlyUpdate(existing, migrations, postgresMigrations, CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Migrations) == len(existing.Migrations) {
		t.Log("no new migration: manifest left unchanged")
		return
	}
	encoded, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(migrationLineageFile, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAppendOnlyUpdateRefusesHistoricalChanges proves the updater cannot be
// used to bless a rewritten historical migration: changed SQLite or PostgreSQL
// DDL, renamed migrations, removal/reordering and truncation all fail, while
// appending one new migration appends exactly one entry and preserves every
// existing entry field-for-field.
func TestAppendOnlyUpdateRefusesHistoricalChanges(t *testing.T) {
	mkMig := func(version int, name, sql string) migration {
		return migration{version: version, name: name, sql: sql}
	}
	current := []migration{
		mkMig(1, "alpha", "CREATE TABLE alpha"),
		mkMig(2, "beta", "CREATE TABLE beta"),
		mkMig(3, "gamma", "CREATE TABLE gamma"),
	}
	pg := map[int]string{1: "CREATE TABLE alpha", 2: "CREATE TABLE beta", 3: "CREATE TABLE gamma"}
	build := func() MigrationLineageManifest {
		m := MigrationLineageManifest{
			LineageName:    "trestle-migration-lineage",
			LineageVersion: 1,
			Normalization:  migrationLineageNormalization,
			FinalVersion:   3,
		}
		for i, c := range current {
			entry := struct {
				Version        int    `json:"version"`
				Name           string `json:"name"`
				SQLiteSHA256   string `json:"sqliteSha256"`
				PostgresSHA256 string `json:"postgresSha256"`
			}{Version: i + 1, Name: c.name, SQLiteSHA256: sha256Hex(normalizeDDL(c.sql)), PostgresSHA256: sha256Hex(normalizeDDL(pg[i+1]))}
			m.Migrations = append(m.Migrations, entry)
		}
		return m
	}
	existing := build()

	cases := []struct {
		name    string
		mutate  func() ([]migration, map[int]string, int)
		wantErr string
	}{
		{"changed historical SQLite DDL", func() ([]migration, map[int]string, int) {
			changed := append([]migration(nil), current...)
			changed[0] = mkMig(1, "alpha", "CREATE TABLE alpha_modified")
			return changed, pg, 3
		}, "SQLite DDL changed"},
		{"changed historical PostgreSQL DDL", func() ([]migration, map[int]string, int) {
			pgChanged := map[int]string{1: "CREATE TABLE alpha_modified", 2: "CREATE TABLE beta", 3: "CREATE TABLE gamma"}
			return current, pgChanged, 3
		}, "PostgreSQL DDL changed"},
		{"renamed historical migration", func() ([]migration, map[int]string, int) {
			renamed := append([]migration(nil), current...)
			renamed[1] = mkMig(2, "betaRENAMED", "CREATE TABLE beta")
			return renamed, pg, 3
		}, "name changed"},
		{"removed a migration (gap)", func() ([]migration, map[int]string, int) {
			return []migration{mkMig(1, "alpha", "CREATE TABLE alpha"), mkMig(3, "gamma", "CREATE TABLE gamma")}, pg, 3
		}, "truncation"},
		{"reordered migrations", func() ([]migration, map[int]string, int) {
			return []migration{mkMig(1, "gamma", "CREATE TABLE gamma"), mkMig(2, "alpha", "CREATE TABLE alpha"), mkMig(3, "beta", "CREATE TABLE beta")}, pg, 3
		}, "immutable"},
		{"truncated compiled history", func() ([]migration, map[int]string, int) {
			return []migration{mkMig(1, "alpha", "CREATE TABLE alpha"), mkMig(2, "beta", "CREATE TABLE beta")}, pg, 3
		}, "truncation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			migrationsArg, pgArg, versionArg := tc.mutate()
			if _, err := appendOnlyUpdate(existing, migrationsArg, pgArg, versionArg); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err=%v, want containing %q", err, tc.wantErr)
			}
		})
	}

	// Appending one new migration appends exactly one entry and preserves every
	// existing entry field-for-field.
	extended := append(append([]migration(nil), current...), mkMig(4, "delta", "CREATE TABLE delta"))
	pgExtended := map[int]string{1: "CREATE TABLE alpha", 2: "CREATE TABLE beta", 3: "CREATE TABLE gamma", 4: "CREATE TABLE delta"}
	updated, err := appendOnlyUpdate(existing, extended, pgExtended, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Migrations) != 4 || updated.FinalVersion != 4 {
		t.Fatalf("append produced %d migrations finalVersion=%d, want 4/4", len(updated.Migrations), updated.FinalVersion)
	}
	for i, entry := range existing.Migrations {
		if updated.Migrations[i] != entry {
			t.Fatalf("existing entry %d changed during append", i+1)
		}
	}
	if last := updated.Migrations[3]; last.Version != 4 || last.Name != "delta" || last.SQLiteSHA256 == "" || last.PostgresSHA256 == "" {
		t.Fatalf("appended entry=%#v", last)
	}

	// Running the updater with no new migration is a no-op.
	same, err := appendOnlyUpdate(existing, current, pg, 3)
	if err != nil {
		t.Fatal(err)
	}
	if same.FinalVersion != 3 || len(same.Migrations) != 3 {
		t.Fatal("no-op run changed the manifest")
	}
	for i := range existing.Migrations {
		if same.Migrations[i] != existing.Migrations[i] {
			t.Fatal("no-op run changed an existing entry")
		}
	}
}

// TestAppendOnlyUpdateManifestEdgeCases closes the truncated-manifest edge
// case: the updater must validate manifest metadata and internal consistency
// before the no-op branch, refuse inconsistent manifests, and handle the empty
// manifest without indexing past it.
func TestAppendOnlyUpdateManifestEdgeCases(t *testing.T) {
	mkMig := func(version int, name, sql string) migration {
		return migration{version: version, name: name, sql: sql}
	}
	current := []migration{
		mkMig(1, "alpha", "CREATE TABLE alpha"),
		mkMig(2, "beta", "CREATE TABLE beta"),
		mkMig(3, "gamma", "CREATE TABLE gamma"),
	}
	pg := map[int]string{1: "CREATE TABLE alpha", 2: "CREATE TABLE beta", 3: "CREATE TABLE gamma"}
	entry := func(version int, name string) struct {
		Version        int    `json:"version"`
		Name           string `json:"name"`
		SQLiteSHA256   string `json:"sqliteSha256"`
		PostgresSHA256 string `json:"postgresSha256"`
	} {
		return struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{Version: version, Name: name, SQLiteSHA256: sha256Hex(normalizeDDL(current[version-1].sql)), PostgresSHA256: sha256Hex(normalizeDDL(pg[version]))}
	}
	build := func(entries []struct {
		Version        int    `json:"version"`
		Name           string `json:"name"`
		SQLiteSHA256   string `json:"sqliteSha256"`
		PostgresSHA256 string `json:"postgresSha256"`
	}, finalVersion int) MigrationLineageManifest {
		return MigrationLineageManifest{
			LineageName:    "trestle-migration-lineage",
			LineageVersion: 1,
			Normalization:  migrationLineageNormalization,
			FinalVersion:   finalVersion,
			Migrations:     entries,
		}
	}

	t.Run("missing final entry with finalVersion==CurrentVersion is not a no-op", func(t *testing.T) {
		m := build([]struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{entry(1, "alpha"), entry(2, "beta")}, 3)
		if _, err := appendOnlyUpdate(m, current, pg, 3); err == nil || !strings.Contains(err.Error(), "truncated or inconsistent") {
			t.Fatalf("err=%v, want truncated-manifest refusal", err)
		}
	})

	t.Run("finalVersion below the last entry", func(t *testing.T) {
		m := build([]struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{entry(1, "alpha"), entry(2, "beta"), entry(3, "gamma")}, 2)
		if _, err := appendOnlyUpdate(m, current, pg, 3); err == nil || !strings.Contains(err.Error(), "truncated or inconsistent") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("finalVersion above the last entry", func(t *testing.T) {
		m := build([]struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{entry(1, "alpha"), entry(2, "beta"), entry(3, "gamma")}, 4)
		if _, err := appendOnlyUpdate(m, current, pg, 4); err == nil || !strings.Contains(err.Error(), "truncated or inconsistent") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("empty manifest appends from scratch", func(t *testing.T) {
		m := build(nil, 0)
		updated, err := appendOnlyUpdate(m, current, pg, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(updated.Migrations) != 3 || updated.FinalVersion != 3 {
			t.Fatalf("empty manifest append produced %d entries finalVersion=%d", len(updated.Migrations), updated.FinalVersion)
		}
	})

	t.Run("empty manifest with nonzero finalVersion is refused", func(t *testing.T) {
		m := build(nil, 3)
		if _, err := appendOnlyUpdate(m, current, pg, 3); err == nil || !strings.Contains(err.Error(), "truncated or inconsistent") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("unsupported lineage version", func(t *testing.T) {
		m := build([]struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{entry(1, "alpha"), entry(2, "beta"), entry(3, "gamma")}, 3)
		m.LineageVersion = 2
		if _, err := appendOnlyUpdate(m, current, pg, 3); err == nil || !strings.Contains(err.Error(), "unsupported manifest lineage version") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("changed normalization metadata", func(t *testing.T) {
		m := build([]struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{entry(1, "alpha"), entry(2, "beta"), entry(3, "gamma")}, 3)
		m.Normalization = "different"
		if _, err := appendOnlyUpdate(m, current, pg, 3); err == nil || !strings.Contains(err.Error(), "normalization contract changed") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("genuine no-op with a complete valid manifest", func(t *testing.T) {
		m := build([]struct {
			Version        int    `json:"version"`
			Name           string `json:"name"`
			SQLiteSHA256   string `json:"sqliteSha256"`
			PostgresSHA256 string `json:"postgresSha256"`
		}{entry(1, "alpha"), entry(2, "beta"), entry(3, "gamma")}, 3)
		updated, err := appendOnlyUpdate(m, current, pg, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(updated.Migrations) != 3 || updated.FinalVersion != 3 {
			t.Fatal("no-op changed the manifest")
		}
	})
}
