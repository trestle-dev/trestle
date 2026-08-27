package records

import (
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedConcurrentRecordReadLoad(t *testing.T) {
	h, session := setup(t)
	for i := 0; i < 100; i++ {
		response := invoke(t, h, session, http.MethodPost, "/api/v1/collections/issues/records", map[string]any{"values": map[string]any{"title": "load-" + strconv.Itoa(i)}}, nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("seed %d: %d %s", i, response.Code, response.Body.String())
		}
	}
	runtime.GC()
	baselineGoroutines := runtime.NumGoroutine()
	started := time.Now()
	var failures atomic.Int64
	var requests atomic.Int64
	var workers sync.WaitGroup
	for worker := 0; worker < 24; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := 0; iteration < 50; iteration++ {
				response := invoke(t, h, session, http.MethodGet, "/api/v1/collections/issues/records?limit=25", nil, nil)
				requests.Add(1)
				if response.Code != http.StatusOK || response.Body.Len() > 64<<10 {
					failures.Add(1)
				}
			}
		}(worker)
	}
	workers.Wait()
	if requests.Load() != 1200 || failures.Load() != 0 {
		t.Fatalf("requests=%d failures=%d", requests.Load(), failures.Load())
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Fatalf("bounded load took %s", elapsed)
	}
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > baselineGoroutines+8 {
		t.Fatalf("goroutines grew from %d to %d", baselineGoroutines, after)
	}
}

func TestRecordResourceLimitsFailControlled(t *testing.T) {
	h, session := setup(t)
	records := make([]map[string]any, 1001)
	for i := range records {
		records[i] = map[string]any{"values": map[string]any{"title": "bounded"}}
	}
	response := invoke(t, h, session, http.MethodPost, "/api/v1/collections/issues/records/batch", map[string]any{"records": records}, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized batch: %d %s", response.Code, response.Body.String())
	}
	response = invoke(t, h, session, http.MethodGet, "/api/v1/collections/issues/records?limit=1000000", nil, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized page: %d %s", response.Code, response.Body.String())
	}
}
