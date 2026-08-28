package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/collections"
	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/storetest"
)

func adminCookieCSRF(t *testing.T, admin *adminauth.Handler) (*httptest.ResponseRecorder, string) {
	t.Helper()
	body := strings.NewReader(`{"email":"admin@example.com","password":"1234567"}`)
	r := httptest.NewRequest("POST", "http://example.test/admin/v1/setup", body)
	r.Host = "example.test"
	r.Header.Set("Origin", "http://example.test")
	w := httptest.NewRecorder()
	admin.ServeHTTP(w, r)
	var out struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return w, out.CSRF
}

func populatePortableFixture(t *testing.T, s *store.Store) {
	t.Helper()
	provider := string(s.Provider())
	admin := adminauth.New(s.DB(), provider)
	setup, csrf := adminCookieCSRF(t, admin)

	schemas := collections.New(s.DB(), admin)
	payload, _ := json.Marshal(map[string]any{"name": "issues", "fields": []map[string]any{
		{"name": "title", "type": "text", "required": true},
		{"name": "done", "type": "boolean"},
		{"name": "score", "type": "number"},
		{"name": "meta", "type": "json"},
	}})
	cr := httptest.NewRequest("POST", "http://example.test/admin/v1/collections", bytes.NewReader(payload))
	cr.Host = "example.test"
	cr.Header.Set("Origin", "http://example.test")
	cr.Header.Set("X-Trestle-CSRF", csrf)
	cr.AddCookie(setup.Result().Cookies()[0])
	w := httptest.NewRecorder()
	schemas.ServeHTTP(w, cr)
	if w.Code != 201 {
		t.Fatalf("collection fixture %d %s", w.Code, w.Body.String())
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.DB().Exec("INSERT INTO _trestle_system_meta(key,value,updated_at) VALUES(?,?,?)", "restore-probe", "preserved", now); err != nil {
		t.Fatal(err)
	}
	blob := []byte("hash-material")
	inserts := []struct {
		stmt string
		args []any
	}{
		{"INSERT INTO _trestle_app_users(id,email,password_hash,created_at) VALUES(?,?,?,?)", []any{"usr_1", "user@example.com", "$argon2id$dummy", now}},
		{"INSERT INTO _trestle_credentials(id,kind,name,secret_hash,scopes,created_at) VALUES(?,?,?,?,?,?)", []any{"cred_1", "service", "svc", blob, "records:read", now}},
		{"INSERT INTO _trestle_collection_rules(collection_id,operation,expression,updated_at) VALUES((SELECT id FROM _trestle_collections WHERE name='issues'),?,?,?)", []any{"view", "actor.id == record.owner", now}},
		{"INSERT INTO _trestle_events(occurred_at,topic,collection_name,record_id,payload_json) VALUES(?,?,?,?,?)", []any{now, "record.created", "issues", "rec_1", `{"n":1}`}},
		{"INSERT INTO _trestle_audit(occurred_at,actor_kind,actor_id,action,target,outcome,request_id,details_json) VALUES(?,?,?,?,?,?,?,?)", []any{now, "admin", "adm_x", "credential.use", "x", "allowed", "req", "{}"}},
		{"INSERT INTO _trestle_jobs(id,kind,payload_json,status,available_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", []any{"job_1", "noop", "{}", "pending", now, now, now}},
		{"INSERT INTO _trestle_webhooks(id,name,url,topics,secret_cipher,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", []any{"wh_1", "hook", "https://example.invalid/hook", "record.created", blob, now, now}},
		{"INSERT INTO _trestle_functions(id,name,provider,target,region,topics,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", []any{"fn_1", "fn", "aws-lambda", "arn:x", "us-east-1", "record.created", now, now}},
		{"INSERT INTO _trestle_files(id,storage_key,original_name,content_type,size,sha256,created_at) VALUES(?,?,?,?,?,?,?)", []any{"file_1", "key/1", "a.txt", "text/plain", 3, "abc", now}},
	}
	for _, item := range inserts {
		if _, err := s.DB().Exec(item.stmt, item.args...); err != nil {
			t.Fatalf("%s: %v", item.stmt[:60], err)
		}
	}

	var colID string
	if err := s.DB().QueryRow("SELECT id FROM _trestle_collections WHERE name='issues'").Scan(&colID); err != nil {
		t.Fatal(err)
	}
	var fieldIDs []string
	rows, err := s.DB().Query("SELECT id FROM _trestle_fields WHERE collection_id=? ORDER BY position", colID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		fieldIDs = append(fieldIDs, id)
	}
	rows.Close()

	table := `"` + collections.PhysicalTableName(colID) + `"`
	cols := []string{`_id`, `_version`, `_created`, `_updated`}
	marks := []string{"?", "?", "?", "?"}
	args := []any{"rec_1", 1, now, now}
	fieldValues := []any{"hello", s.Dialect().Boolean(true), 3, `{"n":1}`}
	for i, fid := range fieldIDs {
		cols = append(cols, `"`+collections.PhysicalColumnName(fid)+`"`)
		marks = append(marks, "?")
		args = append(args, fieldValues[i])
	}
	if _, err := s.DB().Exec("INSERT INTO "+table+" ("+strings.Join(cols, ",")+") VALUES ("+strings.Join(marks, ",")+")", args...); err != nil {
		t.Fatal(err)
	}

}

func buildPortableFixture(t *testing.T, provider string) string {
	t.Helper()
	s := storetest.Open(t, provider)
	populatePortableFixture(t, s)
	var buf bytes.Buffer
	if err := Export(context.Background(), s.DB(), s.Dialect(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestPortableRoundTripAcrossProviders(t *testing.T) {
	for _, src := range storetest.Providers(t) {
		src := src
		var portable string
		t.Run("export-"+src, func(t *testing.T) {
			portable = buildPortableFixture(t, src)
		})
		for _, dst := range storetest.Providers(t) {
			dst := dst
			t.Run(src+"->"+dst, func(t *testing.T) {
				s := storetest.Open(t, dst)
				if err := Import(context.Background(), s.DB(), s.Dialect(), strings.NewReader(portable)); err != nil {
					t.Fatal(err)
				}
				var admins, users, credentials, rules, events, audit, jobs, webhooks, functions, files int
				for name, dstCount := range map[string]*int{
					"_trestle_admins": &admins, "_trestle_app_users": &users, "_trestle_credentials": &credentials,
					"_trestle_collection_rules": &rules, "_trestle_events": &events, "_trestle_audit": &audit,
					"_trestle_jobs": &jobs, "_trestle_webhooks": &webhooks, "_trestle_functions": &functions, "_trestle_files": &files,
				} {
					if err := s.DB().QueryRow("SELECT count(*) FROM " + name).Scan(dstCount); err != nil {
						t.Fatal(err)
					}
				}
				if admins != 1 || users != 1 || credentials != 1 || rules != 1 || events != 1 || audit != 1 || jobs != 1 || webhooks != 1 || functions != 1 || files != 1 {
					t.Fatalf("counts admins=%d users=%d creds=%d rules=%d events=%d audit=%d jobs=%d wh=%d fn=%d files=%d", admins, users, credentials, rules, events, audit, jobs, webhooks, functions, files)
				}
				var collectionsCount int
				if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_collections").Scan(&collectionsCount); err != nil || collectionsCount != 1 {
					t.Fatalf("collections=%d err=%v", collectionsCount, err)
				}
				var colID string
				if err := s.DB().QueryRow("SELECT id FROM _trestle_collections WHERE name='issues'").Scan(&colID); err != nil {
					t.Fatal(err)
				}
				var fieldIDs []string
				var fieldNames []string
				fieldRows, err := s.DB().Query("SELECT id,name FROM _trestle_fields WHERE collection_id=? ORDER BY position", colID)
				if err != nil {
					t.Fatal(err)
				}
				for fieldRows.Next() {
					var id, name string
					fieldRows.Scan(&id, &name)
					fieldIDs = append(fieldIDs, id)
					fieldNames = append(fieldNames, name)
				}
				fieldRows.Close()
				selectParts := []string{`_id`}
				for _, fid := range fieldIDs {
					selectParts = append(selectParts, `"`+collections.PhysicalColumnName(fid)+`"`)
				}
				raw := make([]any, len(fieldIDs))
				dest := make([]any, len(fieldIDs))
				for i := range raw {
					dest[i] = &raw[i]
				}
				var recID string
				if err := s.DB().QueryRow("SELECT " + strings.Join(selectParts, ",") + " FROM " + `"` + collections.PhysicalTableName(colID) + `"` + " WHERE _id='rec_1'").Scan(append([]any{&recID}, dest...)...); err != nil {
					t.Fatal(err)
				}
				if recID != "rec_1" {
					t.Fatalf("record id %q", recID)
				}
				for i, name := range fieldNames {
					if name == "done" {
						if b, ok := decodeAny(s.Dialect(), raw[i]).(bool); !ok || !b {
							t.Fatalf("boolean round trip got %v", raw[i])
						}
					}
					if name == "meta" {
						if v, ok := raw[i].(string); !ok || !strings.Contains(v, `"n"`) {
							t.Fatalf("json round trip got %v", raw[i])
						}
					}
				}
				_ = store.CurrentVersion
			})
		}
	}
}

func decodeAny(dialect store.Dialect, v any) any {
	if b, err := dialect.DecodeBoolean(v); err == nil {
		return b
	}
	return v
}

func TestRestoreLogicalArchive(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		portable := buildPortableFixture(t, provider)
		t.Run(provider, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "logical.zip")
			var buffer bytes.Buffer
			zw := zip.NewWriter(&buffer)
			mw, _ := zw.Create("manifest.json")
			json.NewEncoder(mw).Encode(Manifest{Format: "trestle-backup-v1", SchemaVersion: 13, IncludesDatabase: false})
			pw, _ := zw.Create("portable.json")
			pw.Write([]byte(portable))
			zw.Close()
			os.WriteFile(archive, buffer.Bytes(), 0o600)
			target := filepath.Join(t.TempDir(), "restored")
			if err := Restore(context.Background(), archive, target); err != nil {
				t.Fatal(err)
			}
			s, err := store.Open(context.Background(), target)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var probe string
			if err := s.DB().QueryRow("SELECT value FROM _trestle_system_meta WHERE key='restore-probe'").Scan(&probe); err != nil || probe != "preserved" {
				t.Fatalf("probe=%q err=%v", probe, err)
			}
			var collectionsCount int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_collections").Scan(&collectionsCount); err != nil || collectionsCount != 1 {
				t.Fatalf("collections=%d err=%v", collectionsCount, err)
			}
			var admins int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_admins").Scan(&admins); err != nil || admins != 1 {
				t.Fatalf("admins=%d err=%v", admins, err)
			}
		})
	}
}

