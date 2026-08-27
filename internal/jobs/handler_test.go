package jobs

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trestle-dev/trestle/internal/store"
)

func TestConcurrentClaimsExecuteEachJobOnce(t *testing.T) {
	dbStore, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dbStore.Close()
	db := dbStore.DB()
	h := New(db, nil)
	var executed atomic.Int64
	h.Register("probe", func(context.Context, json.RawMessage) error {
		executed.Add(1)
		return nil
	})
	for i := 0; i < 64; i++ {
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.Enqueue(context.Background(), tx, "probe", map[string]int{"index": i}, ""); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 12; i++ {
				h.runOne(context.Background())
			}
		}()
	}
	workers.Wait()
	if got := executed.Load(); got != 64 {
		t.Fatalf("executed %d jobs, want 64", got)
	}
	var incomplete int
	if err := db.QueryRow("SELECT count(*) FROM _trestle_jobs WHERE status!='succeeded' OR attempts!=1").Scan(&incomplete); err != nil || incomplete != 0 {
		t.Fatalf("incomplete or duplicate jobs: %d (%v)", incomplete, err)
	}
}

func TestExpiredLeaseRecoversAfterRestart(t *testing.T) {
	dbStore, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dbStore.Close()
	db := dbStore.DB()
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec("INSERT INTO _trestle_jobs(id,kind,payload_json,status,attempts,available_at,lease_until,created_at,updated_at) VALUES('interrupted','noop','{}','running',1,?,?,?,?)", past, past, now, now)
	if err != nil {
		t.Fatal(err)
	}
	h := New(db, nil)
	h.runOne(context.Background())
	var status string
	var attempts int
	if err := db.QueryRow("SELECT status,attempts FROM _trestle_jobs WHERE id='interrupted'").Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || attempts != 2 {
		t.Fatalf("recovered job = %s/%d, want succeeded/2", status, attempts)
	}
}
