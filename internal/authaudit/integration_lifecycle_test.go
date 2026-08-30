package authaudit

import (
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/functions"
	"github.com/trestle-dev/trestle/internal/jobs"
	"github.com/trestle-dev/trestle/internal/storetest"
	"github.com/trestle-dev/trestle/internal/webhooks"
)

// TestWebhookFunctionTargetLifecycle exercises webhook and function target
// create, list, enable and disable on both providers, proving the dialect-owned
// boolean handling and lifecycle parity claimed by the parity matrix.
func TestWebhookFunctionTargetLifecycle(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			sess := setupAdmin(t, admin)
			queue := jobs.New(s.DB(), admin)

			wh := mustWebhooks(webhooks.New(s.DB(), admin, queue, t.TempDir()))
			created := do(wh, sess, "POST", "/admin/v1/webhooks", map[string]any{"name": "h", "url": "https://receiver.invalid/x", "topics": []string{"record.created"}})
			if created.Code != 201 {
				t.Fatalf("webhook create %d %s", created.Code, created.Body.String())
			}
			var whID string
			s.DB().QueryRow("SELECT id FROM _trestle_webhooks LIMIT 1").Scan(&whID)
			if list := do(wh, sess, "GET", "/admin/v1/webhooks", nil); list.Code != 200 || !strings.Contains(list.Body.String(), whID) {
				t.Fatalf("webhook list %d %s", list.Code, list.Body.String())
			}
			if w := do(wh, sess, "POST", "/admin/v1/webhooks/"+whID, map[string]any{"action": "disable"}); w.Code != 204 {
				t.Fatalf("webhook disable %d", w.Code)
			}
			var enabled any
			s.DB().QueryRow("SELECT enabled FROM _trestle_webhooks WHERE id=?", whID).Scan(&enabled)
			if on, _ := s.Dialect().DecodeBoolean(enabled); on {
				t.Fatal("webhook still enabled after disable")
			}
			if w := do(wh, sess, "POST", "/admin/v1/webhooks/"+whID, map[string]any{"action": "enable"}); w.Code != 204 {
				t.Fatalf("webhook enable %d", w.Code)
			}

			fn := functions.New(s.DB(), admin, queue, functions.Options{})
			created = do(fn, sess, "POST", "/admin/v1/functions", map[string]any{"name": "fn", "target": "arn:aws:lambda:us-east-1:1:function:x", "region": "us-east-1", "topics": []string{"record.created"}})
			if created.Code != 201 {
				t.Fatalf("function create %d %s", created.Code, created.Body.String())
			}
			var fnID string
			s.DB().QueryRow("SELECT id FROM _trestle_functions LIMIT 1").Scan(&fnID)
			if list := do(fn, sess, "GET", "/admin/v1/functions", nil); list.Code != 200 || !strings.Contains(list.Body.String(), fnID) {
				t.Fatalf("function list %d %s", list.Code, list.Body.String())
			}
			if w := do(fn, sess, "POST", "/admin/v1/functions/"+fnID, map[string]any{"action": "disable"}); w.Code != 204 {
				t.Fatalf("function disable %d", w.Code)
			}
			var fnEnabled any
			s.DB().QueryRow("SELECT enabled FROM _trestle_functions WHERE id=?", fnID).Scan(&fnEnabled)
			if on, _ := s.Dialect().DecodeBoolean(fnEnabled); on {
				t.Fatal("function still enabled after disable")
			}
			if w := do(fn, sess, "POST", "/admin/v1/functions/"+fnID, map[string]any{"action": "enable"}); w.Code != 204 {
				t.Fatalf("function enable %d", w.Code)
			}
		})
	}
}
