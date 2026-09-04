package rules

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/trestle-cv/trestle/internal/adminauth"
	"github.com/trestle-cv/trestle/internal/httperr"
	"github.com/trestle-cv/trestle/internal/store"
	"net/http"
	"strings"
	"time"
)

type Actor struct{ ID, Kind string }
type Handler struct {
	db    store.Executor
	admin *adminauth.Handler
	now   func() time.Time
}

var operations = map[string]bool{"list": true, "view": true, "create": true, "update": true, "delete": true}

func New(db any, admin *adminauth.Handler) *Handler {
	return &Handler{db: store.Adapt(db), admin: admin, now: time.Now}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.admin.Authorize(r, r.Method != http.MethodGet); !ok {
		writeError(w, 403, "authorization_denied", "The request is not authorized.")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/admin/v1/collection-rules/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	collection := parts[0]
	switch {
	case r.Method == http.MethodGet && len(parts) == 1:
		h.get(w, r, collection)
	case r.Method == http.MethodPut && len(parts) == 1:
		h.put(w, r, collection)
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "explain":
		h.explain(w, r, collection)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request, name string) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT operation,expression FROM _trestle_collection_rules WHERE collection_id=(SELECT id FROM _trestle_collections WHERE name=?)`, name)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var op, expr string
		rows.Scan(&op, &expr)
		values[op] = expr
	}
	writeJSON(w, 200, map[string]any{"rules": values})
}
func (h *Handler) put(w http.ResponseWriter, r *http.Request, name string) {
	var in struct {
		Rules map[string]string `json:"rules"`
	}
	if !decode(w, r, &in) {
		return
	}
	for op, expr := range in.Rules {
		if !operations[op] || Validate(expr) != nil {
			writeError(w, 422, "invalid_rule", "A rule is invalid.")
			return
		}
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	defer tx.Rollback()
	var id string
	if tx.QueryRowContext(r.Context(), "SELECT id FROM _trestle_collections WHERE name=?", name).Scan(&id) != nil {
		writeError(w, 404, "collection_not_found", "The collection was not found.")
		return
	}
	if _, err := tx.ExecContext(r.Context(), "DELETE FROM _trestle_collection_rules WHERE collection_id=?", id); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	now := h.now().UTC().Format(time.RFC3339Nano)
	for op, expr := range in.Rules {
		if _, err := tx.ExecContext(r.Context(), "INSERT INTO _trestle_collection_rules(collection_id,operation,expression,updated_at) VALUES(?,?,?,?)", id, op, strings.TrimSpace(expr), now); err != nil {
			writeError(w, 500, "internal_error", "The request could not be completed.")
			return
		}
	}
	if tx.Commit() != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	writeJSON(w, 200, map[string]any{"rules": in.Rules})
}
func (h *Handler) explain(w http.ResponseWriter, r *http.Request, name string) {
	var in struct {
		Operation string         `json:"operation"`
		Actor     Actor          `json:"actor"`
		Values    map[string]any `json:"values"`
	}
	if !decode(w, r, &in) {
		return
	}
	allowed, expr, err := h.Allowed(r, name, in.Operation, in.Actor, in.Values)
	if err != nil {
		writeError(w, 400, "rule_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"allowed": allowed, "expression": expr, "inputs": map[string]any{"actorKind": in.Actor.Kind, "actorId": redact(in.Actor.ID), "fieldNames": keys(in.Values)}, "notice": "Simulation does not replace server enforcement."})
}
func (h *Handler) Allowed(r *http.Request, collection, operation string, actor Actor, values map[string]any) (bool, string, error) {
	if !operations[operation] {
		return false, "", errors.New("unknown operation")
	}
	var expr string
	err := h.db.QueryRowContext(r.Context(), `SELECT expression FROM _trestle_collection_rules WHERE collection_id=(SELECT id FROM _trestle_collections WHERE name=?) AND operation=?`, collection, operation).Scan(&expr)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return Evaluate(expr, actor, values), expr, nil
}
func Validate(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "true" || expr == "false" || expr == `actor.kind == "user"` || expr == `actor.kind == "service"` {
		return nil
	}
	for _, prefix := range []string{"actor.id == record.", "actor.id == input."} {
		if strings.HasPrefix(expr, prefix) {
			field := strings.TrimPrefix(expr, prefix)
			if field != "" && len(field) <= 63 && strings.IndexFunc(field, func(r rune) bool { return !(r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) < 0 {
				return nil
			}
		}
	}
	return errors.New("unsupported rule expression")
}
func Evaluate(expr string, actor Actor, values map[string]any) bool {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "true":
		return true
	case "false":
		return false
	case `actor.kind == "user"`:
		return actor.Kind == "user"
	case `actor.kind == "service"`:
		return actor.Kind == "service"
	}
	for _, prefix := range []string{"actor.id == record.", "actor.id == input."} {
		if strings.HasPrefix(expr, prefix) {
			value, ok := values[strings.TrimPrefix(expr, prefix)].(string)
			return ok && value == actor.ID
		}
	}
	return false
}
func redact(value string) string {
	if len(value) < 6 {
		return "***"
	}
	return value[:3] + "…" + value[len(value)-2:]
}
func keys(values map[string]any) []string {
	result := []string{}
	for key := range values {
		result = append(result, key)
	}
	return result
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
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
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
