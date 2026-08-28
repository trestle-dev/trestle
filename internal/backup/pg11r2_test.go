package backup

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/storetest"
)

func cloneBundle(b PortableBundle) PortableBundle {
	data, _ := json.Marshal(b)
	var out PortableBundle
	json.Unmarshal(data, &out)
	return out
}

func marshalBundle(b PortableBundle) []byte {
	data, _ := json.Marshal(b)
	return data
}

func TestSemanticDigestSensitivity(t *testing.T) {
	var bundle PortableBundle
	json.Unmarshal([]byte(buildPortableFixture(t, "sqlite")), &bundle)
	// Ensure the bundle also contains an application session so its identity is
	// covered by the canonical digest.
	bundle.System.AppSessions = append(bundle.System.AppSessions, map[string]any{
		"id": "aps_1", "user_id": "usr_1", "refresh_hash": "YWJj", "created_at": "2026-01-01T00:00:00Z", "expires_at": "2027-01-01T00:00:00Z",
	})
	base, err := SemanticDigest(marshalBundle(bundle))
	if err != nil {
		t.Fatal(err)
	}

	// Mutating a webhook target identity must change the digest.
	wh := cloneBundle(bundle)
	wh.System.Webhooks[0]["url"] = "https://other.invalid/hook"
	if digest, _ := SemanticDigest(marshalBundle(wh)); digest == base {
		t.Fatal("webhook URL mutation did not change digest")
	}
	wh = cloneBundle(bundle)
	wh.System.Webhooks[0]["topics"] = "other.topic"
	if digest, _ := SemanticDigest(marshalBundle(wh)); digest == base {
		t.Fatal("webhook topics mutation did not change digest")
	}
	// Mutating administrator session identity/owner must change the digest.
	as := cloneBundle(bundle)
	as.System.AdminSessions[0]["admin_id"] = "adm_other"
	if digest, _ := SemanticDigest(marshalBundle(as)); digest == base {
		t.Fatal("admin session owner mutation did not change digest")
	}
	as = cloneBundle(bundle)
	as.System.AdminSessions[0]["id"] = "ses_other"
	if digest, _ := SemanticDigest(marshalBundle(as)); digest == base {
		t.Fatal("admin session identity mutation did not change digest")
	}
	// Mutating application session identity/owner must change the digest.
	app := cloneBundle(bundle)
	app.System.AppSessions[0]["user_id"] = "usr_other"
	if digest, _ := SemanticDigest(marshalBundle(app)); digest == base {
		t.Fatal("app session owner mutation did not change digest")
	}
	app = cloneBundle(bundle)
	app.System.AppSessions[0]["id"] = "aps_other"
	if digest, _ := SemanticDigest(marshalBundle(app)); digest == base {
		t.Fatal("app session identity mutation did not change digest")
	}
	// Changing only the fields deliberately normalized by the restore policy must
	// NOT change the digest.
	pol := cloneBundle(bundle)
	pol.System.Webhooks[0]["secret_cipher"] = "YWJjY2RlZmc="
	pol.System.Webhooks[0]["enabled"] = 1
	pol.System.AdminSessions[0]["revoked_at"] = "2026-02-02T00:00:00Z"
	pol.System.AppSessions[0]["revoked_at"] = "2026-02-02T00:00:00Z"
	if digest, _ := SemanticDigest(marshalBundle(pol)); digest != base {
		t.Fatal("normalized policy fields changed the digest")
	}
}

func TestPortableTimestampsPreserved(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		provider := provider
		var portable string
		t.Run("export-"+provider, func(t *testing.T) {
			portable = buildPortableFixture(t, provider)
		})
		t.Run(provider, func(t *testing.T) {
			var bundle PortableBundle
			if err := json.Unmarshal([]byte(portable), &bundle); err != nil {
				t.Fatal(err)
			}
			if len(bundle.Collections) != 1 {
				t.Fatalf("collections=%d", len(bundle.Collections))
			}
			col := bundle.Collections[0]
			if col.CreatedAt == "" || col.UpdatedAt == "" {
				t.Fatalf("collection timestamps missing: %+v", col)
			}
			if len(col.Fields) == 0 || col.Fields[0].CreatedAt == "" {
				t.Fatalf("field timestamps missing: %+v", col.Fields)
			}
			s := storetest.Open(t, provider)
			if err := Import(context.Background(), s.DB(), s.Dialect(), strings.NewReader(portable)); err != nil {
				t.Fatal(err)
			}
			var createdAt, updatedAt, fieldCreatedAt string
			if err := s.DB().QueryRow("SELECT created_at, updated_at FROM _trestle_collections WHERE name='issues'").Scan(&createdAt, &updatedAt); err != nil {
				t.Fatal(err)
			}
			if createdAt != col.CreatedAt || updatedAt != col.UpdatedAt {
				t.Fatalf("collection timestamps changed: got %s/%s want %s/%s", createdAt, updatedAt, col.CreatedAt, col.UpdatedAt)
			}
			if err := s.DB().QueryRow("SELECT created_at FROM _trestle_fields WHERE collection_id=(SELECT id FROM _trestle_collections WHERE name='issues') ORDER BY position LIMIT 1").Scan(&fieldCreatedAt); err != nil {
				t.Fatal(err)
			}
			if fieldCreatedAt != col.Fields[0].CreatedAt {
				t.Fatalf("field timestamp changed: got %s want %s", fieldCreatedAt, col.Fields[0].CreatedAt)
			}
		})
	}
}

