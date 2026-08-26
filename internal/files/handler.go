package files

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/httperr"
	"github.com/trestle-dev/trestle/internal/identities"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const MaxUpload int64 = 10 << 20
const DefaultQuota int64 = 100 << 20

type Handler struct {
	db          *sql.DB
	admin       *adminauth.Handler
	credentials *identities.Handler
	root        string
	now         func() time.Time
	quota       int64
}
type Metadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Collection  string `json:"collection,omitempty"`
	RecordID    string `json:"recordId,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

func New(db *sql.DB, admin *adminauth.Handler, credentials *identities.Handler, dataDir string) (*Handler, error) {
	root := filepath.Join(dataDir, "files")
	if err := os.MkdirAll(filepath.Join(root, ".staging"), 0700); err != nil {
		return nil, err
	}
	return &Handler{db: db, admin: admin, credentials: credentials, root: root, now: time.Now, quota: DefaultQuota}, nil
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mutation := r.Method != http.MethodGet
	if !h.authorized(r, mutation) {
		writeError(w, 403, "authorization_denied", "The request is not authorized.")
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files":
		h.upload(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/files/"):
		h.download(w, r, strings.TrimPrefix(r.URL.Path, "/api/v1/files/"))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/files/"):
		h.remove(w, r, strings.TrimPrefix(r.URL.Path, "/api/v1/files/"))
	case r.Method == http.MethodPost && r.URL.Path == "/admin/v1/files/cleanup":
		h.cleanup(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/admin/v1/files":
		h.list(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) authorized(r *http.Request, mutation bool) bool {
	if _, ok := h.admin.Authorize(r, mutation); ok {
		return true
	}
	scope := "files:read"
	if mutation {
		scope = "files:write"
	}
	_, ok := h.credentials.Authenticate(r, scope)
	return ok
}
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	var used int64
	if h.db.QueryRowContext(r.Context(), "SELECT coalesce(sum(size),0) FROM _trestle_files").Scan(&used) != nil || used >= h.quota {
		writeError(w, 413, "quota_exceeded", "The storage quota is exhausted.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUpload+1<<20)
	if err := r.ParseMultipartForm(MaxUpload); err != nil {
		writeError(w, 413, "upload_too_large", "The upload exceeds the limit.")
		return
	}
	source, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "file_required", "A multipart file is required.")
		return
	}
	defer source.Close()
	name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/")))
	if name == "" || name == "." || len(name) > 255 {
		writeError(w, 422, "invalid_filename", "The filename is invalid.")
		return
	}
	id := "fil_" + token(18)
	key := token(24)
	staged := filepath.Join(h.root, ".staging", key)
	target := filepath.Join(h.root, key)
	out, err := os.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(source, MaxUpload+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || size > MaxUpload || used+size > h.quota {
		os.Remove(staged)
		writeError(w, 413, "upload_too_large", "The upload exceeds a storage limit.")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err = os.Rename(staged, target); err != nil {
		os.Remove(staged)
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	now := h.now().UTC().Format(time.RFC3339Nano)
	digest := hex.EncodeToString(hash.Sum(nil))
	_, err = h.db.ExecContext(r.Context(), "INSERT INTO _trestle_files(id,storage_key,original_name,content_type,size,sha256,collection_name,record_id,created_at) VALUES(?,?,?,?,?,?,?,?,?)", id, key, name, contentType, size, digest, nullForm(r, "collection"), nullForm(r, "recordId"), now)
	if err != nil {
		os.Remove(target)
		writeError(w, 409, "file_binding_failed", "The file could not be associated.")
		return
	}
	writeJSON(w, 201, Metadata{ID: id, Name: name, ContentType: contentType, Size: size, SHA256: digest, Collection: r.FormValue("collection"), RecordID: r.FormValue("recordId"), CreatedAt: now})
}
func (h *Handler) lookup(r *http.Request, id string) (Metadata, string, error) {
	var m Metadata
	var key string
	var collection, record sql.NullString
	err := h.db.QueryRowContext(r.Context(), "SELECT id,storage_key,original_name,content_type,size,sha256,collection_name,record_id,created_at FROM _trestle_files WHERE id=?", id).Scan(&m.ID, &key, &m.Name, &m.ContentType, &m.Size, &m.SHA256, &collection, &record, &m.CreatedAt)
	if collection.Valid {
		m.Collection = collection.String
	}
	if record.Valid {
		m.RecordID = record.String
	}
	return m, key, err
}
func (h *Handler) download(w http.ResponseWriter, r *http.Request, id string) {
	m, key, err := h.lookup(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "file_not_found", "The file was not found.")
		return
	}
	path := filepath.Join(h.root, key)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, 404, "file_not_found", "The file was not found.")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(w, 404, "file_not_found", "The file was not found.")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="download"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, m.Name, info.ModTime(), file)
}
func (h *Handler) remove(w http.ResponseWriter, r *http.Request, id string) {
	_, key, err := h.lookup(r, id)
	if err != nil {
		writeError(w, 404, "file_not_found", "The file was not found.")
		return
	}
	tx, _ := h.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	tx.ExecContext(r.Context(), "DELETE FROM _trestle_files WHERE id=?", id)
	if err = os.Remove(filepath.Join(h.root, key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, 500, "internal_error", "The file could not be removed.")
		return
	}
	tx.Commit()
	w.WriteHeader(204)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), "SELECT id,original_name,content_type,size,sha256,coalesce(collection_name,''),coalesce(record_id,''),created_at FROM _trestle_files ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	defer rows.Close()
	items := []Metadata{}
	for rows.Next() {
		var m Metadata
		rows.Scan(&m.ID, &m.Name, &m.ContentType, &m.Size, &m.SHA256, &m.Collection, &m.RecordID, &m.CreatedAt)
		items = append(items, m)
	}
	writeJSON(w, 200, map[string]any{"items": items, "quota": h.quota})
}
func (h *Handler) cleanup(w http.ResponseWriter, r *http.Request) {
	known := map[string]bool{}
	rows, _ := h.db.QueryContext(r.Context(), "SELECT storage_key FROM _trestle_files")
	for rows.Next() {
		var key string
		rows.Scan(&key)
		known[key] = true
	}
	rows.Close()
	entries, _ := os.ReadDir(h.root)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || known[entry.Name()] {
			continue
		}
		if os.Remove(filepath.Join(h.root, entry.Name())) == nil {
			removed++
		}
	}
	staged, _ := os.ReadDir(filepath.Join(h.root, ".staging"))
	for _, entry := range staged {
		info, _ := entry.Info()
		if info != nil && h.now().Sub(info.ModTime()) > time.Hour {
			if os.Remove(filepath.Join(h.root, ".staging", entry.Name())) == nil {
				removed++
			}
		}
	}
	writeJSON(w, 200, map[string]int{"removed": removed})
}
func nullForm(r *http.Request, key string) any {
	if value := strings.TrimSpace(r.FormValue(key)); value != "" {
		return value
	}
	return nil
}
func token(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, httperr.New(code, message, w.Header().Get("X-Request-ID")))
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
