package appauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/audit"
	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/storetest"
)

// faultExecutor wraps a store.Executor and fails the first ExecContext whose
// SQL contains the target substring, so a specific statement in a mutation
// path can be made to fail while every preceding statement has already run.
type faultExecutor struct {
	store.Executor
	target  string
	matched bool
}

func (f *faultExecutor) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	if !f.matched && strings.Contains(q, f.target) {
		f.matched = true
		return nil, errors.New("injected statement failure")
	}
	return f.Executor.ExecContext(ctx, q, a...)
}

// BeginTx wraps the underlying transaction so a fault injected inside a
// multi-statement mutation path fails mid-transaction rather than being
// bypassed.
func (f *faultExecutor) BeginTx(ctx context.Context, o *sql.TxOptions) (store.Transaction, error) {
	tx, err := f.Executor.BeginTx(ctx, o)
	if err != nil {
		return nil, err
	}
	return &faultTransaction{Transaction: tx, f: f}, nil
}

type faultTransaction struct {
	store.Transaction
	f *faultExecutor
}

func (ft *faultTransaction) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	if !ft.f.matched && strings.Contains(q, ft.f.target) {
		ft.f.matched = true
		return nil, errors.New("injected statement failure")
	}
	return ft.Transaction.ExecContext(ctx, q, a...)
}

// TestLoginAndRefreshAreAtomicUnderInjectedFailure proves that a session and
// its short-lived access token commit together on both providers. When the
// access-token insert fails after the session row was already written, the
// whole login rolls back (no orphaned session) and a failed refresh leaves the
// original refresh token valid (no partially consumed rotation).
func TestLoginAndRefreshAreAtomicUnderInjectedFailure(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			if _, err := s.DB().Exec("UPDATE _trestle_app_registration_policy SET policy='open' WHERE id=1"); err != nil {
				t.Fatal(err)
			}
			admin := adminauth.New(s.DB(), string(s.Provider()))
			normal := New(s.DB(), admin)
			normal.SetAudit(audit.New(s.DB(), admin, string(s.Provider())))

			register := call(t, normal, "/api/v1/auth/register", map[string]any{"email": "user@example.com", "password": "1234567"})
			if register.Code != 201 {
				t.Fatalf("register %d %s", register.Code, register.Body.String())
			}

			// 1. Login with the access insert forced to fail: the session must
			// roll back too, leaving no session and no access row.
			loginFault := New(&faultExecutor{Executor: s.DB(), target: "_trestle_app_access"}, admin)
			w := call(t, loginFault, "/api/v1/auth/login", map[string]any{"email": "user@example.com", "password": "1234567"})
			if w.Code != 500 {
				t.Fatalf("failed login status=%d body=%s", w.Code, w.Body.String())
			}
			if err := assertCount(t, s, "SELECT count(*) FROM _trestle_app_sessions", 0); err != nil {
				t.Fatal(err)
			}
			if err := assertCount(t, s, "SELECT count(*) FROM _trestle_app_access", 0); err != nil {
				t.Fatal(err)
			}

			// 2. A successful login yields a usable refresh token.
			var loginOut struct {
				RefreshToken string `json:"refreshToken"`
			}
			w = call(t, normal, "/api/v1/auth/login", map[string]any{"email": "user@example.com", "password": "1234567"})
			if w.Code != 200 {
				t.Fatalf("login %d %s", w.Code, w.Body.String())
			}
			if err := jsonDecode(w, &loginOut); err != nil {
				t.Fatal(err)
			}

			// 3. Refresh with the access insert forced to fail: the original
			// session must stay valid (not revoked), no new session and no
			// access row may appear.
			refreshFault := New(&faultExecutor{Executor: s.DB(), target: "_trestle_app_access"}, admin)
			w = call(t, refreshFault, "/api/v1/auth/refresh", map[string]any{"refreshToken": loginOut.RefreshToken})
			if w.Code != 500 {
				t.Fatalf("failed refresh status=%d body=%s", w.Code, w.Body.String())
			}
			var liveSessions int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_app_sessions WHERE revoked_at IS NULL").Scan(&liveSessions); err != nil {
				t.Fatal(err)
			}
			if liveSessions != 1 {
				t.Fatalf("live sessions after failed refresh=%d, want 1 (original session must survive)", liveSessions)
			}
			// The failed refresh must not create a new access row: the count
			// stays at exactly the one row issued by the successful login.
			if err := assertCount(t, s, "SELECT count(*) FROM _trestle_app_access", 1); err != nil {
				t.Fatal(err)
			}

			// 4. The original refresh token still works after the failure.
			w = call(t, normal, "/api/v1/auth/refresh", map[string]any{"refreshToken": loginOut.RefreshToken})
			if w.Code != 200 {
				t.Fatalf("refresh after injected failure %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func assertCount(t *testing.T, s *store.Store, query string, want int) error {
	t.Helper()
	var got int
	if err := s.DB().QueryRow(query).Scan(&got); err != nil {
		return err
	}
	if got != want {
		return errors.New(query + " = " + strconv.Itoa(got) + ", want " + strconv.Itoa(want))
	}
	return nil
}

func jsonDecode(w *httptest.ResponseRecorder, out any) error {
	return json.NewDecoder(w.Body).Decode(out)
}
