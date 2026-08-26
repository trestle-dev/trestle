package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedDashboardAndSPAFallback(t *testing.T) {
	h, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/", "/collections/example", "/assets/css/style.css"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
		if w.Code != 200 {
			t.Fatalf("%s returned %d", route, w.Code)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
}

func TestStaticOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("override-marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), "override-marker") {
		t.Fatal("override not served")
	}
}

func TestMissingAssetDoesNotFallback(t *testing.T) {
	h, _ := New("")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing.js", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d", w.Code)
	}
}
