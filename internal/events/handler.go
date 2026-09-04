package events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trestle-cv/trestle/internal/adminauth"
	"github.com/trestle-cv/trestle/internal/identities"
	"github.com/trestle-cv/trestle/internal/store"
)

type Dispatcher interface {
	Dispatch(context.Context, store.Transaction, string, string, string, any) error
}
type Handler struct {
	db          store.Executor
	admin       *adminauth.Handler
	credentials *identities.Handler
	now         func() time.Time
	dispatchers []Dispatcher
}

func (h *Handler) ConfigureDispatcher(dispatcher Dispatcher) {
	h.dispatchers = append(h.dispatchers, dispatcher)
}

type Event struct {
	Sequence   int64  `json:"sequence"`
	OccurredAt string `json:"occurredAt"`
	Topic      string `json:"topic"`
	Collection string `json:"collection,omitempty"`
	RecordID   string `json:"recordId,omitempty"`
	Payload    any    `json:"payload"`
}

func New(db any, admin *adminauth.Handler, credentials *identities.Handler) *Handler {
	return &Handler{db: store.Adapt(db), admin: admin, credentials: credentials, now: time.Now}
}
func (h *Handler) Emit(ctx context.Context, tx store.Transaction, topic, collection, recordID string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO _trestle_events(occurred_at,topic,collection_name,record_id,payload_json) VALUES(?,?,?,?,?)", h.now().UTC().Format(time.RFC3339Nano), topic, null(collection), null(recordID), string(encoded))
	for _, dispatcher := range h.dispatchers {
		if err == nil {
			err = dispatcher.Dispatch(ctx, tx, topic, collection, recordID, payload)
		}
	}
	return err
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.admin.Authorize(r, false); !ok {
		if _, ok = h.credentials.Authenticate(r, "records:read"); !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	if r.URL.Path == "/admin/v1/events" {
		h.list(w, r)
		return
	}
	if r.URL.Path != "/api/v1/realtime" {
		http.NotFound(w, r)
		return
	}
	h.stream(w, r)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	items, _ := h.read(r.Context(), after, 200, r.URL.Query().Get("topic"))
	writeJSON(w, map[string]any{"items": items})
}
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	heartbeat := time.NewTicker(15 * time.Second)
	poll := time.NewTicker(500 * time.Millisecond)
	defer heartbeat.Stop()
	defer poll.Stop()
	send := func() {
		items, _ := h.read(r.Context(), after, 100, r.URL.Query().Get("topic"))
		for _, event := range items {
			encoded, _ := json.Marshal(event)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Topic, encoded)
			after = event.Sequence
		}
		flusher.Flush()
	}
	// Send an observable frame immediately, even when the journal is empty.
	// Without it some browsers remain in "Connecting" until the first 15-second
	// heartbeat because Flush has no bytes to deliver.
	writeReady(w)
	flusher.Flush()
	send()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			send()
		case <-heartbeat.C:
			writeHeartbeat(w)
			flusher.Flush()
		}
	}
}
func (h *Handler) read(ctx context.Context, after int64, limit int, topic string) ([]Event, error) {
	query := "SELECT sequence,occurred_at,topic,coalesce(collection_name,''),coalesce(record_id,''),payload_json FROM _trestle_events WHERE sequence>?"
	args := []any{after}
	if strings.TrimSpace(topic) != "" {
		query += " AND topic=?"
		args = append(args, topic)
	}
	query += " ORDER BY sequence LIMIT ?"
	args = append(args, limit)
	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var e Event
		var payload string
		if err = rows.Scan(&e.Sequence, &e.OccurredAt, &e.Topic, &e.Collection, &e.RecordID, &payload); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(payload), &e.Payload)
		items = append(items, e)
	}
	return items, rows.Err()
}
func null(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

// writeHeartbeat emits an observable SSE heartbeat event so clients can
// distinguish a healthy idle transport from a genuinely stale one. Browser
// EventSource exposes named events to JavaScript (unlike comment frames), so a
// client can refresh its activity time without adding an item to the UI.
func writeHeartbeat(w io.Writer) {
	fmt.Fprint(w, "event: heartbeat\ndata: {}\n\n")
}

func writeReady(w io.Writer) {
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
}
