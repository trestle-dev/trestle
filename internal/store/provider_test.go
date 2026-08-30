package store

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestProviderDiagnosticsDoNotExposeConnectionMaterial(t *testing.T) {
	// SQLite leg.
	sqlite, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	if d := sqlite.Diagnostics(); d.Provider != SQLite || d.SchemaVersion != CurrentVersion || d.MaxOpen != 1 {
		t.Fatalf("sqlite diagnostics: %#v", d)
	}
	// Real PostgreSQL leg (skips without TRESTLE_TEST_POSTGRES_URL).
	if os.Getenv("TRESTLE_TEST_POSTGRES_URL") != "" {
		url := ownedURL(t)
		raw, openErr := sql.Open("postgres", url)
		if openErr != nil {
			t.Fatal(openErr)
		}
		resetPostgres(t, raw)
		raw.Close()
		pg, openErr := OpenWith(context.Background(), Options{DataDir: t.TempDir(), Provider: Postgres, URL: url, MaxOpen: 8, MaxIdle: 2, ConnectTimeout: 5 * time.Second})
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer pg.Close()
		if d := pg.Diagnostics(); d.Provider != Postgres || d.SchemaVersion != CurrentVersion || d.MaxOpen != 8 {
			t.Fatalf("postgres diagnostics: %#v", d)
		}
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
