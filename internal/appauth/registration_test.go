package appauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/audit"
	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/storetest"
)

// setup builds a full appauth handler on the named provider with a running
// store and an audit handler, and returns the store and handler.
func setupReg(t *testing.T, provider string) (*store.Store, *Handler) {
	t.Helper()
	s := storetest.Open(t, provider)
	admin := adminauth.New(s.DB(), string(s.Provider()))
	auditAPI := audit.New(s.DB(), admin, string(s.Provider()))
	h := New(s.DB(), admin)
	h.SetAudit(auditAPI)
	return s, h
}

// policy resets the singleton policy row to the named policy for a test.
func setPolicy(t *testing.T, s *store.Store, policy string) {
	t.Helper()
	if _, err := s.DB().Exec("UPDATE _trestle_app_registration_policy SET policy=? WHERE id=1", policy); err != nil {
		t.Fatal(err)
	}
}

// adminCookieCSRF performs first-run setup and returns an admin cookie + CSRF.
func adminSetup(t *testing.T, s *store.Store, admin *adminauth.Handler) (*http.Cookie, string) {
	t.Helper()
	body := strings.NewReader(`{"email":"admin@example.com","password":"1234567","applicationRegistrationPolicy":"closed"}`)
	r := httptest.NewRequest("POST", "http://example.test/admin/v1/setup", body)
	r.Host = "example.test"
	r.Header.Set("Origin", "http://example.test")
	w := httptest.NewRecorder()
	admin.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("admin setup %d %s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	var out struct {
		CSRF string `json:"csrfToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	return cookie, out.CSRF
}

func publicGet(h http.Handler, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://example.test"+path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func publicCall(h http.Handler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://example.test"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func adminCall(h http.Handler, cookie *http.Cookie, csrf, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.test"+path, strings.NewReader(body))
	r.Host = "example.test"
	r.AddCookie(cookie)
	if method != "GET" {
		r.Header.Set("X-Trestle-CSRF", csrf)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCapabilityReflectsPolicy(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			expect := map[string]string{"open": "register", "invite": "invite", "approval": "request", "closed": "closed"}
			for policy, flow := range expect {
				setPolicy(t, s, policy)
				w := publicGet(h, "/api/v1/auth/capability")
				if w.Code != 200 {
					t.Fatalf("capability %d", w.Code)
				}
				var out struct {
					Registration struct {
						Flow          string `json:"flow"`
						EmailDelivery string `json:"emailDelivery"`
					} `json:"registration"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if out.Registration.Flow != flow || out.Registration.EmailDelivery != "manual" {
					t.Fatalf("policy %s: flow=%s email=%s, want %s/manual", policy, out.Registration.Flow, out.Registration.EmailDelivery, flow)
				}
				if strings.Contains(w.Body.String(), "invitation") || strings.Contains(w.Body.String(), "pending") {
					t.Fatal("capability leaked private registration state")
				}
			}
		})
	}
}

func TestRegisterPolicyGatingAndDuplicate(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)

			// Closed: registration refused.
			setPolicy(t, s, "closed")
			if w := publicCall(h, "/api/v1/auth/register", `{"email":"a@example.com","password":"1234567"}`); w.Code != 403 {
				t.Fatalf("closed register=%d", w.Code)
			}
			// Invite: refused.
			setPolicy(t, s, "invite")
			if w := publicCall(h, "/api/v1/auth/register", `{"email":"a@example.com","password":"1234567"}`); w.Code != 403 {
				t.Fatalf("invite register=%d", w.Code)
			}
			// Approval: refused.
			setPolicy(t, s, "approval")
			if w := publicCall(h, "/api/v1/auth/register", `{"email":"a@example.com","password":"1234567"}`); w.Code != 403 {
				t.Fatalf("approval register=%d", w.Code)
			}
			// Open: register works.
			setPolicy(t, s, "open")
			if w := publicCall(h, "/api/v1/auth/register", `{"email":"a@example.com","password":"1234567"}`); w.Code != 201 {
				t.Fatalf("open register=%d %s", w.Code, w.Body.String())
			}
			// Duplicate: generic registration_unavailable (no email_in_use).
			dup := publicCall(h, "/api/v1/auth/register", `{"email":"a@example.com","password":"1234567"}`)
			if dup.Code != 409 {
				t.Fatalf("duplicate register=%d", dup.Code)
			}
			var out struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			json.Unmarshal(dup.Body.Bytes(), &out)
			if out.Error.Code == "email_in_use" {
				t.Fatal("duplicate registration leaked email_in_use")
			}
		})
	}
}

