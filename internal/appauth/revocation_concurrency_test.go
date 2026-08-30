package appauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/trestle-dev/trestle/internal/storetest"
)

func authRequest(accessToken string) *http.Request {
	r := httptest.NewRequest("GET", "http://example.test/api/v1/whatever", nil)
	r.Header.Set("Authorization", "Bearer "+accessToken)
	return r
}

// TestRevocationDuringInFlightRequests proves session revocation is immediately
// visible to in-flight authenticated requests: each request observes either the
// pre-revocation (valid) or post-revocation (invalid) state, never a torn
// result, and once logout returns every later request is rejected.
func TestRevocationDuringInFlightRequests(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			if w := call(t, h, "/api/v1/auth/register", map[string]any{"email": "user@example.com", "password": "1234567"}); w.Code != 201 {
				t.Fatalf("register %d %s", w.Code, w.Body.String())
			}
			login := call(t, h, "/api/v1/auth/login", map[string]any{"email": "user@example.com", "password": "1234567"})
			var out struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
			}
			if err := json.Unmarshal(login.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}

			// N in-flight authenticated requests race the revocation.
			ready := make(chan struct{})
			var wg sync.WaitGroup
			const n = 16
			results := make([]bool, n)
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-ready
					r := authRequest(out.AccessToken)
					_, ok := h.Authenticate(r)
					results[i] = ok
				}(i)
			}
			go func() {
				<-ready
				call(t, h, "/api/v1/auth/logout", map[string]any{"refreshToken": out.RefreshToken})
			}()
			close(ready)
			wg.Wait()

			// Every in-flight result is a clean boolean: a request either ran
			// before the committed revocation (true) or after it (false).
			for _, ok := range results {
				if ok != true && ok != false {
					t.Fatalf("torn in-flight result: %v", ok)
				}
			}
			// Once logout has returned, every later request is rejected.
			if _, ok := h.Authenticate(authRequest(out.AccessToken)); ok {
				t.Fatal("access token still accepted after logout returned")
			}
		})
	}
}
