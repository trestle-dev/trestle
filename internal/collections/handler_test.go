package collections

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/store"
)

type authSession struct {
	cookie *http.Cookie
	csrf   string
}

func setupTest(t *testing.T) (*Handler, authSession) {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	auth := adminauth.New(s.DB())
	body := bytes.NewBufferString(`{"email":"admin@example.com","password":"correct horse battery staple"}`)
	r := httptest.NewRequest("POST", "http://example.test/admin/v1/setup", body)
	r.Host = "example.test"
	r.Header.Set("Origin", "http://example.test")
	w := httptest.NewRecorder()
	auth.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return New(s.DB(), auth), authSession{w.Result().Cookies()[0], response.CSRF}
}
func call(t *testing.T, h http.Handler, s authSession, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		json.NewEncoder(&b).Encode(body)
	}
	r := httptest.NewRequest(method, "http://example.test"+path, &b)
	r.Host = "example.test"
	r.Header.Set("Origin", "http://example.test")
	r.Header.Set("X-Trestle-CSRF", s.csrf)
	if s.cookie != nil {
		r.AddCookie(s.cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCollectionMetadataLifecycle(t *testing.T) {
	h, s := setupTest(t)
	payload := input{Name: "issues", Fields: []Field{{Name: "title", Type: "text", Required: true}, {Name: "priority", Type: "number", Default: json.RawMessage(`1`)}}}
	w := call(t, h, s, "POST", "/admin/v1/collections", payload)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"issues"`) || !strings.Contains(w.Body.String(), `"title"`) {
		t.Fatal(w.Body.String())
	}
	var created Collection
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	titleID := created.Fields[0].ID
	w = call(t, h, s, "GET", "/admin/v1/collections", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "issues") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	payload.Name = "incidents"
	payload.Fields = append(payload.Fields, Field{Name: "open", Type: "boolean", Default: json.RawMessage(`true`)})
	w = call(t, h, s, "PATCH", "/admin/v1/collections/issues", payload)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "incidents") {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	var updated Collection
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Fields[0].ID != titleID {
		t.Fatalf("unchanged field ID changed: %s -> %s", titleID, updated.Fields[0].ID)
	}
	w = call(t, h, s, "GET", "/admin/v1/collections/incidents", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"open"`) {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	w = call(t, h, s, "DELETE", "/admin/v1/collections/incidents", nil)
	if w.Code != 204 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	w = call(t, h, s, "GET", "/admin/v1/collections/incidents", nil)
	if w.Code != 404 {
		t.Fatalf("deleted get: %d", w.Code)
	}
}
func TestCollectionValidationAndAuthorization(t *testing.T) {
	h, s := setupTest(t)
	bad := input{Name: "_system", Fields: []Field{{Name: "same", Type: "text"}, {Name: "same", Type: "shell"}}}
	w := call(t, h, s, "POST", "/admin/v1/collections", bad)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "duplicate") || !strings.Contains(w.Body.String(), "unsupported") {
		t.Fatalf("validation: %d %s", w.Code, w.Body.String())
	}
	w = call(t, h, authSession{}, "GET", "/admin/v1/collections", nil)
	if w.Code != 401 {
		t.Fatalf("unauthorized read: %d", w.Code)
	}
	s.csrf = "wrong"
	w = call(t, h, s, "POST", "/admin/v1/collections", input{Name: "notes"})
	if w.Code != 403 {
		t.Fatalf("csrf mutation: %d", w.Code)
	}
}

func TestDefaultMustMatchFieldType(t *testing.T) {
	h, s := setupTest(t)
	w := call(t, h, s, "POST", "/admin/v1/collections", input{Name: "events", Fields: []Field{{Name: "starts_at", Type: "datetime", Default: json.RawMessage(`"not-a-date"`)}, {Name: "count", Type: "number", Default: json.RawMessage(`"many"`)}}})
	if w.Code != 422 || strings.Count(w.Body.String(), "wrong_type") != 2 {
		t.Fatalf("default validation: %d %s", w.Code, w.Body.String())
	}
}
func TestDuplicateCollectionRollsBack(t *testing.T) {
	h, s := setupTest(t)
	payload := input{Name: "notes", Fields: []Field{{Name: "body", Type: "text"}}}
	if w := call(t, h, s, "POST", "/admin/v1/collections", payload); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	if w := call(t, h, s, "POST", "/admin/v1/collections", payload); w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}
	w := call(t, h, s, "GET", "/admin/v1/collections", nil)
	if strings.Count(w.Body.String(), `"name":"notes"`) != 1 {
		t.Fatal(w.Body.String())
	}
}
