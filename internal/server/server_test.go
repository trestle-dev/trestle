package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/requestmeta"
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

func TestForwardedHeadersRequireTrustedImmediatePeer(t *testing.T) {
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(requestmeta.Scheme(r) + " " + requestmeta.ClientIP(r)))
	})
	s := NewWithOptions(slog.Default(), echo, nil, nil, Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}})

	untrusted := httptest.NewRequest(http.MethodGet, "/", nil)
	untrusted.RemoteAddr = "198.51.100.10:1234"
	untrusted.Header.Set("X-Forwarded-Proto", "https")
	untrusted.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, untrusted)
	if got := strings.TrimSpace(w.Body.String()); got != "http 198.51.100.10" {
		t.Fatalf("untrusted forwarding accepted: %q", got)
	}

	trusted := httptest.NewRequest(http.MethodGet, "/", nil)
	trusted.RemoteAddr = "10.1.2.3:1234"
	trusted.Header.Set("X-Forwarded-Proto", "https")
	trusted.Header.Set("X-Forwarded-For", "203.0.113.7")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, trusted)
	if got := strings.TrimSpace(w.Body.String()); got != "https 203.0.113.7" {
		t.Fatalf("trusted forwarding ignored: %q", got)
	}
}

func TestForwardedChainStopsAtFirstUntrustedHop(t *testing.T) {
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(requestmeta.ClientIP(r))) })
	s := NewWithOptions(slog.Default(), echo, nil, nil, Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-For", "192.0.2.50, 198.51.100.9, 10.2.3.4")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if got := strings.TrimSpace(w.Body.String()); got != "198.51.100.9" {
		t.Fatalf("unexpected client: %q", got)
	}
}