func TestExportSnapshotConsistency(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			populatePortableFixture(t, s)
			tx, err := beginSnapshot(context.Background(), s.DB())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			var before int
			if err := tx.QueryRow("SELECT count(*) FROM _trestle_admins").Scan(&before); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				s.DB().Exec("INSERT INTO _trestle_admins(id,email,password_hash,created_at) VALUES('adm_extra','extra@example.com','h','2026-01-01T00:00:00Z')")
			}()
			time.Sleep(100 * time.Millisecond)
			var after int
			if err := tx.QueryRow("SELECT count(*) FROM _trestle_admins").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatalf("snapshot inconsistent: before=%d after=%d", before, after)
			}
			tx.Rollback()
			<-done
		})
	}
}

func TestRestorePolicyRevokesSessionsAndIntegrationSecrets(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		provider := provider
		var portable string
		t.Run("export-"+provider, func(t *testing.T) {
			portable = buildPortableFixture(t, provider)
		})
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			if err := Import(context.Background(), s.DB(), s.Dialect(), strings.NewReader(portable)); err != nil {
				t.Fatal(err)
			}
			var enabled bool
			var cipher []byte
			if err := s.DB().QueryRow("SELECT enabled, secret_cipher FROM _trestle_webhooks WHERE id='wh_1'").Scan(&enabled, &cipher); err != nil {
				t.Fatal(err)
			}
			if enabled || len(cipher) != 0 {
				t.Fatalf("webhook not disabled/cleared: enabled=%v cipherLen=%d", enabled, len(cipher))
			}
			var revoked int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_admin_sessions WHERE revoked_at IS NULL").Scan(&revoked); err != nil || revoked != 0 {
				t.Fatalf("admin sessions not revoked: %d err=%v", revoked, err)
			}
			var appAccess int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_app_access").Scan(&appAccess); err != nil || appAccess != 0 {
				t.Fatalf("app access not cleared: %d err=%v", appAccess, err)
			}
			var credentialHashes int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_credentials WHERE secret_hash IS NOT NULL").Scan(&credentialHashes); err != nil || credentialHashes != 1 {
				t.Fatalf("credential hashes lost: %d err=%v", credentialHashes, err)
			}
		})
	}
}

