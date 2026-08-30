package identities

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/httperr"
	"github.com/trestle-dev/trestle/internal/store"
)

type Handler struct {
	db    store.Executor
	admin *adminauth.Handler
	now   func() time.Time
}
type Principal struct {
	ID, Kind, Name string
	Scopes         map[string]bool
}
type createInput struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expiresAt,omitempty"`
}

var allowedScopes = map[string]bool{"records:read": true, "records:write": true, "files:read": true, "files:write": true}

func New(db any, admin *adminauth.Handler) *Handler {
	return &Handler{db: store.Adapt(db), admin: admin, now: time.Now}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.admin.Authorize(r, r.Method != http.MethodGet)
	if !ok {
		writeError(w, 403, "authorization_denied", "The request is not authorized.")
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/admin/v1/credentials":
		h.list(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/v1/credentials":
		h.create(w, r, principal.AdminID)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/admin/v1/credentials/"):
		h.revoke(w, r, strings.TrimPrefix(r.URL.Path, "/admin/v1/credentials/"))
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request, adminID string) {
	var in createInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || (in.Kind != "service" && in.Kind != "personal") || len(in.Scopes) == 0 {
		writeError(w, 422, "validation_failed", "The credential definition is invalid.")
		return
	}
	seen := map[string]bool{}
	for _, scope := range in.Scopes {
		if !allowedScopes[scope] || seen[scope] {
			writeError(w, 422, "validation_failed", "A credential scope is invalid.")
			return
		}
		seen[scope] = true
	}
	var expires any
	if in.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, in.ExpiresAt)
		if err != nil || !parsed.After(h.now()) {
			writeError(w, 422, "validation_failed", "The expiry is invalid.")
			return
		}
		expires = parsed.UTC().Format(time.RFC3339Nano)
	}
	raw := "tr_" + token(32)
	sum := sha256.Sum256([]byte(raw))
	id := "cred_" + token(15)
	owner := any(nil)
	if in.Kind == "personal" {
		owner = adminID
	}
	now := h.now().UTC().Format(time.RFC3339Nano)
	_, err := h.db.ExecContext(r.Context(), "INSERT INTO _trestle_credentials(id,kind,name,owner_admin_id,secret_hash,scopes,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?)", id, in.Kind, in.Name, owner, sum[:], strings.Join(in.Scopes, ","), now, expires)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "kind": in.Kind, "name": in.Name, "scopes": in.Scopes, "secret": raw, "createdAt": now, "expiresAt": expires})
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), "SELECT id,kind,name,scopes,created_at,expires_at,revoked_at,last_used_at FROM _trestle_credentials ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, kind, name, scopes, created string
		var expires, revoked, last sql.NullString
		rows.Scan(&id, &kind, &name, &scopes, &created, &expires, &revoked, &last)
		items = append(items, map[string]any{"id": id, "kind": kind, "name": name, "scopes": strings.Split(scopes, ","), "createdAt": created, "expiresAt": nullable(expires), "revoked": revoked.Valid, "lastUsedAt": nullable(last)})
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (h *Handler) revoke(w http.ResponseWriter, r *http.Request, id string) {
	result, err := h.db.ExecContext(r.Context(), "UPDATE _trestle_credentials SET revoked_at=? WHERE id=? AND revoked_at IS NULL", h.now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		writeError(w, 404, "credential_not_found", "The credential was not found.")
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) Authenticate(r *http.Request, scope string) (Principal, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return Principal{}, false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	sum := sha256.Sum256([]byte(raw))
	var p Principal
	var scopes, expires string
	var expiry, revoked sql.NullString
	err := h.db.QueryRowContext(r.Context(), "SELECT id,kind,name,scopes,coalesce(expires_at,''),expires_at,revoked_at FROM _trestle_credentials WHERE secret_hash=?", sum[:]).Scan(&p.ID, &p.Kind, &p.Name, &scopes, &expires, &expiry, &revoked)
	if err != nil || revoked.Valid {
		return Principal{}, false
	}
	if expiry.Valid {
		parsed, _ := time.Parse(time.RFC3339Nano, expires)
		if !parsed.After(h.now()) {
			return Principal{}, false
		}
	}
	p.Scopes = map[string]bool{}
	for _, item := range strings.Split(scopes, ",") {
		p.Scopes[item] = true
	}
	if !p.Scopes[scope] {
		return Principal{}, false
	}
	now := h.now().UTC().Format(time.RFC3339Nano)
	h.db.ExecContext(r.Context(), "UPDATE _trestle_credentials SET last_used_at=? WHERE id=?", now, p.ID)
	h.db.ExecContext(r.Context(), "INSERT INTO _trestle_audit(occurred_at,actor_kind,actor_id,action,target,outcome,request_id) VALUES(?,?,?,?,?,?,?)", now, p.Kind, p.ID, "credential.use", scope, "allowed", r.Header.Get("X-Request-ID"))
	return p, true
}
func token(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func nullable(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		writeError(w, 400, "invalid_json", "The request body is invalid.")
		return false
	}
	return true
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, httperr.New(code, message, w.Header().Get("X-Request-ID")))
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
