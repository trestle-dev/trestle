package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFreshOpenRestartAndPragmas(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	var version, foreignKeys int
	if err := s.DB().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if version != CurrentVersion || foreignKeys != 1 {
		t.Fatalf("version=%d foreign_keys=%d", version, foreignKeys)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != CurrentVersion {
		t.Fatalf("got %d migration records", count)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("data permissions %o", info.Mode().Perm())
	}
}

func TestRefusesFutureSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trestle.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err = Open(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = s.apply(ctx, migration{2, "broken", "CREATE TABLE should_exist(id INTEGER); THIS IS NOT SQL;"})
	if err == nil {
		t.Fatal("broken migration succeeded")
	}
	var count int
	err = s.DB().QueryRow("SELECT count(*) FROM sqlite_master WHERE name='should_exist'").Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("migration was not rolled back: count=%d err=%v", count, err)
	}
}
