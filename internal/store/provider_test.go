package store

import (
	"context"
	"strings"
	"testing"
)

func TestProviderDiagnosticsDoNotExposeConnectionMaterial(t *testing.T) {
	s, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d := s.Diagnostics()
	if d.Provider != SQLite || d.SchemaVersion != CurrentVersion || d.MaxOpen != 1 {
		t.Fatalf("unexpected diagnostics: %#v", d)
	}
}

func TestEveryLogicalMigrationHasPostgresDDL(t *testing.T) {
	if len(postgresMigrations) != CurrentVersion {
		t.Fatalf("got %d postgres migrations", len(postgresMigrations))
	}
	for _, migration := range migrations {
		definition := postgresMigrations[migration.version]
		if strings.TrimSpace(definition) == "" {
			t.Fatalf("migration %d has no postgres DDL", migration.version)
		}
		if strings.Contains(definition, " STRICT") || strings.Contains(definition, " BLOB") || strings.Contains(definition, "AUTOINCREMENT") || strings.Contains(definition, "PRAGMA") {
			t.Fatalf("migration %d contains SQLite-only DDL", migration.version)
		}
	}
}

func TestParseProvider(t *testing.T) {
	for _, value := range []string{"sqlite", "postgres"} {
		if _, err := ParseProvider(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	if _, err := ParseProvider("mysql"); err == nil {
		t.Fatal("accepted unsupported provider")
	}
}
