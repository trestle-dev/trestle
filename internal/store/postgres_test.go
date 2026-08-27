package store

import (
	"context"
	"database/sql"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func postgresTestURL(t *testing.T) string {
	t.Helper()
	value := os.Getenv("TRESTLE_TEST_POSTGRES_URL")
	if value == "" {
		t.Skip("TRESTLE_TEST_POSTGRES_URL is not configured")
	}
	return value
}

// ownedURL acquires the test ownership advisory lock before returning the URL
// so tests from parallel package binaries sharing the disposable database
// serialize instead of resetting one another's schema.
func ownedURL(t *testing.T) string {
	t.Helper()
	url := postgresTestURL(t)
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
	t.Cleanup(func() {
		conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(839201347563)")
		conn.Close()
		raw.Close()
	})
	return url
}

func resetPostgres(t *testing.T, raw *sql.DB) {
	t.Helper()
	rows, err := raw.Query(`SELECT tablename FROM pg_tables WHERE schemaname='public'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	rows.Close()
	for _, table := range tables {
		quoted, err := QuoteIdentifier(table)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec("DROP TABLE " + quoted + " CASCADE"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostgresFreshRestartAndFutureSchema(t *testing.T) {
	url := ownedURL(t)
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	resetPostgres(t, raw)
	defer resetPostgres(t, raw)
	options := Options{DataDir: t.TempDir(), Provider: Postgres, URL: url, MaxOpen: 8, MaxIdle: 2}
	s, err := OpenWith(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_schema_migrations").Scan(&count); err != nil || count != CurrentVersion {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if s.Provider() != Postgres || s.Diagnostics().MaxOpen != 8 {
		t.Fatalf("diagnostics=%#v", s.Diagnostics())
	}
	s.Close()
	s, err = OpenWith(context.Background(), options)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	s.Close()
	if _, err := raw.Exec("INSERT INTO _trestle_schema_migrations(version,name,applied_at) VALUES(999,'future','now')"); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWith(context.Background(), options); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("future schema error=%v", err)
	}
}

func TestPostgresFailedMigrationRollsBack(t *testing.T) {
	url := ownedURL(t)
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	resetPostgres(t, raw)
	defer resetPostgres(t, raw)
	s, err := OpenWith(context.Background(), Options{DataDir: t.TempDir(), Provider: Postgres, URL: url})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	postgresMigrations[99] = "CREATE TABLE _must_rollback(id INTEGER); INVALID SQL"
	defer delete(postgresMigrations, 99)
	if err := s.apply(context.Background(), migration{version: 99, name: "injected"}); err == nil {
		t.Fatal("broken migration succeeded")
	}
	var exists bool
	if err := s.DB().QueryRow("SELECT to_regclass('_must_rollback') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}

func TestPostgresConcurrentStartupSerializesMigrations(t *testing.T) {
	url := ownedURL(t)
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	resetPostgres(t, raw)
	defer resetPostgres(t, raw)
	options := Options{DataDir: t.TempDir(), Provider: Postgres, URL: url}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := OpenWith(context.Background(), options)
			if err == nil {
				err = s.Close()
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := raw.QueryRow("SELECT count(*) FROM _trestle_schema_migrations").Scan(&count); err != nil || count != CurrentVersion {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestPostgresDiagnosticsAppliedVersion(t *testing.T) {
	url := ownedURL(t)
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	resetPostgres(t, raw)
	defer resetPostgres(t, raw)
	options := Options{DataDir: t.TempDir(), Provider: Postgres, URL: url, MaxOpen: 8, MaxIdle: 2}
	s, err := OpenWith(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if d := s.Diagnostics(); d.SchemaVersion != CurrentVersion {
		t.Fatalf("fresh schema version %d", d.SchemaVersion)
	}
	s.Close()
	s, err = OpenWith(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if d := s.Diagnostics(); d.SchemaVersion != CurrentVersion {
		t.Fatalf("restart schema version %d", d.SchemaVersion)
	}
	s.Close()
	if _, err := raw.Exec("INSERT INTO _trestle_schema_migrations(version,name,applied_at) VALUES(999,'future','now')"); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWith(context.Background(), options); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("future schema error=%v", err)
	}
}

func TestPostgresMigrationHistoryValidation(t *testing.T) {
	url := ownedURL(t)
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	resetPostgres(t, raw)
	defer resetPostgres(t, raw)
	if _, err := raw.Exec("CREATE TABLE _trestle_schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("INSERT INTO _trestle_schema_migrations(version,name,applied_at) VALUES(1,'system foundation','now'),(2,'administrator identity','now'),(4,'record idempotency','now')"); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWith(context.Background(), Options{DataDir: t.TempDir(), Provider: Postgres, URL: url}); err == nil || !strings.Contains(err.Error(), "not contiguous") {
		t.Fatalf("non-contiguous error=%v", err)
	}
}

func TestWithConnectTimeout(t *testing.T) {
	got, err := withConnectTimeout("postgres://user:secret@db.example:5432/trestle?sslmode=require", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "connect_timeout=3") || !strings.Contains(got, "sslmode=require") || !strings.Contains(got, "user:secret@db.example:5432/trestle") {
		t.Fatalf("rewritten URL=%q", got)
	}
	if strings.Contains(got, "%3A") {
		t.Fatalf("userinfo was escaped instead of preserved: %q", got)
	}
	got, err = withConnectTimeout("postgres://u:p@db/trestle?connect_timeout=120&sslmode=disable", 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "connect_timeout=120") {
		t.Fatalf("user connect_timeout not overridden: %q", got)
	}
	if !strings.Contains(got, "connect_timeout=4") {
		t.Fatalf("trestle connect_timeout missing: %q", got)
	}
	same := "postgres://u:p@db/trestle?sslmode=require"
	got, err = withConnectTimeout(same, 0)
	if err != nil || got != same {
		t.Fatalf("zero timeout rewrote URL to %q err=%v", got, err)
	}
	if _, err := withConnectTimeout(same, 1500*time.Millisecond); err == nil {
		t.Fatal("accepted sub-second timeout")
	}
	if _, err := withConnectTimeout("not a url", time.Second); err == nil {
		t.Fatal("accepted invalid URL")
	}
}

func TestPostgresConnectTimeoutEnforced(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	released := make(chan struct{})
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			close(done)
			return
		}
		accepted <- conn
		<-released
		conn.Close()
		close(done)
	}()
	url := "postgres://trestle_pgtest:redacted@" + ln.Addr().String() + "/nope?sslmode=disable"

	start := time.Now()
	if _, err := Probe(context.Background(), Postgres, url, 2*time.Second); err == nil {
		t.Fatal("probe did not time out")
	}
	elapsed := time.Since(start)
	if elapsed < 1700*time.Millisecond || elapsed > 6*time.Second {
		t.Fatalf("probe returned in %s; expected ~2s bound", elapsed)
	}
	conn := <-accepted
	close(released)
	conn.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stall goroutine did not exit")
	}

	start = time.Now()
	s, err := OpenWith(context.Background(), Options{DataDir: t.TempDir(), Provider: Postgres, URL: url, ConnectTimeout: 2 * time.Second})
	elapsed = time.Since(start)
	if err == nil {
		if s != nil {
			s.Close()
		}
		t.Fatal("startup did not time out")
	}
	if elapsed < 1700*time.Millisecond || elapsed > 6*time.Second {
		t.Fatalf("startup returned in %s; expected ~2s bound", elapsed)
	}
}
