package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/storetest"
)

func openStoreAt(t *testing.T, dir, provider, url string) *store.Store {
	t.Helper()
	if provider == "sqlite" {
		s, err := store.Open(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s, err := store.OpenWith(context.Background(), store.Options{DataDir: dir, Provider: store.Postgres, URL: url, MaxOpen: 4, MaxIdle: 1, ConnectTimeout: 10 * 1e9})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func verifyMigrated(t *testing.T, provider, dir, url string) {
	t.Helper()
	s := openStoreAt(t, dir, provider, url)
	defer s.Close()
	var admins, collectionsCount int
	if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_admins").Scan(&admins); err != nil || admins != 1 {
		t.Fatalf("%s admins=%d err=%v", provider, admins, err)
	}
	if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_collections").Scan(&collectionsCount); err != nil || collectionsCount != 1 {
		t.Fatalf("%s collections=%d err=%v", provider, collectionsCount, err)
	}
	var probe string
	if err := s.DB().QueryRow("SELECT value FROM _trestle_system_meta WHERE key='restore-probe'").Scan(&probe); err != nil || probe != "preserved" {
		t.Fatalf("%s probe=%q err=%v", provider, probe, err)
	}
}

func TestCrossProviderMigrationRoundTrip(t *testing.T) {
	url := storetest.PostgresURL(t)
	release := storetest.Lock(t, url)
	defer release()

	for _, direction := range [][2]string{{"sqlite", "postgres"}, {"postgres", "sqlite"}} {
		src, dst := direction[0], direction[1]
		t.Run(src+"->"+dst, func(t *testing.T) {
			// Reset the shared PostgreSQL database first so it starts empty
			// whether it is the source or the target of this direction.
			storetest.ResetPostgres(t, url)
			// Build the source fixture.
			sourceDir := t.TempDir()
			sourceStore := openStoreAt(t, sourceDir, src, url)
			populatePortableFixture(t, sourceStore)
			if err := sourceStore.Close(); err != nil {
				t.Fatal(err)
			}

			targetDir := t.TempDir()
			report, err := Migrate(context.Background(), MigrateOptions{
				SourceProvider: store.Provider(src), SourceDir: sourceDir, SourceURL: url,
				TargetProvider: store.Provider(dst), TargetDir: targetDir, TargetURL: url,
				DryRun: true, Confirm: false,
			})
			if err != nil {
				t.Fatalf("dry run %s->%s: %v", src, dst, err)
			}
			if report.WritesPerformed || report.Collections != 1 || report.Records != 1 {
				t.Fatalf("dry run report=%+v", report)
			}
			// A dry run must not have touched the target.
			if dst == "postgres" {
				var admins int
				probe := openStoreAt(t, t.TempDir(), "postgres", url)
				probe.DB().QueryRow("SELECT count(*) FROM _trestle_admins").Scan(&admins)
				probe.Close()
				if admins != 0 {
					t.Fatalf("dry run wrote to postgres: admins=%d", admins)
				}
			}

			report, err = Migrate(context.Background(), MigrateOptions{
				SourceProvider: store.Provider(src), SourceDir: sourceDir, SourceURL: url,
				TargetProvider: store.Provider(dst), TargetDir: targetDir, TargetURL: url,
				Confirm: true,
			})
			if err != nil {
				t.Fatalf("migrate %s->%s: %v", src, dst, err)
			}
			if !report.WritesPerformed || report.Checksum == "" || report.Collections != 1 {
				t.Fatalf("report=%+v", report)
			}
			verifyMigrated(t, dst, targetDir, url)
		})
	}
}

func migrateChain(t *testing.T, url, first, second string) {
	t.Helper()
	storetest.ResetPostgres(t, url)
	sourceDir := t.TempDir()
	firstStore := openStoreAt(t, sourceDir, first, url)
	populatePortableFixture(t, firstStore)
	firstStore.Close()
	middleDir := t.TempDir()
	if _, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: store.Provider(first), SourceDir: sourceDir, SourceURL: url,
		TargetProvider: store.Provider(second), TargetDir: middleDir, TargetURL: url,
		Confirm: true,
	}); err != nil {
		t.Fatalf("%s->%s: %v", first, second, err)
	}
	verifyMigrated(t, second, middleDir, url)
	finalDir := t.TempDir()
	// The second hop targets the original first provider. PostgreSQL must be
	// empty again only when it is that target; when it is the second hop's
	// source it must retain the migrated data.
	if first == "postgres" {
		storetest.ResetPostgres(t, url)
	}
	if _, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: store.Provider(second), SourceDir: middleDir, SourceURL: url,
		TargetProvider: store.Provider(first), TargetDir: finalDir, TargetURL: url,
		Confirm: true,
	}); err != nil {
		t.Fatalf("%s->%s: %v", second, first, err)
	}
	verifyMigrated(t, first, finalDir, url)
}

