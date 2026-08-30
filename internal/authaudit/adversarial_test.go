package authaudit

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/apidocs"
	"github.com/trestle-dev/trestle/internal/appauth"
	"github.com/trestle-dev/trestle/internal/audit"
	"github.com/trestle-dev/trestle/internal/collections"
	"github.com/trestle-dev/trestle/internal/events"
	"github.com/trestle-dev/trestle/internal/files"
	"github.com/trestle-dev/trestle/internal/functions"
	"github.com/trestle-dev/trestle/internal/identities"
	"github.com/trestle-dev/trestle/internal/jobs"
	"github.com/trestle-dev/trestle/internal/records"
	"github.com/trestle-dev/trestle/internal/rules"
	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/storetest"
	"github.com/trestle-dev/trestle/internal/webhooks"
)

// buildApp wires the full handler stack (mirroring cmd/trestle) over one
// store so route protection can be exercised end to end.
func buildApp(t *testing.T, provider string) (http.Handler, *store.Store, adminSession) {
	t.Helper()
	s := storetest.Open(t, provider)
	admin := adminauth.New(s.DB(), string(s.Provider()))
	sess := setupAdmin(t, admin)
	collectionsAPI := collections.New(s.DB(), admin)
	credentials := identities.New(s.DB(), admin)
	recordAPI := records.New(s.DB(), admin, credentials)
	applicationAuth := appauth.New(s.DB(), admin)
	accessRules := rules.New(s.DB(), admin)
	recordAPI.ConfigureAccess(applicationAuth, accessRules)
	eventAPI := events.New(s.DB(), admin, credentials)
	recordAPI.ConfigureEvents(eventAPI)
	auditAPI := audit.New(s.DB(), admin, string(s.Provider()))
	recordAPI.ConfigureAudit(auditAPI)
	jobAPI := jobs.New(s.DB(), admin)
	webhookAPI := mustWebhooks(webhooks.New(s.DB(), admin, jobAPI, t.TempDir()))
	eventAPI.ConfigureDispatcher(webhookAPI)
	functionAPI := functions.New(s.DB(), admin, jobAPI, functions.Options{})
	eventAPI.ConfigureDispatcher(functionAPI)
	apiDocs := apidocs.New(s.DB(), admin)
	fileAPI, err := files.New(s.DB(), admin, credentials, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/", applicationAuth)
	mux.Handle("/api/v1/collections/", recordAPI)
	mux.Handle("/api/v1/files", fileAPI)
	mux.Handle("/api/v1/files/", fileAPI)
	mux.Handle("/api/v1/realtime", eventAPI)
	mux.Handle("/api/v1/openapi.json", apiDocs)
	mux.Handle("/admin/v1/collections", collectionsAPI)
	mux.Handle("/admin/v1/collections/", collectionsAPI)
	mux.Handle("/admin/v1/data/", recordAPI)
	mux.Handle("/admin/v1/app-users", applicationAuth)
	mux.Handle("/admin/v1/app-users/", applicationAuth)
	mux.Handle("/admin/v1/credentials", credentials)
	mux.Handle("/admin/v1/credentials/", credentials)
	mux.Handle("/admin/v1/collection-rules/", accessRules)
	mux.Handle("/admin/v1/files", fileAPI)
	mux.Handle("/admin/v1/files/", fileAPI)
	mux.Handle("/admin/v1/events", eventAPI)
	mux.Handle("/admin/v1/audit", auditAPI)
	mux.Handle("/admin/v1/jobs", jobAPI)
	mux.Handle("/admin/v1/jobs/", jobAPI)
	mux.Handle("/admin/v1/webhooks", webhookAPI)
	mux.Handle("/admin/v1/webhooks/", webhookAPI)
	mux.Handle("/admin/v1/functions", functionAPI)
	mux.Handle("/admin/v1/functions/", functionAPI)
	mux.Handle("/admin/v1/api/schema", apiDocs)
	mux.Handle("/admin/v1/", admin)
	return mux, s, sess
}

// TestUnauthenticatedRoutesAreRejected proves every protected admin and API
// route rejects an unauthenticated request and never returns success or leaks
// credentials.
func TestUnauthenticatedRoutesAreRejected(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			app, _, _ := buildApp(t, provider)
			protected := []string{
				"GET /admin/v1/collections",
				"POST /admin/v1/collections",
				"GET /admin/v1/credentials",
				"POST /admin/v1/credentials",
				"GET /admin/v1/collection-rules/anything",
				"GET /admin/v1/files",
				"GET /admin/v1/events",
				"GET /admin/v1/audit",
				"GET /admin/v1/jobs",
				"GET /admin/v1/webhooks",
				"GET /admin/v1/functions",
				"GET /admin/v1/api/schema",
				"GET /api/v1/collections/x/records",
			}
			for _, spec := range protected {
				parts := strings.SplitN(spec, " ", 2)
				w := httptest.NewRecorder()
				app.ServeHTTP(w, httptest.NewRequest(parts[0], "http://example.test"+parts[1], nil))
				if w.Code == 200 || w.Code == 201 {
					t.Errorf("%s unauthenticated returned %d", spec, w.Code)
				}
				if strings.Contains(strings.ToLower(w.Body.String()), "password") {
					t.Errorf("%s leaked a password: %s", spec, w.Body.String())
				}
			}
		})
	}
}

