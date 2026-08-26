package appauth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/store"
)

func setup(t *testing.T) *Handler {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return New(s.DB(), adminauth.New(s.DB()))
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
	h := setup(t)
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
}
func TestConcurrentRefreshOnlyOneWins(t *testing.T) {
	h := setup(t)
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
}
func TestGenericLoginFailureAndPasswordMinimum(t *testing.T) {
	h := setup(t)
	w := call(t, h, "/api/v1/auth/register", map[string]any{"email": "x@example.com", "password": "123456"})
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
	a := call(t, h, "/api/v1/auth/login", map[string]any{"email": "missing@example.com", "password": "wrong"})
	b := call(t, h, "/api/v1/auth/login", map[string]any{"email": "x@example.com", "password": "wrong"})
	if a.Code != b.Code || !strings.Contains(a.Body.String(), "invalid_credentials") {
		t.Fatal("enumerating login response")
	}
}
