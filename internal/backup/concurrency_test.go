package backup

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/trestle-dev/trestle/internal/storetest"
)

// TestConcurrentImportSingleWinner proves two concurrent imports into one empty
// initialized destination never merge partially: at most one import succeeds,
// and the destination is either fully the archive or unchanged (empty).
func TestConcurrentImportSingleWinner(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		provider := provider
		var portable string
		t.Run("export-"+provider, func(t *testing.T) {
			portable = buildPortableFixture(t, provider)
		})
		t.Run("concurrent-"+provider, func(t *testing.T) {
			dst := storetest.Open(t, provider)
			run := func() error {
				return Import(context.Background(), dst.DB(), dst.Dialect(), strings.NewReader(portable))
			}
			ready := make(chan struct{})
			var a, b error
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); <-ready; a = run() }()
			go func() { defer wg.Done(); <-ready; b = run() }()
			close(ready)
			wg.Wait()

			successes := 0
			for _, err := range []error{a, b} {
				if err == nil {
					successes++
				}
			}
			if successes > 1 {
				t.Fatalf("concurrent imports: %d succeeded (a=%v b=%v), want at most 1", successes, a, b)
			}

			// Deterministic end state: either the full archive is present, or
			// nothing changed. No partial merge is possible.
			var collections int
			if err := dst.DB().QueryRow("SELECT count(*) FROM _trestle_collections").Scan(&collections); err != nil {
				t.Fatal(err)
			}
			if successes == 1 && collections != 1 {
				t.Fatalf("winner left %d collections, want 1", collections)
			}
			if successes == 0 && collections != 0 {
				t.Fatalf("no winner but %d collections present (partial merge)", collections)
			}
		})
	}
}