func TestRestorePortableArchiveToPostgres(t *testing.T) {
	url := storetest.PostgresURL(t)
	release := storetest.Lock(t, url)
	defer release()
	storetest.ResetPostgres(t, url)
	fixtureDir := t.TempDir()
	fixtureStore := openStoreAt(t, fixtureDir, "postgres", url)
	populatePortableFixture(t, fixtureStore)
	var portable bytes.Buffer
	if err := Export(context.Background(), fixtureStore.DB(), fixtureStore.Dialect(), &portable); err != nil {
		t.Fatal(err)
	}
	fixtureStore.Close()
	storetest.ResetPostgres(t, url)
	t.Run("restore-to-postgres", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "pg-backup.zip")
		var buffer bytes.Buffer
		zw := zip.NewWriter(&buffer)
		mw, _ := zw.Create("manifest.json")
		json.NewEncoder(mw).Encode(Manifest{Format: "trestle-backup-v1", SchemaVersion: 13, IncludesDatabase: false})
		pw, _ := zw.Create("portable.json")
		pw.Write(portable.Bytes())
		zw.Close()
		os.WriteFile(archive, buffer.Bytes(), 0o600)
		storetest.ResetPostgres(t, url)
		if err := Restore(context.Background(), archive, "", RestoreOptions{Provider: store.Postgres, URL: url}); err != nil {
			t.Fatal(err)
		}
		s := openStoreAt(t, t.TempDir(), "postgres", url)
		defer s.Close()
		var admins int
		if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_admins").Scan(&admins); err != nil || admins != 1 {
			t.Fatalf("admins=%d err=%v", admins, err)
		}
		var enabled bool
		if err := s.DB().QueryRow("SELECT enabled FROM _trestle_webhooks WHERE id='wh_1'").Scan(&enabled); err != nil || enabled {
			t.Fatalf("webhook enabled after restore: %v err=%v", enabled, err)
		}
	})
}