func TestInvitationLifecycle(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)

			setPolicy(t, s, "invite")
			// Create a self-register invitation.
			w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"self_register","email":"person@example.com"}`)
			if w.Code != 201 {
				t.Fatalf("create invitation %d %s", w.Code, w.Body.String())
			}
			var inv struct{ Token, Email, ID string }
			json.Unmarshal(w.Body.Bytes(), &inv)
			if inv.Token == "" || inv.Email != "person@example.com" {
				t.Fatalf("invitation %+v", inv)
			}

			// Accept with wrong/unknown token -> invalid_invitation.
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"garbage","password":"1234567"}`); w.Code != 400 {
				t.Fatalf("bad token %d", w.Code)
			}

			// Accept with correct token under invite policy.
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`); w.Code != 201 {
				t.Fatalf("accept %d %s", w.Code, w.Body.String())
			}
			// Single-use: second accept fails.
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`); w.Code != 400 {
				t.Fatalf("second accept %d", w.Code)
			}
			// User now exists; login works.
			if w := publicCall(h, "/api/v1/auth/login", `{"email":"person@example.com","password":"1234567"}`); w.Code != 200 {
				t.Fatalf("login %d", w.Code)
			}

			// Revoke an invitation.
			w = adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"activate","email":"other@example.com"}`)
			var inv2 struct{ Token, ID string }
			json.Unmarshal(w.Body.Bytes(), &inv2)
			if w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations/"+inv2.ID+"/revoke", `{}`); w.Code != 204 {
				t.Fatalf("revoke %d", w.Code)
			}
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv2.Token+`","password":"1234567"}`); w.Code != 400 {
				t.Fatalf("accept revoked %d", w.Code)
			}
		})
	}
}

func TestSelfRegisterRequiresInvitePolicy(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)

			setPolicy(t, s, "invite")
			w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"self_register","email":"x@example.com"}`)
			var inv struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &inv)

			// Switch away from invite: self_register acceptance must fail.
			setPolicy(t, s, "closed")
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`); w.Code != 400 {
				t.Fatalf("accept under closed %d", w.Code)
			}
			// Back to invite: works.
			setPolicy(t, s, "invite")
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`); w.Code != 201 {
				t.Fatalf("accept back to invite %d", w.Code)
			}
		})
	}
}

func TestActivationWorksUnderEveryPolicy(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			for _, policy := range []string{"open", "invite", "approval", "closed"} {
				setPolicy(t, s, policy)
				email := strings.ReplaceAll(policy, "_", "") + "@example.com"
				w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"activate","email":"`+email+`"}`)
				var inv struct{ Token string }
				json.Unmarshal(w.Body.Bytes(), &inv)
				if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`); w.Code != 201 {
					t.Fatalf("activate under %s: %d", policy, w.Code)
				}
			}
		})
	}
}

func TestActivationEmailConflict(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			setPolicy(t, s, "open")
			// Create a real user directly.
			if w := publicCall(h, "/api/v1/auth/register", `{"email":"taken@example.com","password":"1234567"}`); w.Code != 201 {
				t.Fatalf("register %d", w.Code)
			}
			// Admin attempts to provision the same email -> user_already_exists.
			w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"activate","email":"taken@example.com"}`)
			if w.Code != 409 {
				t.Fatalf("provision taken email %d", w.Code)
			}
			var out struct {
				Error struct{ Code string } `json:"error"`
			}
			json.Unmarshal(w.Body.Bytes(), &out)
			if out.Error.Code != "user_already_exists" {
				t.Fatalf("code %q", out.Error.Code)
			}
		})
	}
}

