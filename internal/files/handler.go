package files

import (
	"context"
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
	"github.com/trestle-dev/trestle/internal/store"
	"io"
	"log/slog"
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
	db          store.Executor
	admin       *adminauth.Handler
	credentials *identities.Handler
	root        string
	storage     objectStorage
	provider    string
	now         func() time.Time
	quota       int64
	log         *slog.Logger
}
type Options struct{ Backend, S3Endpoint, S3Region, S3Bucket, S3AccessKey, S3SecretKey string }
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

func New(db any, admin *adminauth.Handler, credentials *identities.Handler, dataDir string, options ...Options) (*Handler, error) {
	root := filepath.Join(dataDir, "files")
	if err := os.MkdirAll(filepath.Join(root, ".staging"), 0700); err != nil {
		return nil, err
	}
	var storage objectStorage = &localStorage{root: root}
	provider := "local"
	if len(options) > 0 && options[0].Backend == "s3" {
		storage = newS3Storage(options[0])
		provider = "s3"
	}
	return &Handler{db: store.Adapt(db), admin: admin, credentials: credentials, root: root, storage: storage, provider: provider, now: time.Now, quota: DefaultQuota}, nil
}
func (h *Handler) SetLogger(log *slog.Logger) { h.log = log }
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
	case r.Method == http.MethodGet && r.URL.Path == "/admin/v1/storage/status":
		h.status(w, r)
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
	if h.db.QueryRowContext(r.Context(), "SELECT coalesce(sum(size),0) FROM _trestle_files WHERE deleted_at IS NULL").Scan(&used) != nil || used >= h.quota {
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
	if err = h.storage.Put(r.Context(), key, staged, contentType); err != nil {
		os.Remove(staged)
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	now := h.now().UTC().Format(time.RFC3339Nano)
	digest := hex.EncodeToString(hash.Sum(nil))
	_, err = h.db.ExecContext(r.Context(), "INSERT INTO _trestle_files(id,storage_key,original_name,content_type,size,sha256,collection_name,record_id,created_at) VALUES(?,?,?,?,?,?,?,?,?)", id, key, name, contentType, size, digest, nullForm(r, "collection"), nullForm(r, "recordId"), now)
	if err != nil {
		_ = h.storage.Delete(r.Context(), key)
		writeError(w, 409, "file_binding_failed", "The file could not be associated.")
		return
	}
	writeJSON(w, 201, Metadata{ID: id, Name: name, ContentType: contentType, Size: size, SHA256: digest, Collection: r.FormValue("collection"), RecordID: r.FormValue("recordId"), CreatedAt: now})
}
func (h *Handler) lookup(r *http.Request, id string) (Metadata, string, error) {
	var m Metadata
	var key string
	var collection, record sql.NullString
	err := h.db.QueryRowContext(r.Context(), "SELECT id,storage_key,original_name,content_type,size,sha256,collection_name,record_id,created_at FROM _trestle_files WHERE id=? AND deleted_at IS NULL", id).Scan(&m.ID, &key, &m.Name, &m.ContentType, &m.Size, &m.SHA256, &collection, &record, &m.CreatedAt)
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
	if err = h.storage.Serve(w, r, key, m); err != nil {
		writeError(w, 404, "file_not_found", "The file was not found.")
	}
}
func (h *Handler) remove(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	var key string
	if err := h.db.QueryRowContext(ctx, "SELECT storage_key FROM _trestle_files WHERE id=?", id).Scan(&key); err != nil {
		writeError(w, 404, "file_not_found", "The file was not found.")
		return
	}
	now := h.now().UTC().Format(time.RFC3339Nano)
	// Durable deletion intent: mark the metadata unavailable to readers and
	// record the deletion intent in one transaction. No storage object is
	// deleted before this commits, and a failed transaction changes nothing.
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO _trestle_file_deletions(id,storage_key,status,attempts,created_at) VALUES(?,?,'pending',0,?) ON CONFLICT(id) DO NOTHING", id, key, now); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	if _, err := tx.ExecContext(ctx, "UPDATE _trestle_files SET deleted_at=? WHERE id=?", now, id); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	// Storage deletion runs only after durable intent is committed. A storage
	// failure leaves the intent pending and the file unavailable; startup and
	// the periodic recovery worker resume it. Success is returned only after
	// the object is gone and the intent is finalized.
	if err := h.storage.Delete(ctx, key); err != nil {
		writeError(w, 500, "deletion_pending", "The file is marked for deletion and will be cleaned up automatically.")
		return
	}
	if err := h.finalizeDeletion(ctx, id, now); err != nil {
		writeError(w, 500, "internal_error", "The request could not be completed.")
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) finalizeDeletion(ctx context.Context, id, now string) error {
	_, err := h.db.ExecContext(ctx, "UPDATE _trestle_file_deletions SET status='done', finalized_at=? WHERE id=? AND status='pending'", now, id)
	return err
}

// ResumePendingDeletions recovers unfinished file deletions: it ensures every
// file whose metadata is marked deleted has a deletion-intent row (including
// files restored from an archive whose deletion was pending), deletes the
// storage object for each processable pending intent, and finalizes. Storage
// deletion is idempotent, so a duplicate worker run or a crash between storage
// deletion and finalization both converge to the finalized state.
//
// Target selection fails closed: a pending intent is processed only when its
// file metadata is absent or marked deleted, and before each storage deletion
// the sweep confirms no live metadata (deleted_at IS NULL) references that
// storage key. An intent paired with live metadata is a conflict: it is
// skipped, logged, and not finalized, and it remains recoverable once the live
// reference is deliberately removed.
func (h *Handler) ResumePendingDeletions(ctx context.Context) (int, error) {
	if _, err := h.db.ExecContext(ctx, "INSERT INTO _trestle_file_deletions(id,storage_key,status,attempts,created_at) SELECT id,storage_key,'pending',0,deleted_at FROM _trestle_files WHERE deleted_at IS NOT NULL AND id NOT IN (SELECT id FROM _trestle_file_deletions) ON CONFLICT(id) DO NOTHING"); err != nil {
		return 0, err
	}
	// Only pending intents whose file row is absent or marked deleted are
	// processable; live metadata paired with an intent is a conflict.
	rows, err := h.db.QueryContext(ctx, `SELECT d.id, d.storage_key
		FROM _trestle_file_deletions d
		LEFT JOIN _trestle_files f ON f.id = d.id
		WHERE d.status='pending' AND (f.id IS NULL OR f.deleted_at IS NOT NULL)`)
	if err != nil {
		return 0, err
	}
	type target struct{ id, key string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.key); err != nil {
			rows.Close()
			return 0, err
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	now := h.now().UTC().Format(time.RFC3339Nano)
	processed := 0
	for _, t := range targets {
		// Fail closed: never delete an object referenced by live metadata.
		var live int
		if err := h.db.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_files WHERE storage_key=? AND deleted_at IS NULL", t.key).Scan(&live); err != nil {
			continue
		}
		if live > 0 {
			if h.log != nil {
				h.log.Warn("file deletion conflict: storage key still referenced by live metadata; intent left pending", "id", t.id)
			}
			continue
		}
		if err := h.storage.Delete(ctx, t.key); err != nil {
			continue
		}
		if _, err := h.db.ExecContext(ctx, "UPDATE _trestle_file_deletions SET status='done', finalized_at=? WHERE id=? AND status='pending'", now, t.id); err == nil {
			processed++
		}
	}
	return processed, nil
}

// RunDeletionRecovery periodically resumes pending file deletions so a storage
// failure during a continuously running process converges without waiting for a
// restart. It is bounded, stops when ctx is cancelled, and logs observable
// status; errors and conflicts are logged and pending deletions remain
// recoverable at the next tick or at startup.
func (h *Handler) RunDeletionRecovery(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	run := func() {
		processed, err := h.ResumePendingDeletions(ctx)
		if err != nil {
			if h.log != nil {
				h.log.Error("file deletion recovery failed", "error", err)
			}
			return
		}
		if processed > 0 && h.log != nil {
			h.log.Info("resumed pending file deletions", "count", processed)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), "SELECT id,original_name,content_type,size,sha256,coalesce(collection_name,''),coalesce(record_id,''),created_at FROM _trestle_files WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT 200")
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
	removed, _ := h.storage.Cleanup(r.Context(), known)
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
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"provider": h.provider, "configured": true, "quota": h.quota})
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
