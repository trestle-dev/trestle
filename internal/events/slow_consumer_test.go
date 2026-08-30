package events

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/identities"
	"github.com/trestle-dev/trestle/internal/storetest"
)

// syncBuffer is a thread-safe response buffer for the healthy subscriber so
// the test can read it while the stream goroutine writes.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// syncWriter adapts a syncBuffer to http.ResponseWriter.
type syncWriter struct {
	header http.Header
	buf    *syncBuffer
}

func (w *syncWriter) Header() http.Header         { return w.header }
func (w *syncWriter) WriteHeader(int)             {}
func (w *syncWriter) Write(b []byte) (int, error) { return w.buf.Write(b) }
func (w *syncWriter) Flush()                      {}

// blockingWriter is a ResponseWriter whose first Write blocks until released,
// simulating a slow or stalled SSE consumer on its own connection.
type blockingWriter struct {
	header  http.Header
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Header() http.Header { return w.header }
func (w *blockingWriter) WriteHeader(int)     {}
func (w *blockingWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { <-w.release })
	return len(b), nil
}
func (w *blockingWriter) Flush() {}

// TestSlowConsumerDoesNotBlockOtherSubscribers proves a stalled SSE consumer
// blocks only its own connection: a healthy subscriber still receives committed
// events while the slow one is stuck.
func TestSlowConsumerDoesNotBlockOtherSubscribers(t *testing.T) {
	for _, provider := range storetest.Providers(t) {
		t.Run(provider, func(t *testing.T) {
			s := storetest.Open(t, provider)
			admin := adminauth.New(s.DB(), string(s.Provider()))
			h := New(s.DB(), admin, identities.New(s.DB(), admin))

			// Healthy subscriber on a synchronized buffer.
			healthy := &syncBuffer{}
			healthyReq := httptest.NewRequest("GET", "http://example.test/api/v1/realtime", nil)
			ctx, cancel := context.WithCancel(healthyReq.Context())
			healthyReq = healthyReq.WithContext(ctx)
			doneHealthy := make(chan struct{})
			go func() {
				defer close(doneHealthy)
				h.stream(&syncWriter{header: http.Header{}, buf: healthy}, healthyReq)
			}()
			defer cancel()

			// Stalled subscriber whose first write never completes.
			slow := &blockingWriter{header: http.Header{}, release: make(chan struct{})}
			slowReq := httptest.NewRequest("GET", "http://example.test/api/v1/realtime", nil)
			slowCtx, slowCancel := context.WithCancel(slowReq.Context())
			slowReq = slowReq.WithContext(slowCtx)
			defer slowCancel()
			doneSlow := make(chan struct{})
			go func() {
				defer close(doneSlow)
				h.stream(slow, slowReq)
			}()

			// Commit an event; the healthy subscriber must receive it while the
			// slow subscriber stays blocked.
			emitOne(t, h, 1)
			deadline := time.Now().Add(3 * time.Second)
			received := false
			for time.Now().Before(deadline) && !received {
				if strings.Contains(healthy.String(), "record.created") {
					received = true
				}
				time.Sleep(50 * time.Millisecond)
			}
			if !received {
				t.Fatalf("healthy subscriber did not receive the committed event; buffer=%q", healthy.String())
			}

			// The slow subscriber is still blocked (its goroutine is alive), and
			// releasing it lets the stream continue.
			select {
			case <-doneSlow:
				t.Fatal("slow subscriber finished despite a blocked write")
			default:
			}
			close(slow.release)
			slowCancel()
			select {
			case <-doneSlow:
			case <-time.After(2 * time.Second):
				t.Fatal("slow subscriber did not stop after release and cancel")
			}
		})
	}
}