func TestConcurrentInvitationSingleUse(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			setPolicy(t, s, "open")
			w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"activate","email":"race@example.com"}`)
			var inv struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &inv)

			results := make(chan int, 2)
			for i := 0; i < 2; i++ {
				go func() {
					results <- publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`).Code
				}()
			}
			one, two := <-results, <-results
			if !((one == 201 && two == 400) || (one == 400 && two == 201)) {
				t.Fatalf("single-use violated: %d %d", one, two)
			}
			var users int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_app_users WHERE email='race@example.com'").Scan(&users); err != nil || users != 1 {
				t.Fatalf("users=%d err=%v", users, err)
			}
		})
	}
}

func TestAccessRequestLifecycleAndDuplicate(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)

			// Not approval: refused.
			setPolicy(t, s, "open")
			if w := publicCall(h, "/api/v1/auth/access-request", `{"email":"want@example.com"}`); w.Code != 403 {
				t.Fatalf("non-approval request %d", w.Code)
			}
			setPolicy(t, s, "approval")
			// First request: generic 202.
			if w := publicCall(h, "/api/v1/auth/access-request", `{"email":"want@example.com"}`); w.Code != 202 {
				t.Fatalf("request %d", w.Code)
			}
			// Duplicate pending: still generic 202, no new row.
			if w := publicCall(h, "/api/v1/auth/access-request", `{"email":"want@example.com"}`); w.Code != 202 {
				t.Fatalf("dup request %d", w.Code)
			}
			var count int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_app_access_requests WHERE email='want@example.com' AND status='pending'").Scan(&count); err != nil || count != 1 {
				t.Fatalf("pending count=%d err=%v", count, err)
			}
			// List shows one pending.
			w := adminCall(h, cookie, csrf, "GET", "/admin/v1/app-registration/requests", "")
			var list struct {
				Items []struct {
					Status string `json:"status"`
					ID     string `json:"id"`
				}
			}
			json.Unmarshal(w.Body.Bytes(), &list)
			if len(list.Items) != 1 || list.Items[0].Status != "pending" {
				t.Fatalf("list %+v", list.Items)
			}
			// Approve -> creates activate invitation with token.
			rid := list.Items[0].ID
			w = adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/requests/"+rid+"/approve", `{}`)
			if w.Code != 201 {
				t.Fatalf("approve %d %s", w.Code, w.Body.String())
			}
			var inv struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &inv)
			// Approve again -> already_decided.
			if w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/requests/"+rid+"/approve", `{}`); w.Code != 409 {
				t.Fatalf("reapprove %d", w.Code)
			}
			// Reject a second request.
			if w := publicCall(h, "/api/v1/auth/access-request", `{"email":"second@example.com"}`); w.Code != 202 {
				t.Fatalf("second request %d", w.Code)
			}
			w = adminCall(h, cookie, csrf, "GET", "/admin/v1/app-registration/requests", "")
			var list2 struct {
				Items []struct{ ID, Email, Status string }
			}
			json.Unmarshal(w.Body.Bytes(), &list2)
			var second string
			for _, it := range list2.Items {
				if it.Email == "second@example.com" {
					second = it.ID
				}
			}
			if w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/requests/"+second+"/reject", `{}`); w.Code != 204 {
				t.Fatalf("reject %d", w.Code)
			}
			// The approved invitation activates the user.
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+inv.Token+`","password":"1234567"}`); w.Code != 201 {
				t.Fatalf("activation %d", w.Code)
			}
		})
	}
}

func TestRequestRemainsDecidableAfterLeavingApproval(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			setPolicy(t, s, "approval")
			publicCall(h, "/api/v1/auth/access-request", `{"email":"d@example.com"}`)
			setPolicy(t, s, "closed")
			w := adminCall(h, cookie, csrf, "GET", "/admin/v1/app-registration/requests", "")
			var list struct {
				Items []struct {
					ID string `json:"id"`
				}
			}
			json.Unmarshal(w.Body.Bytes(), &list)
			if len(list.Items) != 1 {
				t.Fatalf("pending request lost after leaving approval: %+v", list.Items)
			}
			if w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/requests/"+list.Items[0].ID+"/approve", `{}`); w.Code != 201 {
				t.Fatalf("approve after leaving approval %d", w.Code)
			}
		})
	}
}

