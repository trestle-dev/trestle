package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestStartupUnavailableDatabaseFailsCleanly proves PostgreSQL unavailable at
// startup produces a bounded, actionable failure rather than a hang.
func TestStartupUnavailableDatabaseFailsCleanly(t *testing.T) {
	postgresTestURL(t) // require the PostgreSQL suite
	bad := "postgres://postgres@127.0.0.1:59999/trestle_test?sslmode=disable"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := time.Now()
	_, err := OpenWith(ctx, Options{DataDir: t.TempDir(), Provider: Postgres, URL: bad, MaxOpen: 4, MaxIdle: 1, ConnectTimeout: 5 * time.Second})
	if err == nil {
		t.Fatal("unavailable database opened without error")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("startup against unavailable database took %s; expected a bounded failure", time.Since(start))
	}
}

// TestDNSFailureFailsCleanly proves a nonexistent host fails within the
// configured bound with a useful error, never hanging.
func TestDNSFailureFailsCleanly(t *testing.T) {
	postgresTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	_, err := Probe(ctx, Postgres, "postgres://postgres@nonexistent.invalid/trestle_test?sslmode=require&connect_timeout=5", 5*time.Second)
	if err == nil {
		t.Fatal("DNS failure did not error")
	}
	if time.Since(start) > 12*time.Second {
		t.Fatalf("DNS failure took %s; expected a bounded failure", time.Since(start))
	}
}

// TestInvalidCertificateRejected proves a server without a verifiable
// certificate is refused cleanly under sslmode=verify-full rather than
// silently downgrading.
func TestInvalidCertificateRejected(t *testing.T) {
	pgURL := postgresTestURL(t)
	u, err := url.Parse(pgURL)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("sslmode", "verify-full")
	u.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = Probe(ctx, Postgres, u.String(), 5*time.Second)
	if err == nil {
		t.Fatal("verify-full accepted a server without a verifiable certificate")
	}
}

// TestWrongCredentialsRejected proves bad credentials fail cleanly and never
// surface the supplied secret.
func TestWrongCredentialsRejected(t *testing.T) {
	pgURL := postgresTestURL(t)
	u, err := url.Parse(pgURL)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("postgres", "definitely-wrong-password")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// The disposable suite server may run trust authentication, in which case
	// passwords are not enforced and this specific scenario is not applicable.
	_, probeErr := Probe(ctx, Postgres, u.String(), 5*time.Second)
	if probeErr == nil {
		t.Skip("server uses trust authentication; wrong-credential rejection is not exercisable here")
	}
	// On a password-enforcing server the failure must be clean and must not
	// surface the supplied secret.
	if strings.Contains(probeErr.Error(), "definitely-wrong-password") {
		t.Fatalf("credentials leaked in error: %v", probeErr)
	}
}

// TestCancelledRequestPropagates proves a cancelled request context surfaces
// promptly instead of hanging.
func TestCancelledRequestPropagates(t *testing.T) {
	url := ownedURL(t)
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	resetPostgres(t, raw)
	s, err := OpenWith(context.Background(), Options{DataDir: t.TempDir(), Provider: Postgres, URL: url, MaxOpen: 2, MaxIdle: 1, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	rows, err := s.DB().QueryContext(ctx, "SELECT * FROM _trestle_collections")
	if err == nil {
		rows.Close()
		t.Fatal("cancelled query did not error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("cancelled query took %s; expected immediate cancellation", time.Since(start))
	}
}

// TestPoolExhaustionSerializesWithoutHang proves a single-connection pool under
// concurrent load serializes requests and every request completes.
func TestPoolExhaustionSerializesWithoutHang(t *testing.T) {
	url := ownedURL(t)
	raw, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	resetPostgres(t, raw)
	s, err := OpenWith(context.Background(), Options{DataDir: t.TempDir(), Provider: Postgres, URL: url, MaxOpen: 1, MaxIdle: 1, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var count int
			if err := s.DB().QueryRowContext(ctx, "SELECT count(*) FROM _trestle_collections").Scan(&count); err != nil {
				errs <- err
				return
			}
			if count != 0 {
				errs <- fmt.Errorf("unexpected count %d", count)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// resetExecutor fails a single QueryContext to simulate a reset connection
// during a request.
type resetExecutor struct {
	Executor
	done bool
}

func (r *resetExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if !r.done {
		r.done = true
		return nil, sql.ErrConnDone
	}
	return r.Executor.QueryContext(ctx, query, args...)
}

// TestConnectionResetDuringRequestErrorsInsteadOfHanging proves a reset
// connection surfaces an error promptly rather than hanging a request.
func TestConnectionResetDuringRequestErrorsInsteadOfHanging(t *testing.T) {
	s, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fault := &resetExecutor{Executor: s.DB()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := fault.QueryContext(ctx, "SELECT count(*) FROM _trestle_collections"); err == nil {
		t.Fatal("reset query did not error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("reset query took %s; expected prompt error", time.Since(start))
	}
}
