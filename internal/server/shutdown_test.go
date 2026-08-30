package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestGracefulShutdownDrainsInFlightRequest proves a Shutdown call lets an
// in-flight request complete and returns within the drain deadline.
func TestGracefulShutdownDrainsInFlightRequest(t *testing.T) {
	app := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// A slow route is not part of the mux, so wrap with a handler that serves it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			slow.ServeHTTP(w, r)
			return
		}
		app.Handler().ServeHTTP(w, r)
	})}
	go srv.Serve(listener)
	defer srv.Close()

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, httptest.NewRequest("GET", "http://"+listener.Addr().String()+"/slow", nil))
		done <- w
	}()
	time.Sleep(50 * time.Millisecond) // let the slow request begin

	start := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("shutdown took %s, want within the 2s deadline", time.Since(start))
	}
	select {
	case w := <-done:
		if w.Code != http.StatusNoContent {
			t.Fatalf("in-flight request completed with %d", w.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request did not drain")
	}
}
