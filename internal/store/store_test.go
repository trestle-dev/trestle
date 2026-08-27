package store

import (
	"context"
	"database/sql"
	"fmt"
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

func TestUpgradeFromEveryRetainedSchemaVersion(t *testing.T) {
	for from := 0; from <= CurrentVersion; from++ {
		from := from
		t.Run(fmt.Sprintf("v%d", from), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "trestle.db")
			db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
			if err != nil {
				t.Fatal(err)
			}
			fixture := &Store{db: db, path: path}
			for _, m := range migrations {
				if m.version > from {
					break
				}
				if err := fixture.apply(context.Background(), m); err != nil {
					t.Fatalf("build v%d fixture: %v", from, err)
				}
			}
			if from > 0 {
				if _, err := db.Exec("INSERT INTO _trestle_system_meta(key,value,updated_at) VALUES('upgrade-probe',?,?)", fmt.Sprintf("from-v%d", from), "2026-01-01T00:00:00Z"); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			upgraded, err := Open(context.Background(), dir)
			if err != nil {
				t.Fatalf("upgrade v%d -> v%d: %v", from, CurrentVersion, err)
			}
			defer upgraded.Close()
			var version, migrationsApplied int
			var integrity string
			if err := upgraded.DB().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
				t.Fatal(err)
			}
			if err := upgraded.DB().QueryRow("SELECT count(*) FROM _trestle_schema_migrations").Scan(&migrationsApplied); err != nil {
				t.Fatal(err)
			}
			if err := upgraded.DB().QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
				t.Fatal(err)
			}
			if version != CurrentVersion || migrationsApplied != CurrentVersion || integrity != "ok" {
				t.Fatalf("version=%d migrations=%d integrity=%q", version, migrationsApplied, integrity)
			}
			if from > 0 {
				var probe string
				if err := upgraded.DB().QueryRow("SELECT value FROM _trestle_system_meta WHERE key='upgrade-probe'").Scan(&probe); err != nil || probe != fmt.Sprintf("from-v%d", from) {
					t.Fatalf("preserved probe=%q err=%v", probe, err)
				}
			}
		})
	}
}

func TestInterruptedUpgradePreservesPriorVersionAndData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trestle.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &Store{db: db, path: path}
	for _, m := range migrations[:6] {
		if err := fixture.apply(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO _trestle_system_meta(key,value,updated_at) VALUES('before-failure','preserved','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	err = fixture.apply(context.Background(), migration{7, "injected failure", "CREATE TABLE _must_rollback(id INTEGER); INVALID SQL"})
	if err == nil {
		t.Fatal("injected migration succeeded")
	}
	var version, rolledBack int
	var value string
	db.QueryRow("PRAGMA user_version").Scan(&version)
	db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='_must_rollback'").Scan(&rolledBack)
	db.QueryRow("SELECT value FROM _trestle_system_meta WHERE key='before-failure'").Scan(&value)
	if version != 6 || rolledBack != 0 || value != "preserved" {
		t.Fatalf("version=%d rollback-table=%d value=%q", version, rolledBack, value)
	}
}
