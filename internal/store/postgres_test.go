package store

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"

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
	url := postgresTestURL(t)
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
	url := postgresTestURL(t)
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
	url := postgresTestURL(t)
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
