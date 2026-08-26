package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReadinessAndRequestID(t *testing.T) {
	var logs bytes.Buffer
	s := New(slog.New(slog.NewJSONHandler(&logs, nil)))
	for _, test := range []struct {
		path   string
		status int
	}{{"/system/health", 200}, {"/system/ready", 503}} {
		r := httptest.NewRequest(http.MethodGet, test.path, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s: got %d", test.path, w.Code)
		}
		if w.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing request ID")
		}
	}
	s.SetReady(true)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/system/ready", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "ready") {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(logs.String(), "Authorization") {
		t.Fatal("sensitive header logged")
	}
}
