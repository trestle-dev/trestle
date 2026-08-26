package records

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/collections"
	"github.com/trestle-dev/trestle/internal/store"
)

type session struct {
	cookie *http.Cookie
	csrf   string
}

func setup(t *testing.T) (*Handler, session) {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	auth := adminauth.New(s.DB())
	w := invoke(t, auth, session{}, "POST", "/admin/v1/setup", map[string]any{"email": "admin@example.com", "password": "correct horse battery staple"}, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var login struct {
		CSRF string `json:"csrfToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &login)
	sess := session{w.Result().Cookies()[0], login.CSRF}
	schemas := collections.New(s.DB(), auth)
	w = invoke(t, schemas, sess, "POST", "/admin/v1/collections", map[string]any{"name": "issues", "fields": []map[string]any{{"name": "title", "type": "text", "required": true}, {"name": "done", "type": "boolean", "default": false}, {"name": "score", "type": "number"}}}, nil)
	if w.Code != 201 {
		t.Fatalf("schema: %d %s", w.Code, w.Body.String())
	}
	return New(s.DB(), auth), sess
}
func invoke(t *testing.T, h http.Handler, s session, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		json.NewEncoder(&b).Encode(body)
	}
	r := httptest.NewRequest(method, "http://example.test"+path, &b)
	r.Host = "example.test"
	r.Header.Set("Origin", "http://example.test")
	r.Header.Set("X-Trestle-CSRF", s.csrf)
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	if s.cookie != nil {
		r.AddCookie(s.cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRecordCRUDAndVersions(t *testing.T) {
	h, s := setup(t)
	w := invoke(t, h, s, "POST", "/api/v1/collections/issues/records", map[string]any{"values": map[string]any{"title": "first", "score": 2}}, nil)
	if w.Code != 201 || w.Header().Get("ETag") != "\"1\"" {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created Record
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Values["done"] != false {
		t.Fatalf("default missing: %#v", created)
	}
	w = invoke(t, h, s, "GET", "/api/v1/collections/issues/records/"+created.ID, nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = invoke(t, h, s, "PATCH", "/api/v1/collections/issues/records/"+created.ID, map[string]any{"values": map[string]any{"done": true}}, map[string]string{"If-Match": "\"1\""})
	if w.Code != 200 || w.Header().Get("ETag") != "\"2\"" {
		t.Fatalf("update %d %s", w.Code, w.Body.String())
	}
	w = invoke(t, h, s, "PATCH", "/api/v1/collections/issues/records/"+created.ID, map[string]any{"values": map[string]any{"title": "stale"}}, map[string]string{"If-Match": "\"1\""})
	if w.Code != 412 {
		t.Fatalf("stale %d", w.Code)
	}
	w = invoke(t, h, s, "DELETE", "/api/v1/collections/issues/records/"+created.ID, nil, map[string]string{"If-Match": "\"2\""})
	if w.Code != 204 {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
}
func TestValidationAndAtomicBatch(t *testing.T) {
	h, s := setup(t)
	w := invoke(t, h, s, "POST", "/api/v1/collections/issues/records", map[string]any{"values": map[string]any{"done": "yes", "unknown": 1}}, nil)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "wrong_type") || !strings.Contains(w.Body.String(), "unknown") {
		t.Fatalf("validation %d %s", w.Code, w.Body.String())
	}
	w = invoke(t, h, s, "POST", "/api/v1/collections/issues/records/batch", map[string]any{"records": []map[string]any{{"id": "rec_same", "values": map[string]any{"title": "one"}}, {"id": "rec_same", "values": map[string]any{"title": "two"}}}}, nil)
	if w.Code != 409 {
		t.Fatalf("batch conflict %d %s", w.Code, w.Body.String())
	}
	w = invoke(t, h, s, "GET", "/api/v1/collections/issues/records", nil, nil)
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("batch was not atomic: %s", w.Body.String())
	}
}
func TestMutationNeedsVersionAndCSRF(t *testing.T) {
	h, s := setup(t)
	w := invoke(t, h, s, "POST", "/api/v1/collections/issues/records", map[string]any{"values": map[string]any{"title": "first"}}, nil)
	var created Record
	json.Unmarshal(w.Body.Bytes(), &created)
	w = invoke(t, h, s, "PATCH", "/api/v1/collections/issues/records/"+created.ID, map[string]any{"values": map[string]any{"title": "next"}}, nil)
	if w.Code != 428 {
		t.Fatalf("missing version %d", w.Code)
	}
	s.csrf = "bad"
	w = invoke(t, h, s, "DELETE", "/api/v1/collections/issues/records/"+created.ID, nil, map[string]string{"If-Match": "1"})
	if w.Code != 403 {
		t.Fatalf("csrf %d", w.Code)
	}
}

func TestIdempotentCreateProjectionAndBounds(t *testing.T) {
	h, s := setup(t)
	headers := map[string]string{"Idempotency-Key": "request-17"}
	w := invoke(t, h, s, "POST", "/api/v1/collections/issues/records?fields=title", map[string]any{"values": map[string]any{"title": "same", "score": 4}}, headers)
	if w.Code != 201 || strings.Contains(w.Body.String(), `"score"`) {
		t.Fatalf("first create %d %s", w.Code, w.Body.String())
	}
	var first Record
	json.Unmarshal(w.Body.Bytes(), &first)
	w = invoke(t, h, s, "POST", "/api/v1/collections/issues/records?fields=title", map[string]any{"values": map[string]any{"title": "ignored"}}, headers)
	var replay Record
	json.Unmarshal(w.Body.Bytes(), &replay)
	if w.Code != 200 || w.Header().Get("Idempotency-Replayed") != "true" || replay.ID != first.ID {
		t.Fatalf("replay %d %s", w.Code, w.Body.String())
	}
	w = invoke(t, h, s, "GET", "/api/v1/collections/issues/records?limit=101", nil, nil)
	if w.Code != 400 {
		t.Fatalf("limit %d %s", w.Code, w.Body.String())
	}
}