func TestChainedMigrationRoundTrips(t *testing.T) {
	url := storetest.PostgresURL(t)
	release := storetest.Lock(t, url)
	defer release()
	t.Run("sqlite-postgres-sqlite", func(t *testing.T) { migrateChain(t, url, "sqlite", "postgres") })
	t.Run("postgres-sqlite-postgres", func(t *testing.T) { migrateChain(t, url, "postgres", "sqlite") })
}

func TestMigrationRequiresExplicitConfirmation(t *testing.T) {
	url := storetest.PostgresURL(t)
	release := storetest.Lock(t, url)
	defer release()
	storetest.ResetPostgres(t, url)
	sourceDir := t.TempDir()
	sourceStore := openStoreAt(t, sourceDir, "sqlite", url)
	populatePortableFixture(t, sourceStore)
	sourceStore.Close()
	// A real run without confirmation must fail before touching the target.
	_, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: "sqlite", SourceDir: sourceDir,
		TargetProvider: "postgres", TargetURL: url, TargetDir: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("unconfirmed migration error=%v", err)
	}
	var admins int
	probe := openStoreAt(t, t.TempDir(), "postgres", url)
	probe.DB().QueryRow("SELECT count(*) FROM _trestle_admins").Scan(&admins)
	probe.Close()
	if admins != 0 {
		t.Fatalf("unconfirmed migration wrote to target: admins=%d", admins)
	}
}

func TestMigrationSourceRemainsUnchanged(t *testing.T) {
	url := storetest.PostgresURL(t)
	release := storetest.Lock(t, url)
	defer release()
	storetest.ResetPostgres(t, url)
	sourceDir := t.TempDir()
	sourceStore := openStoreAt(t, sourceDir, "sqlite", url)
	populatePortableFixture(t, sourceStore)
	sourceStore.Close()
	sourcePath := filepath.Join(sourceDir, "trestle.db")
	before, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	// Dry run and a real run must both leave the source byte-identical.
	if _, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: "sqlite", SourceDir: sourceDir,
		TargetProvider: "postgres", TargetURL: url, TargetDir: t.TempDir(),
		DryRun: true,
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("dry run modified the source database")
	}
	storetest.ResetPostgres(t, url)
	if _, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: "sqlite", SourceDir: sourceDir,
		TargetProvider: "postgres", TargetURL: url, TargetDir: t.TempDir(),
		Confirm: true,
	}); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("real migration modified the source database")
	}
}

func TestStaleSourceRejectedWithoutWrites(t *testing.T) {
	// A sqlite source at a non-current schema version must be refused without
	// the migration writing to it.
	sourceDir := t.TempDir()
	sourceStore := openStoreAt(t, sourceDir, "sqlite", "")
	populatePortableFixture(t, sourceStore)
	// Downgrade the version marker so the source is stale.
	sourceStore.DB().Exec("PRAGMA user_version = 12")
	sourceStore.DB().Exec("DELETE FROM _trestle_schema_migrations WHERE version=13")
	sourceStore.Close()
	sourcePath := filepath.Join(sourceDir, "trestle.db")
	before, _ := os.ReadFile(sourcePath)
	_, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: "sqlite", SourceDir: sourceDir,
		TargetProvider: "postgres", TargetURL: storetest.PostgresURL(t), TargetDir: t.TempDir(),
		Confirm: true,
	})
	if err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatalf("stale source error=%v", err)
	}
	after, _ := os.ReadFile(sourcePath)
	if !bytes.Equal(before, after) {
		t.Fatal("stale-source refusal wrote to the source")
	}
}
