package storetest

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/trestle-dev/trestle/internal/store"
)

// OwnershipLock is the advisory lock key that serializes real-PostgreSQL test
// access across parallel package test binaries sharing one disposable database.
const OwnershipLock = 839201347563

// Open opens a disposable store for the named provider. The postgres provider
// requires TRESTLE_TEST_POSTGRES_URL, takes the ownership lock, and resets the
// shared disposable database before and after the test so each case starts
// from a blank schema.
func Open(t *testing.T, provider string) *store.Store {
	t.Helper()
	if provider == string(store.SQLite) {
		s, err := store.Open(context.Background(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	url := PostgresURL(t)
	release := Lock(t, url)
	ResetPostgres(t, url)
	s, err := store.OpenWith(context.Background(), store.Options{DataDir: t.TempDir(), Provider: store.Postgres, URL: url, MaxOpen: 8, MaxIdle: 2, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		ResetPostgres(t, url)
		release()
	})
	return s
}

// Providers returns the supported providers; PostgreSQL is exercised only when
// TRESTLE_TEST_POSTGRES_URL is configured.
func Providers(t *testing.T) []string {
	t.Helper()
	providers := []string{"sqlite"}
	if os.Getenv("TRESTLE_TEST_POSTGRES_URL") != "" {
		providers = append(providers, "postgres")
	}
	return providers
}

func PostgresURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TRESTLE_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("TRESTLE_TEST_POSTGRES_URL is not configured")
	}
	return url
}

// Lock acquires a session-level advisory lock on a dedicated connection held
// until the returned release function runs. This serializes real-PostgreSQL
// tests across package binaries that share the disposable database, so a
// resetting test cannot corrupt a concurrent one.
func Lock(t *testing.T, url string) func() {
	t.Helper()
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := raw.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock(839201347563)"); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(839201347563)")
			conn.Close()
			raw.Close()
		})
	}
}

// ResetPostgres drops every public-schema table in the disposable database so
// a test begins from a blank provider. Names come from the catalog and are
// quoted through the store's strict identifier validator.
func ResetPostgres(t *testing.T, url string) {
	t.Helper()
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	rows, err := raw.Query("SELECT tablename FROM pg_tables WHERE schemaname='public'")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		quoted, err := store.QuoteIdentifier(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec("DROP TABLE " + quoted + " CASCADE"); err != nil {
			t.Fatal(err)
		}
	}
}
