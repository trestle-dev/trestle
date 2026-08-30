package appauth

import (
	"encoding/json"
	"fmt"
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
// visible to in-flight authenticated requests. Each iteration races K
// authenticated requests against the logout (the logout is synchronized in the
// same barrier and waited on), captures the logout status, and then proves
// every subsequent request is rejected. The race is repeated across iterations
// so both interleavings (some requests before the committed revocation, some
// after) are exercised without requiring both in any single run.
func TestRevocationDuringInFlightRequests(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			successIterations := 0
			const iterations = 30
			const inflight = 12
			for iter := 0; iter < iterations; iter++ {
				email := fmt.Sprintf("user%d@example.com", iter)
				if w := call(t, h, "/api/v1/auth/register", map[string]any{"email": email, "password": "1234567"}); w.Code != 201 {
					t.Fatalf("register %d %s", w.Code, w.Body.String())
				}
				login := call(t, h, "/api/v1/auth/login", map[string]any{"email": email, "password": "1234567"})
				var out struct {
					AccessToken  string `json:"accessToken"`
					RefreshToken string `json:"refreshToken"`
				}
				if err := json.Unmarshal(login.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}

				ready := make(chan struct{})
				var wg sync.WaitGroup
				results := make([]bool, inflight)
				for i := 0; i < inflight; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						<-ready
						_, ok := h.Authenticate(authRequest(out.AccessToken))
						results[i] = ok
					}(i)
				}
				var logoutStatus int
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-ready
					logoutStatus = call(t, h, "/api/v1/auth/logout", map[string]any{"refreshToken": out.RefreshToken}).Code
				}()
				close(ready)
				wg.Wait() // waits for the logout goroutine too

				if logoutStatus != 204 {
					t.Fatalf("logout status=%d, want 204", logoutStatus)
				}
				// At least one request observed the pre-revocation state across
				// the iteration set; individual runs may observe either side.
				sawSuccess := false
				for _, ok := range results {
					if ok {
						sawSuccess = true
					}
				}
				if sawSuccess {
					successIterations++
				}
				// After logout has returned, every later request is rejected.
				if _, ok := h.Authenticate(authRequest(out.AccessToken)); ok {
					t.Fatalf("iteration %d: access token accepted after logout returned", iter)
				}
			}
			if successIterations == 0 {
				t.Fatal("race never exercised the pre-revocation path across iterations")
			}
		})
	}
}
