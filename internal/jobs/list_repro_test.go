package jobs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trestle-dev/trestle/internal/storetest"
)

// TestJobListFieldsRoundTrip is the regression test for the jobs-list scan:
// payload_json must be scanned as a string and converted to RawMessage, because
// database/sql cannot scan a string driver value directly into
// *json.RawMessage on this driver (which previously lost every job field except
// id and kind).
func TestJobListFieldsRoundTrip(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			h := New(s.DB(), nil)
			tx, err := s.DB().BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.Enqueue(context.Background(), tx, "webhook", map[string]any{"a": 1}, ""); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			query := "SELECT id,kind,payload_json,status,attempts,max_attempts,available_at,coalesce(lease_until,''),coalesce(last_error,''),created_at,updated_at FROM _trestle_jobs ORDER BY created_at DESC LIMIT 200"
			rows, err := s.DB().QueryContext(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var items []Job
			for rows.Next() {
				var j Job
				var payload string
				if err := rows.Scan(&j.ID, &j.Kind, &payload, &j.Status, &j.Attempts, &j.MaxAttempts, &j.AvailableAt, &j.LeaseUntil, &j.LastError, &j.CreatedAt, &j.UpdatedAt); err != nil {
					t.Fatal(err)
				}
				j.Payload = json.RawMessage(payload)
				items = append(items, j)
			}
			if len(items) != 1 {
				t.Fatalf("items=%d", len(items))
			}
			if items[0].Status != "pending" || items[0].Payload == nil || string(items[0].Payload) == "" {
				raw, _ := json.Marshal(items[0])
				t.Fatalf("list round-trip lost fields: %s", raw)
			}
		})
	}
}
