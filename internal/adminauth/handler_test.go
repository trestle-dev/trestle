package adminauth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/requestmeta"
	"github.com/trestle-dev/trestle/internal/store"
)

func testHandler(t *testing.T) *Handler {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return New(s.DB())
}

func TestTrustedHTTPSIssuesSecureCookie(t *testing.T) {
	h := testHandler(t)
	r := httptest.NewRequest(http.MethodPost, "/admin/v1/setup", strings.NewReader(`{"email":"admin@example.com","password":"mudblood"}`))
	r.Host = "example.test"
	r.Header.Set("Origin", "https://example.test")
	r = requestmeta.With(r, "https", "203.0.113.9")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	if cookies := w.Result().Cookies(); len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("expected secure cookie: %#v", cookies)
	}
}
func request(t *testing.T, h http.Handler, method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &data)
	r.Host = "example.test"
	r.Header.Set("Origin", "http://example.test")
	r.RemoteAddr = "192.0.2.1:1234"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if csrf != "" {
		r.Header.Set("X-Trestle-CSRF", csrf)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSetupLoginLogoutLifecycle(t *testing.T) {
	h := testHandler(t)
	w := request(t, h, "GET", "/admin/v1/setup/status", nil, nil, "")
	if !strings.Contains(w.Body.String(), "true") {
		t.Fatal(w.Body.String())
	}
	w = request(t, h, "POST", "/admin/v1/setup", credentials{Email: "Admin@Example.com", Password: "correct horse battery staple"}, nil, "")
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	setupCookie := w.Result().Cookies()[0]
	var session sessionResponse
	if err := json.NewDecoder(w.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	w = request(t, h, "POST", "/admin/v1/setup", credentials{Email: "other@example.com", Password: "correct horse battery staple"}, nil, "")
	if w.Code != 409 {
		t.Fatalf("second setup: %d", w.Code)
	}
	w = request(t, h, "DELETE", "/admin/v1/session", nil, setupCookie, "")
	if w.Code != 403 {
		t.Fatalf("logout without csrf: %d", w.Code)
	}
	w = request(t, h, "DELETE", "/admin/v1/session", nil, setupCookie, session.CSRFToken)
	if w.Code != 204 {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "GET", "/admin/v1/session", nil, setupCookie, "")
	if !strings.Contains(w.Body.String(), "false") {
		t.Fatal("revoked session remained active")
	}
	w = request(t, h, "POST", "/admin/v1/session", credentials{Email: "admin@example.com", Password: "wrong password"}, nil, "")
	if w.Code != 401 || !strings.Contains(w.Body.String(), "invalid_credentials") {
		t.Fatalf("bad login: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "POST", "/admin/v1/session", credentials{Email: "admin@example.com", Password: "correct horse battery staple"}, nil, "")
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
}

func TestSetupValidationAndOrigin(t *testing.T) {
	h := testHandler(t)
	w := request(t, h, "POST", "/admin/v1/setup", credentials{Email: "bad", Password: "short"}, nil, "")
	if w.Code != 422 {
		t.Fatalf("got %d", w.Code)
	}
	r := httptest.NewRequest("POST", "/admin/v1/setup", strings.NewReader(`{"email":"a@example.com","password":"correct horse battery staple"}`))
	r.Host = "example.test"
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("origin got %d", w.Code)
	}
}

func TestPasswordEncoding(t *testing.T) {
	encoded, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") || !verifyPassword(encoded, "correct horse battery staple") || verifyPassword(encoded, "wrong password") {
		t.Fatal("password verification contract failed")
	}
	if _, err := hashPassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
}