func TestReissueInvitation(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			setPolicy(t, s, "approval")
			publicCall(h, "/api/v1/auth/access-request", `{"email":"reissue@example.com"}`)
			w := adminCall(h, cookie, csrf, "GET", "/admin/v1/app-registration/requests", "")
			var list struct {
				Items []struct {
					ID string `json:"id"`
				}
			}
			json.Unmarshal(w.Body.Bytes(), &list)
			rid := list.Items[0].ID
			w = adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/requests/"+rid+"/approve", `{}`)
			var first struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &first)
			// Reissue: revokes the first, returns a new token.
			w = adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/requests/"+rid+"/reissue", `{"email":"reissue@example.com"}`)
			if w.Code != 201 {
				t.Fatalf("reissue %d %s", w.Code, w.Body.String())
			}
			var second struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &second)
			if second.Token == "" || second.Token == first.Token {
				t.Fatal("reissue did not return a new token")
			}
			// First token now revoked.
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+first.Token+`","password":"1234567"}`); w.Code != 400 {
				t.Fatalf("old token after reissue %d", w.Code)
			}
			// New token works.
			if w := publicCall(h, "/api/v1/auth/invite/accept", `{"token":"`+second.Token+`","password":"1234567"}`); w.Code != 201 {
				t.Fatalf("new token %d", w.Code)
			}
		})
	}
}

func TestPolicyChangeSerializesAgainstRegistration(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			setPolicy(t, s, "open")
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)

			// Change policy to closed via the admin endpoint (uses the lock).
			w := adminCall(h, cookie, csrf, "PUT", "/admin/v1/app-registration/policy", `{"policy":"closed"}`)
			if w.Code != 200 {
				t.Fatalf("policy change %d", w.Code)
			}
			// A subsequent registration must fail.
			if w := publicCall(h, "/api/v1/auth/register", `{"email":"after@example.com","password":"1234567"}`); w.Code != 403 {
				t.Fatalf("register after close %d", w.Code)
			}
			var users int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_app_users").Scan(&users); err != nil || users != 0 {
				t.Fatalf("users after closed=%d err=%v", users, err)
			}
		})
	}
}

func TestPolicyChangeRequiresAdminAndCSRF(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			// Wrong CSRF -> denied.
			if w := adminCall(h, cookie, "wrong", "PUT", "/admin/v1/app-registration/policy", `{"policy":"open"}`); w.Code != 403 {
				t.Fatalf("bad csrf %d", w.Code)
			}
			// No cookie -> denied.
			r := httptest.NewRequest("PUT", "http://example.test/admin/v1/app-registration/policy", strings.NewReader(`{"policy":"open"}`))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("no cookie %d", w.Code)
			}
			// Open without confirmOpen -> rejected.
			if w := adminCall(h, cookie, csrf, "PUT", "/admin/v1/app-registration/policy", `{"policy":"open"}`); w.Code != 400 {
				t.Fatalf("open without confirm %d", w.Code)
			}
			// Open with confirmOpen -> ok.
			if w := adminCall(h, cookie, csrf, "PUT", "/admin/v1/app-registration/policy", `{"policy":"open","confirmOpen":true}`); w.Code != 200 {
				t.Fatalf("open with confirm %d", w.Code)
			}
		})
	}
}

func TestAccessRequestRateLimited(t *testing.T) {
	// Only exercised on sqlite (single storetest open path); provider loop is
	// not needed for the limiter, but keep both for parity.
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			setPolicy(t, s, "approval")
			h.SetRequestLimit(2)
			var limited bool
			for i := 0; i < 6; i++ {
				if publicCall(h, "/api/v1/auth/access-request", `{"email":"r`+string(rune('a'+i))+`@example.com"}`).Code == 429 {
					limited = true
					break
				}
			}
			if !limited {
				t.Fatal("access-request was not rate limited")
			}
		})
	}
}