// TestCookieFlagsAndCSRFEnforcement proves the admin cookie is HttpOnly and
// SameSite=Strict, and that browser mutations without a valid CSRF token or
// with a foreign origin are rejected.
func TestCookieFlagsAndCSRFEnforcement(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			app, _, sess := buildApp(t, provider)
			if sess.cookie.Name != "trestle_admin_session" {
				t.Fatalf("cookie name %q", sess.cookie.Name)
			}
			if !sess.cookie.HttpOnly {
				t.Error("admin cookie is not HttpOnly")
			}
			if sess.cookie.SameSite != http.SameSiteStrictMode {
				t.Error("admin cookie is not SameSite=Strict")
			}
			noCSRF := httptest.NewRequest("POST", "http://example.test/admin/v1/collections", bytes.NewBufferString(`{"name":"x","fields":[]}`))
			noCSRF.Host = "example.test"
			noCSRF.Header.Set("Origin", "http://example.test")
			noCSRF.AddCookie(sess.cookie)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, noCSRF)
			if w.Code != 403 {
				t.Fatalf("mutation without CSRF returned %d, want 403", w.Code)
			}
			foreign := httptest.NewRequest("POST", "http://example.test/admin/v1/collections", bytes.NewBufferString(`{"name":"x","fields":[]}`))
			foreign.Host = "example.test"
			foreign.Header.Set("Origin", "http://evil.example")
			foreign.Header.Set("X-Trestle-CSRF", sess.csrf)
			foreign.AddCookie(sess.cookie)
			w = httptest.NewRecorder()
			app.ServeHTTP(w, foreign)
			if w.Code != 403 {
				t.Fatalf("mutation with foreign origin returned %d, want 403", w.Code)
			}
		})
	}
}

// failBeginExecutor fails BeginTx; failCommitExecutor fails Commit.
type failBeginExecutor struct {
	store.Executor
}
type failCommitExecutor struct {
	store.Executor
}
type failCommitTx struct {
	store.Transaction
}

func (f *failBeginExecutor) BeginTx(ctx context.Context, o *sql.TxOptions) (store.Transaction, error) {
	return nil, errors.New("injected begin failure")
}
func (f *failCommitExecutor) BeginTx(ctx context.Context, o *sql.TxOptions) (store.Transaction, error) {
	tx, err := f.Executor.BeginTx(ctx, o)
	if err != nil {
		return nil, err
	}
	return &failCommitTx{Transaction: tx}, nil
}
func (t *failCommitTx) Commit() error { return errors.New("injected commit failure") }

// TestSecuritySensitiveBeginAndCommitFailures proves the transactional
// app-user disable path fails closed on begin and commit failures, not only on
// Exec failures.
func TestSecuritySensitiveBeginAndCommitFailures(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			sess := setupAdmin(t, admin)

			beginApp := appauth.New(&failBeginExecutor{Executor: s.DB()}, admin)
			if w := do(beginApp, sess, "POST", "/admin/v1/app-users/usr_none/disable", nil); w.Code != 500 {
				t.Fatalf("disable begin failure returned %d, want 500", w.Code)
			}
			commitApp := appauth.New(&failCommitExecutor{Executor: s.DB()}, admin)
			if w := do(commitApp, sess, "POST", "/admin/v1/app-users/usr_none/disable", nil); w.Code != 500 {
				t.Fatalf("disable commit failure returned %d, want 500", w.Code)
			}
		})
	}
}

