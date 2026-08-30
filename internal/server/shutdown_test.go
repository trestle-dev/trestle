package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// TestGracefulShutdownDrainsTrackedRequest proves Shutdown waits for an
// in-flight request tracked through the real HTTP listener: a request is sent
// over an HTTP client, the handler signals it started, Shutdown is called, the
// handler is then released and completes, the client receives the response,
// and Shutdown does not return before the handler finished.
func TestGracefulShutdownDrainsTrackedRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var finished atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		finished.Store(time.Now().UnixNano())
		w.WriteHeader(http.StatusNoContent)
	})
	srv := &http.Server{Handler: handler}
	go srv.Serve(listener)
	defer srv.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	type result struct {
		status int
		err    error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := client.Get("http://" + listener.Addr().String() + "/")
		if err != nil {
			done <- result{0, err}
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		done <- result{resp.StatusCode, nil}
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("handler never started")
	}

	shutdownDone := make(chan time.Time, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
		shutdownDone <- time.Now()
	}()

	// Shutdown must not return before the tracked request finishes.
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned while the in-flight request was still blocked")
	case <-time.After(300 * time.Millisecond):
	}

	// Release the handler; the client receives its response and Shutdown
	// returns only after the handler finished.
	close(release)
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("client request error: %v", res.err)
		}
		if res.status != http.StatusNoContent {
			t.Fatalf("client status %d, want 204", res.status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client never received a response")
	}
	select {
	case shutdownAt := <-shutdownDone:
		if shutdownAt.UnixNano() < finished.Load() {
			t.Fatal("Shutdown returned before the tracked request finished")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown never returned after the request finished")
	}
}
