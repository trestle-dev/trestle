package backup

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/trestle-cv/trestle/internal/store"
	"github.com/trestle-cv/trestle/internal/storetest"
)

// dataTableCount returns the number of physical collection tables present.
func dataTableCount(t *testing.T, db store.Executor) int {
	t.Helper()
	var count int
	var err error
	if db.Dialect().Provider() == store.Postgres {
		err = db.QueryRow("SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename LIKE '_trestle_data_%'").Scan(&count)
	} else {
		err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name LIKE '_trestle_data_%'").Scan(&count)
	}
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// TestConcurrentImportSingleWinner proves two concurrent imports into one empty
// initialized destination never merge partially. A canonical semantic digest
// covers every portable-owned table and physical record table: for exactly one
// winner the destination digest must equal the source archive digest; for zero
// winners it must equal the pre-import destination digest. No temporary or
// partially created physical tables may remain in either case.
func TestConcurrentImportSingleWinner(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		provider := provider
		var portable string
		var srcDigest string
		t.Run("export-"+provider, func(t *testing.T) {
			portable = buildPortableFixture(t, provider)
			d, err := SemanticDigest([]byte(portable))
			if err != nil {
				t.Fatal(err)
			}
			srcDigest = d
		})
		t.Run("concurrent-"+provider, func(t *testing.T) {
			dst := storetest.Open(t, provider)

			// Pre-import digest of the empty initialized destination.
			var emptyBuf bytes.Buffer
			if err := Export(context.Background(), dst.DB(), dst.Dialect(), &emptyBuf); err != nil {
				t.Fatal(err)
			}
			emptyDigest, err := SemanticDigest(emptyBuf.Bytes())
			if err != nil {
				t.Fatal(err)
			}

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

			// Re-export the destination and compare canonical semantic digests.
			var finalBuf bytes.Buffer
			if err := Export(context.Background(), dst.DB(), dst.Dialect(), &finalBuf); err != nil {
				t.Fatal(err)
			}
			finalDigest, err := SemanticDigest(finalBuf.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			var collections int
			if err := dst.DB().QueryRow("SELECT count(*) FROM _trestle_collections").Scan(&collections); err != nil {
				t.Fatal(err)
			}
			tables := dataTableCount(t, dst.DB())

			switch successes {
			case 1:
				if finalDigest != srcDigest {
					t.Fatalf("winner destination digest %s != source archive digest %s", finalDigest, srcDigest)
				}
			case 0:
				if finalDigest != emptyDigest {
					t.Fatalf("no winner but destination changed (partial merge): digest %s != empty %s", finalDigest, emptyDigest)
				}
			}
			if tables != collections {
				t.Fatalf("physical data tables=%d but collections=%d (temporary or partially created tables remain)", tables, collections)
			}
		})
	}
}
