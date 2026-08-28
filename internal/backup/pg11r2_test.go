package backup

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
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

func destinationState(t *testing.T, s *store.Store) string {
	t.Helper()
	var b strings.Builder
	for _, table := range portableTables {
		var count int
		s.DB().QueryRow("SELECT count(*) FROM " + quote(table)).Scan(&count)
		b.WriteString(table + "=" + strconv.Itoa(count) + " ")
	}
	rows, err := s.DB().Query("SELECT key FROM _trestle_system_meta ORDER BY key")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		rows.Scan(&key)
		b.WriteString("meta:" + key + " ")
	}
	return b.String()
}

// TestEmptyDestinationRejectsIndependentState seeds each independent durable
// root category into an otherwise initialized destination and proves restore /
// migration refuses it before importing anything, leaving the destination
// unchanged, on both providers.
func TestEmptyDestinationRejectsIndependentState(t *testing.T) {
	minimal := []byte(`{"format":"trestle-portable-v1","schemaVersion":13,"collections":[],"system":{}}`)
	seeds := []struct {
		name string
		stmt string
		args []any
	}{
		{"app user", "INSERT INTO _trestle_app_users(id,email,password_hash,created_at) VALUES(?,?,?,?)", []any{"usr_1", "u@example.com", "h", "2026-01-01T00:00:00Z"}},
		{"credential", "INSERT INTO _trestle_credentials(id,kind,name,secret_hash,scopes,created_at) VALUES(?,?,?,?,?,?)", []any{"cred_1", "service", "s", []byte("YWJj"), "records:read", "2026-01-01T00:00:00Z"}},
		{"event", "INSERT INTO _trestle_events(occurred_at,topic,collection_name,record_id,payload_json) VALUES(?,?,?,?,?)", []any{"2026-01-01T00:00:00Z", "t", "c", "r", "{}"}},
		{"audit", "INSERT INTO _trestle_audit(occurred_at,actor_kind,actor_id,action,target,outcome,request_id,details_json) VALUES(?,?,?,?,?,?,?,?)", []any{"2026-01-01T00:00:00Z", "admin", "a", "x", "t", "ok", "r", "{}"}},
		{"job", "INSERT INTO _trestle_jobs(id,kind,payload_json,status,available_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", []any{"job_1", "noop", "{}", "pending", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{"webhook", "INSERT INTO _trestle_webhooks(id,name,url,topics,secret_cipher,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", []any{"wh_1", "h", "https://x.invalid", "t", []byte("YWJj"), "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{"function", "INSERT INTO _trestle_functions(id,name,provider,target,region,topics,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", []any{"fn_1", "f", "aws-lambda", "arn:x", "us-east-1", "t", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}},
		{"file metadata", "INSERT INTO _trestle_files(id,storage_key,original_name,content_type,size,sha256,created_at) VALUES(?,?,?,?,?,?,?)", []any{"file_1", "k", "n", "t", 1, "abc", "2026-01-01T00:00:00Z"}},
		{"custom system metadata", "INSERT INTO _trestle_system_meta(key,value,updated_at) VALUES(?,?,?)", []any{"custom", "x", "2026-01-01T00:00:00Z"}},
	}
	for _, provider := range storetest.Providers(t) {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			url := ""
			release := func() {}
			if provider == "postgres" {
				url = storetest.PostgresURL(t)
				release = storetest.Lock(t, url)
			}
			defer release()
			for _, seed := range seeds {
				t.Run(seed.name, func(t *testing.T) {
					if provider == "postgres" {
						storetest.ResetPostgres(t, url)
					}
					s := openStoreAt(t, t.TempDir(), provider, url)
					if _, err := s.DB().Exec(seed.stmt, seed.args...); err != nil {
						t.Fatalf("seed: %v", err)
					}
					before := destinationState(t, s)
					err := Import(context.Background(), s.DB(), s.Dialect(), strings.NewReader(string(minimal)))
					after := destinationState(t, s)
					s.Close()
					if err == nil || (!strings.Contains(err.Error(), "not empty") && !strings.Contains(err.Error(), "unexpected system metadata")) {
						t.Fatalf("seed %s error=%v", seed.name, err)
					}
					if before != after {
						t.Fatalf("seed %s changed the destination", seed.name)
					}
				})
			}
			// A valid empty initialized destination still imports.
			if provider == "postgres" {
				storetest.ResetPostgres(t, url)
			}
			s := openStoreAt(t, t.TempDir(), provider, url)
			err := Import(context.Background(), s.DB(), s.Dialect(), strings.NewReader(string(minimal)))
			s.Close()
			if err != nil {
				t.Fatalf("empty destination import: %v", err)
			}
		})
	}
}
