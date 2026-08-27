package databasesetup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/config"
	"github.com/trestle-dev/trestle/internal/store"
)

func TestFirstRunDatabaseSelectionPersistsSQLite(t *testing.T) {
	dir := t.TempDir()
	database, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := New(adminauth.New(database.DB()), Options{DataDir: dir, Current: store.SQLite, MaxOpen: 10, MaxIdle: 2, ConnectTimeout: time.Second, ConnMaxLifetime: time.Hour})
	r := httptest.NewRequest(http.MethodPost, "/admin/v1/database/setup", strings.NewReader(`{"provider":"sqlite","url":""}`))
	r.Host = "example.test"
	r.Header.Set("Origin", "http://example.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	stored, found, err := config.ReadDatabaseBootstrap(dir)
	if err != nil || !found || stored.Provider != "sqlite" {
		t.Fatalf("stored=%#v found=%v err=%v", stored, found, err)
	}
}

func TestExplicitAndCompletedSetupRefuseMutation(t *testing.T) {
	dir := t.TempDir()
	database, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	admin := adminauth.New(database.DB())
	h := New(admin, Options{DataDir: dir, Current: store.SQLite, Explicit: true, ConnectTimeout: time.Second})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/v1/database/setup", strings.NewReader(`{"provider":"sqlite"}`)))
	if w.Code != 409 {
		t.Fatalf("explicit status=%d", w.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/setup", strings.NewReader(`{"email":"admin@example.com","password":"mudblood"}`))
	request.Host = "example.test"
	response := httptest.NewRecorder()
	admin.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/v1/database/setup", nil))
	if w.Code != 409 {
		var body any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		t.Fatalf("completed status=%d body=%v", w.Code, body)
	}
}