func historyMutationSource(t *testing.T, provider, url, dir string, mutate func(db store.Executor)) string {
	t.Helper()
	if provider == "postgres" {
		storetest.ResetPostgres(t, url)
	}
	dir = filepath.Join(dir, provider)
	s := openStoreAt(t, dir, provider, url)
	populatePortableFixture(t, s)
	mutate(s.DB())
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMigrationHistoryValidationReadOnly(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			url := ""
			release := func() {}
			if provider == "postgres" {
				url = storetest.PostgresURL(t)
				release = storetest.Lock(t, url)
				storetest.ResetPostgres(t, url)
			}
			defer release()
			// Correct history: read-only open succeeds.
			dir := historyMutationSource(t, provider, url, t.TempDir(), func(db store.Executor) {})
			_, _, closeFn, err := openSource(context.Background(), MigrateOptions{SourceProvider: store.Provider(provider), SourceDir: dir, SourceURL: url})
			if err != nil {
				if closeFn != nil {
					closeFn()
				}
				t.Fatalf("valid history rejected: %v", err)
			}
			if closeFn != nil {
				closeFn()
			}
			// Gap: version 5 removed.
			dir = historyMutationSource(t, provider, url, t.TempDir(), func(db store.Executor) {
				db.Exec("DELETE FROM _trestle_schema_migrations WHERE version=5")
			})
			_, _, closeFn, err = openSource(context.Background(), MigrateOptions{SourceProvider: store.Provider(provider), SourceDir: dir, SourceURL: url})
			if err == nil || !strings.Contains(err.Error(), "not contiguous") {
				if closeFn != nil {
					closeFn()
				}
				t.Fatalf("gapped history error=%v", err)
			}
			if closeFn != nil {
				closeFn()
			}
			// Future version 999.
			dir = historyMutationSource(t, provider, url, t.TempDir(), func(db store.Executor) {
				db.Exec("INSERT INTO _trestle_schema_migrations(version,name,applied_at) VALUES(999,'future','now')")
			})
			_, _, closeFn, err = openSource(context.Background(), MigrateOptions{SourceProvider: store.Provider(provider), SourceDir: dir, SourceURL: url})
			if err == nil || !strings.Contains(err.Error(), "newer") {
				if closeFn != nil {
					closeFn()
				}
				t.Fatalf("future history error=%v", err)
			}
			if closeFn != nil {
				closeFn()
			}
			// Wrong name for a known version.
			dir = historyMutationSource(t, provider, url, t.TempDir(), func(db store.Executor) {
				db.Exec("UPDATE _trestle_schema_migrations SET name='wrong name' WHERE version=5")
			})
			_, _, closeFn, err = openSource(context.Background(), MigrateOptions{SourceProvider: store.Provider(provider), SourceDir: dir, SourceURL: url})
			if err == nil || !strings.Contains(err.Error(), "unexpected name") {
				if closeFn != nil {
					closeFn()
				}
				t.Fatalf("wrong-name history error=%v", err)
			}
			if closeFn != nil {
				closeFn()
			}
		})
	}
}

// TestPostgresMigrationSourceNonMutation proves a PostgreSQL migration source is
// never written: a read-only re-export of the source after a dry run and after
// a real migration yields the identical canonical content.
func TestPostgresMigrationSourceNonMutation(t *testing.T) {
	url := storetest.PostgresURL(t)
	release := storetest.Lock(t, url)
	defer release()
	storetest.ResetPostgres(t, url)
	dir := t.TempDir()
	s := openStoreAt(t, dir, "postgres", url)
	populatePortableFixture(t, s)
	before := sourceDigestOf(t, dir, url)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	if _, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: "postgres", SourceDir: dir, SourceURL: url,
		TargetProvider: "sqlite", TargetDir: targetDir, TargetURL: "",
		DryRun: true,
	}); err != nil {
		t.Fatal(err)
	}
	after := sourceDigestOf(t, dir, url)
	if after != before {
		t.Fatal("dry run wrote to the PostgreSQL source")
	}
	targetDir = t.TempDir()
	if _, err := Migrate(context.Background(), MigrateOptions{
		SourceProvider: "postgres", SourceDir: dir, SourceURL: url,
		TargetProvider: "sqlite", TargetDir: targetDir, TargetURL: "",
		Confirm: true,
	}); err != nil {
		t.Fatal(err)
	}
	after = sourceDigestOf(t, dir, url)
	if after != before {
		t.Fatal("real migration wrote to the PostgreSQL source")
	}
}

func sourceDigestOf(t *testing.T, dir, url string) string {
	t.Helper()
	source, dialect, closeFn, err := openSource(context.Background(), MigrateOptions{SourceProvider: "postgres", SourceDir: dir, SourceURL: url})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	var buf strings.Builder
	if err := Export(context.Background(), source, dialect, &buf); err != nil {
		t.Fatal(err)
	}
	digest, err := SemanticDigest([]byte(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