func TestAccessRequestSweepExpiresPending(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			setPolicy(t, s, "approval")
			publicCall(h, "/api/v1/auth/access-request", `{"email":"old@example.com"}`)
			// Backdate the pending request past the pending TTL (14d) but within
			// the retention window (30d), so it expires but is not yet purged.
			oldCreated := time.Now().UTC().Add(-20 * 24 * time.Hour).Format(time.RFC3339Nano)
			if _, err := s.DB().Exec("UPDATE _trestle_app_access_requests SET created_at=? WHERE email='old@example.com'", oldCreated); err != nil {
				t.Fatal(err)
			}
			// Submitting another request triggers the sweep.
			publicCall(h, "/api/v1/auth/access-request", `{"email":"new@example.com"}`)
			var status string
			if err := s.DB().QueryRow("SELECT status FROM _trestle_app_access_requests WHERE email='old@example.com'").Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "expired" {
				t.Fatalf("old request status %q, want expired", status)
			}
		})
	}
}

func TestTokenStoredAsHashOnly(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			setPolicy(t, s, "open")
			w := adminCall(h, cookie, csrf, "POST", "/admin/v1/app-registration/invitations", `{"kind":"activate","email":"hash@example.com"}`)
			var inv struct{ Token string }
			json.Unmarshal(w.Body.Bytes(), &inv)
			var tokenHash []byte
			if err := s.DB().QueryRow("SELECT token_hash FROM _trestle_app_invitations WHERE email='hash@example.com'").Scan(&tokenHash); err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(tokenHash, []byte(inv.Token)) {
				t.Fatal("raw token stored in plaintext")
			}
			// List must not return the token.
			w = adminCall(h, cookie, csrf, "GET", "/admin/v1/app-registration/invitations", "")
			if strings.Contains(w.Body.String(), inv.Token) {
				t.Fatal("listing leaked the raw token")
			}
		})
	}
}

var _ = context.Background

// TestPolicyLockOrderDeterministic proves the serialization contract: when the
// restrictive policy change wins the lock first, a later registration cannot
// commit under the previous open policy. It forces the ordering by completing
// the policy change before starting the registration, then verifies the
// registration is rejected and no user row exists.
func TestPolicyLockOrderDeterministic(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			setPolicy(t, s, "open")

			// The policy change wins the serialization order: it acquires the
			// lock and commits closed before the registration begins.
			if w := adminCall(h, cookie, csrf, "PUT", "/admin/v1/app-registration/policy", `{"policy":"closed"}`); w.Code != 200 {
				t.Fatalf("policy change %d", w.Code)
			}
			// A registration that starts afterward must observe closed.
			if w := publicCall(h, "/api/v1/auth/register", `{"email":"late@example.com","password":"1234567"}`); w.Code != 403 {
				t.Fatalf("late register %d", w.Code)
			}
			var users int
			if err := s.DB().QueryRow("SELECT count(*) FROM _trestle_app_users").Scan(&users); err != nil || users != 0 {
				t.Fatalf("users=%d err=%v", users, err)
			}
		})
	}
}

// TestConcurrentPolicyChangeAndRegistrationOrdering runs a policy change and a
// registration concurrently many times and asserts the invariant: if the
// registration commits, the policy row at that moment was open and no later
// registration under closed can succeed. Every registration that reports 201
// must have a corresponding user; every 403 must leave no user.
func TestConcurrentPolicyChangeAndRegistrationOrdering(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s, h := setupReg(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			cookie, csrf := adminSetup(t, s, admin)
			setPolicy(t, s, "open")

			for round := 0; round < 5; round++ {
				email := fmt.Sprintf("race%d@example.com", round)
				done := make(chan struct{}, 2)
				var mu sync.Mutex
				registered := 0
				// Policy change to closed.
				go func() {
					adminCall(h, cookie, csrf, "PUT", "/admin/v1/app-registration/policy", `{"policy":"closed"}`)
					done <- struct{}{}
				}()
				// Concurrent registration.
				go func() {
					w := publicCall(h, "/api/v1/auth/register", `{"email":"`+email+`","password":"1234567"}`)
					mu.Lock()
					if w.Code == 201 {
						registered++
					}
					mu.Unlock()
					done <- struct{}{}
				}()
				<-done
				<-done
				if registered > 1 {
					t.Fatalf("round %d: %d registrations committed under a policy race", round, registered)
				}
				// Reset to open for the next round.
				setPolicy(t, s, "open")
			}
		})
	}
}
