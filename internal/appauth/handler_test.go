package appauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/storetest"
)

func setup(t *testing.T, provider string) *Handler {
	t.Helper()
	s := storetest.Open(t, provider)
	h := New(s.DB(), adminauth.New(s.DB(), string(s.Provider())))
	// The default registration policy for a fresh database is closed; most
	// registration tests exercise the open-registration path, so set it open.
	if _, err := s.DB().Exec("UPDATE _trestle_app_registration_policy SET policy='open' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	return h
}
func call(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	json.NewEncoder(&b).Encode(body)
	r := httptest.NewRequest("POST", path, &b)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func registerAndLogin(t *testing.T, h *Handler) string {
	w := call(t, h, "/api/v1/auth/register", map[string]any{"email": "user@example.com", "password": "1234567"})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	w = call(t, h, "/api/v1/auth/login", map[string]any{"email": "user@example.com", "password": "1234567"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var out struct {
		RefreshToken string `json:"refreshToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	return out.RefreshToken
}
func TestRegistrationLoginRotationRevocation(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			refresh := registerAndLogin(t, h)
			w := call(t, h, "/api/v1/auth/refresh", map[string]any{"refreshToken": refresh})
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			w = call(t, h, "/api/v1/auth/refresh", map[string]any{"refreshToken": refresh})
			if w.Code != 401 {
				t.Fatalf("reused refresh %d", w.Code)
			}
			var next struct {
				RefreshToken string `json:"refreshToken"`
			}
			callResult := call(t, h, "/api/v1/auth/login", map[string]any{"email": "user@example.com", "password": "1234567"})
			json.Unmarshal(callResult.Body.Bytes(), &next)
			w = call(t, h, "/api/v1/auth/logout", map[string]any{"refreshToken": next.RefreshToken})
			if w.Code != 204 {
				t.Fatal(w.Code)
			}
		})
	}
}
func TestConcurrentRefreshOnlyOneWins(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			refresh := registerAndLogin(t, h)
			codes := make(chan int, 2)
			var wg sync.WaitGroup
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					codes <- call(t, h, "/api/v1/auth/refresh", map[string]any{"refreshToken": refresh}).Code
				}()
			}
			wg.Wait()
			close(codes)
			ok, denied := 0, 0
			for code := range codes {
				if code == 200 {
					ok++
				}
				if code == 401 {
					denied++
				}
			}
			if ok != 1 || denied != 1 {
				t.Fatalf("ok=%d denied=%d", ok, denied)
			}
		})
	}
}
func TestEmailUniquenessIsCaseInsensitive(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			w := call(t, h, "/api/v1/auth/register", map[string]any{"email": "User@Example.COM", "password": "1234567"})
			if w.Code != 201 {
				t.Fatal(w.Body.String())
			}
			w = call(t, h, "/api/v1/auth/register", map[string]any{"email": "user@example.com", "password": "1234567"})
			if w.Code != 409 {
				t.Fatalf("case-variant registration: %d %s", w.Code, w.Body.String())
			}
			w = call(t, h, "/api/v1/auth/login", map[string]any{"email": "USER@example.com", "password": "1234567"})
			if w.Code != 200 {
				t.Fatalf("case-variant login: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGenericLoginFailureAndPasswordMinimum(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			w := call(t, h, "/api/v1/auth/register", map[string]any{"email": "x@example.com", "password": "123456"})
			if w.Code != 422 {
				t.Fatal(w.Code)
			}
			a := call(t, h, "/api/v1/auth/login", map[string]any{"email": "missing@example.com", "password": "wrong"})
			b := call(t, h, "/api/v1/auth/login", map[string]any{"email": "x@example.com", "password": "wrong"})
			if a.Code != b.Code || !strings.Contains(a.Body.String(), "invalid_credentials") {
				t.Fatal("enumerating login response")
			}
		})
	}
}

func TestAccessTokenIsSeparateAndExpiresWithSession(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			registerAndLogin(t, h)
			w := call(t, h, "/api/v1/auth/login", map[string]any{"email": "user@example.com", "password": "1234567"})
			var out struct{ AccessToken, RefreshToken string }
			json.Unmarshal(w.Body.Bytes(), &out)
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer "+out.AccessToken)
			if id, ok := h.Authenticate(r); !ok || id == "" {
				t.Fatal("access token denied")
			}
			r.Header.Set("Authorization", "Bearer "+out.RefreshToken)
			if _, ok := h.Authenticate(r); ok {
				t.Fatal("refresh token accepted as access token")
			}
			call(t, h, "/api/v1/auth/logout", map[string]any{"refreshToken": out.RefreshToken})
			r.Header.Set("Authorization", "Bearer "+out.AccessToken)
			if _, ok := h.Authenticate(r); ok {
				t.Fatal("access survived session revocation")
			}
		})
	}
}

func TestConcurrentLoginsCoexistAndRevokeIndependently(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			registerAndLogin(t, h)
			type sessionTokens struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
			}
			type loginResult struct {
				code int
				out  sessionTokens
			}
			const workers = 4
			start := make(chan struct{})
			results := make(chan loginResult, workers)
			body := map[string]any{"email": "user@example.com", "password": "1234567"}
			for i := 0; i < workers; i++ {
				go func() {
					<-start
					var b bytes.Buffer
					json.NewEncoder(&b).Encode(body)
					r := httptest.NewRequest("POST", "/api/v1/auth/login", &b)
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					var out sessionTokens
					json.Unmarshal(w.Body.Bytes(), &out)
					results <- loginResult{code: w.Code, out: out}
				}()
			}
			close(start)
			logins := make([]loginResult, 0, workers)
			for i := 0; i < workers; i++ {
				logins = append(logins, <-results)
			}
			seen := map[string]bool{}
			for i, lg := range logins {
				if lg.code != 200 || lg.out.AccessToken == "" || lg.out.RefreshToken == "" {
					t.Fatalf("login %d code=%d tokens empty", i, lg.code)
				}
				if seen[lg.out.AccessToken] || seen[lg.out.RefreshToken] {
					t.Fatal("concurrent logins produced a duplicate token")
				}
				seen[lg.out.AccessToken] = true
				seen[lg.out.RefreshToken] = true
			}
			authenticate := func(token string) bool {
				r := httptest.NewRequest("GET", "/", nil)
				r.Header.Set("Authorization", "Bearer "+token)
				_, ok := h.Authenticate(r)
				return ok
			}
			for i, lg := range logins {
				if !authenticate(lg.out.AccessToken) {
					t.Fatalf("concurrent login %d did not authenticate", i)
				}
			}
			call(t, h, "/api/v1/auth/logout", map[string]any{"refreshToken": logins[0].out.RefreshToken})
			if authenticate(logins[0].out.AccessToken) {
				t.Fatal("first session survived revocation")
			}
			for i := 1; i < workers; i++ {
				if !authenticate(logins[i].out.AccessToken) {
					t.Fatalf("session %d was revoked by the first logout", i)
				}
			}
		})
	}
}