// TestAuthAdversarialIdentity covers disabled users, revoked sessions, token
// replay, session fixation (distinct cookies per login) and credential
// leakage in responses.
func TestAuthAdversarialIdentity(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			sess := setupAdmin(t, admin)
			appUser := appauth.New(s.DB(), admin)

			if w := do(appUser, sess, "POST", "/api/v1/auth/register", map[string]any{"email": "u@example.com", "password": "1234567"}); w.Code != 201 {
				t.Fatalf("register %d", w.Code)
			}
			login1 := do(appUser, sess, "POST", "/api/v1/auth/login", map[string]any{"email": "u@example.com", "password": "1234567"})
			var tok struct {
				UserID       string `json:"userId"`
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
			}
			json.Unmarshal(login1.Body.Bytes(), &tok)
			if strings.Contains(login1.Body.String(), "1234567") {
				t.Fatal("password leaked in login response")
			}

			// Session fixation: a second login issues a different refresh token.
			login2 := do(appUser, sess, "POST", "/api/v1/auth/login", map[string]any{"email": "u@example.com", "password": "1234567"})
			var tok2 struct {
				RefreshToken string `json:"refreshToken"`
			}
			json.Unmarshal(login2.Body.Bytes(), &tok2)
			if tok2.RefreshToken == tok.RefreshToken {
				t.Fatal("login reissued the same refresh token (fixation)")
			}

			// Logout revokes the session; the access token must no longer work.
			if w := do(appUser, sess, "POST", "/api/v1/auth/logout", map[string]any{"refreshToken": tok.RefreshToken}); w.Code != 204 {
				t.Fatalf("logout %d", w.Code)
			}
			authReq := httptest.NewRequest("GET", "http://example.test/api/v1/x", nil)
			authReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
			if _, ok := appUser.Authenticate(authReq); ok {
				t.Fatal("revoked session still authenticates")
			}
			// Token replay: the consumed refresh token is rejected on refresh.
			if w := do(appUser, sess, "POST", "/api/v1/auth/refresh", map[string]any{"refreshToken": tok.RefreshToken}); w.Code != 401 {
				t.Fatalf("refresh token replay returned %d, want 401", w.Code)
			}

			// Disabled user: login is rejected with a generic error.
			if w := do(appUser, sess, "POST", "/admin/v1/app-users/"+tok.UserID+"/disable", nil); w.Code != 204 {
				t.Fatalf("disable user %d %s", w.Code, w.Body.String())
			}
			denied := do(appUser, sess, "POST", "/api/v1/auth/login", map[string]any{"email": "u@example.com", "password": "1234567"})
			if denied.Code != 401 || strings.Contains(denied.Body.String(), "disabled") {
				t.Fatalf("disabled-user login returned %d %s", denied.Code, denied.Body.String())
			}
		})
	}
}

// TestRateLimiterThrottlesAbuse proves repeated failed logins from one client
// address are throttled while a distinct address is unaffected.
func TestRateLimiterThrottlesAbuse(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			sess := setupAdmin(t, admin)
			appUser := appauth.New(s.DB(), admin)
			for i := 0; i < 11; i++ {
				w := do(appUser, sess, "POST", "/api/v1/auth/login", map[string]any{"email": "missing@example.com", "password": "wrong"})
				if i < 10 && w.Code != 401 {
					t.Fatalf("attempt %d returned %d, want 401", i, w.Code)
				}
				if i == 10 && w.Code != 429 {
					t.Fatalf("attempt %d returned %d, want 429 (throttled)", i, w.Code)
				}
			}
			// A different client address is not throttled.
			r := httptest.NewRequest("POST", "http://example.test/api/v1/auth/login", bytes.NewBufferString(`{"email":"missing@example.com","password":"wrong"}`))
			r.Host = "example.test"
			r.RemoteAddr = "10.0.0.99:1234"
			w := httptest.NewRecorder()
			appUser.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatalf("distinct-address login returned %d, want 401 (not throttled)", w.Code)
			}
		})
	}
}
