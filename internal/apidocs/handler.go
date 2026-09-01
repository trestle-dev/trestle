package apidocs

import (
	"database/sql"
	"encoding/json"
	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/buildinfo"
	"github.com/trestle-dev/trestle/internal/files"
	"github.com/trestle-dev/trestle/internal/store"
	"net/http"
)

type Handler struct {
	db    store.Executor
	admin *adminauth.Handler
}

func New(db any, admin *adminauth.Handler) *Handler {
	return &Handler{db: store.Adapt(db), admin: admin}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/openapi.json":
		write(w, openAPI())
	case "/api/v1/capabilities":
		write(w, capabilities())
	case "/admin/v1/api/schema":
		if _, ok := h.admin.Authorize(r, false); !ok {
			http.Error(w, "forbidden", 403)
			return
		}
		h.schema(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) schema(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), "SELECT c.name,f.name,f.type,f.required,f.is_unique FROM _trestle_collections c LEFT JOIN _trestle_fields f ON f.collection_id=c.id ORDER BY c.name,f.position")
	if err != nil {
		http.Error(w, "schema unavailable", 500)
		return
	}
	defer rows.Close()
	dialect := h.db.Dialect()
	collections := map[string][]map[string]any{}
	for rows.Next() {
		var collection string
		var name, kind sql.NullString
		var requiredRaw, uniqueRaw any
		if err := rows.Scan(&collection, &name, &kind, &requiredRaw, &uniqueRaw); err != nil {
			http.Error(w, "schema unavailable", 500)
			return
		}
		required, err := dialect.DecodeBoolean(requiredRaw)
		if err != nil {
			http.Error(w, "schema unavailable", 500)
			return
		}
		unique, err := dialect.DecodeBoolean(uniqueRaw)
		if err != nil {
			http.Error(w, "schema unavailable", 500)
			return
		}
		if _, ok := collections[collection]; !ok {
			collections[collection] = []map[string]any{}
		}
		if name.Valid {
			collections[collection] = append(collections[collection], map[string]any{"name": name.String, "type": kind.String, "required": required, "unique": unique})
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "schema unavailable", 500)
		return
	}
	write(w, map[string]any{"collections": collections, "capabilities": capabilities()})
}
func capabilities() map[string]any {
	return map[string]any{"version": buildinfo.Current(), "apiVersion": "v1", "features": []string{"collections", "records", "auth", "files", "realtime", "jobs", "webhooks", "aws-lambda", "audit", "backups"}, "registrationPolicies": []string{"open", "invite", "approval", "closed"}, "limits": map[string]any{"recordPageMax": 100, "recordBatchMax": 1000, "uploadBytes": files.MaxUpload, "requestBodyBytes": 8 << 20, "eventBatch": 100}}
}
func openAPI() map[string]any {
	return map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "Trestle HTTP API", "version": "1"}, "servers": []map[string]string{{"url": "/"}}, "paths": map[string]any{"/api/v1/capabilities": map[string]any{"get": operation("Discover capabilities")}, "/api/v1/collections/{collection}/records": map[string]any{"get": operation("List records"), "post": operation("Create record")}, "/api/v1/collections/{collection}/records/{id}": map[string]any{"get": operation("Read record"), "patch": operation("Update record"), "delete": operation("Delete record")}, "/api/v1/files": map[string]any{"post": operation("Upload file")}, "/api/v1/realtime": map[string]any{"get": operation("Subscribe to events")}}, "components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer"}, "adminCookie": map[string]string{"type": "apiKey", "in": "cookie", "name": "trestle_admin"}}}}
}
func operation(summary string) map[string]any {
	return map[string]any{"summary": summary, "responses": map[string]any{"200": map[string]string{"description": "Success"}, "default": map[string]string{"description": "Structured API error"}}}
}
func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
