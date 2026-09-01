package appauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/trestle-dev/trestle/internal/storetest"
)

func authRequest(accessToken string) *http.Request {
	r := httptest.NewRequest("GET", "http://example.test/api/v1/whatever", nil)
	r.Header.Set("Authorization", "Bearer "+accessToken)
	return r
}

// TestRevocationDuringInFlightRequests proves session revocation is visible to
// requests started after logout returns. In-flight requests raced with the
// logout may complete before or after the committed revocation; the test does
// not require either in-flight side in any single run (that would depend on
// scheduler timing and be flaky in CI). It guarantees, per iteration:
//   - concurrent in-flight requests return a clean boolean result;
//   - logout is synchronized in the same barrier and awaited (status 204);
//   - every request started after logout returns is rejected.
func TestRevocationDuringInFlightRequests(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			h := setup(t, provider)
			// The register rate limit (10/min) would reject the many sequential
			// registrations this concurrency test performs, so raise it.
			h.limiter = newLimiter(1000, time.Minute)
			const iterations = 20
			const inflight = 8
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
				wg.Wait() // includes the logout goroutine

				if logoutStatus != 204 {
					t.Fatalf("logout status=%d, want 204", logoutStatus)
				}
				// Every in-flight result is a clean boolean (requests may have
				// completed before or after the committed revocation; neither
				// side is required in any run).
				for _, ok := range results {
					if ok != true && ok != false {
						t.Fatalf("torn in-flight result: %v", ok)
					}
				}
				// Every request started after logout returns is rejected.
				if _, ok := h.Authenticate(authRequest(out.AccessToken)); ok {
					t.Fatalf("iteration %d: access token accepted after logout returned", iter)
				}
			}
		})
	}
}
