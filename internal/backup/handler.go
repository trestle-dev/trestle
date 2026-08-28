package backup

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/store"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Handler struct {
	db                store.Executor
	admin             *adminauth.Handler
	dataDir, provider string
	now               func() time.Time
}
type Manifest struct {
	Format             string `json:"format"`
	CreatedAt          string `json:"createdAt"`
	SchemaVersion      int    `json:"schemaVersion"`
	StorageProvider    string `json:"storageProvider"`
	IncludesDatabase   bool   `json:"includesDatabase"`
	IncludesLocalFiles bool   `json:"includesLocalFiles"`
	Secrets            string `json:"secrets"`
}

func New(db any, admin *adminauth.Handler, dataDir, provider string) (*Handler, error) {
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Handler{db: store.Adapt(db), admin: admin, dataDir: dataDir, provider: provider, now: time.Now}, nil
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mutation := r.Method != http.MethodGet
	if _, ok := h.admin.Authorize(r, mutation); !ok {
		http.Error(w, "forbidden", 403)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/admin/v1/backups":
		h.list(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/v1/backups":
		h.create(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/admin/v1/backups/"):
		h.download(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/v1/restores/preflight":
		h.preflight(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/admin/v1/export":
		h.export(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/v1/imports/dry-run":
		h.importDryRun(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	id := "backup-" + h.now().UTC().Format("20060102-150405.000000000") + ".zip"
	final := filepath.Join(h.dataDir, "backups", id)
	temp := final + ".tmp"
	snapshot := filepath.Join(h.dataDir, "backups", id+".db")
	sqliteSnapshot := h.db.Dialect().Provider() == store.SQLite
	if sqliteSnapshot {
		if _, err := h.db.ExecContext(r.Context(), "VACUUM INTO ?", snapshot); err != nil {
			http.Error(w, "database snapshot failed", 500)
			return
		}
		defer os.Remove(snapshot)
	}
	out, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		http.Error(w, "backup create failed", 500)
		return
	}
	archive := zip.NewWriter(out)
	manifest := Manifest{Format: "trestle-backup-v1", CreatedAt: h.now().UTC().Format(time.RFC3339Nano), SchemaVersion: store.CurrentVersion, StorageProvider: h.provider, IncludesDatabase: sqliteSnapshot, IncludesLocalFiles: h.provider == "local", Secrets: "administrator password hashes and encrypted integration secrets are included; live sessions are included and should be revoked after cross-host restore"}
	archiveErr := addJSON(archive, "manifest.json", manifest)
	if archiveErr == nil {
		if sqliteSnapshot {
			archiveErr = addFile(archive, "trestle.db", snapshot)
		}
	}
	if archiveErr == nil {
		var portable bytes.Buffer
		if err := Export(r.Context(), h.db, h.db.Dialect(), &portable); err != nil {
			archiveErr = errors.New("portable archive failed")
		} else {
			archiveErr = addBytes(archive, "portable.json", portable.Bytes())
		}
	}
	filesRoot := filepath.Join(h.dataDir, "files")
	if archiveErr == nil && h.provider == "local" {
		archiveErr = filepath.Walk(filesRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) && path == filesRoot {
					return nil
				}
				return walkErr
			}
			if info.IsDir() || strings.Contains(path, string(filepath.Separator)+".staging"+string(filepath.Separator)) {
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("local backup refuses symlinked file objects")
			}
			relative, err := filepath.Rel(filesRoot, path)
			if err != nil {
				return err
			}
			return addFile(archive, filepath.ToSlash(filepath.Join("files", relative)), path)
		})
	}
	closeErr := archive.Close()
	if err = out.Close(); err == nil && archiveErr != nil {
		err = archiveErr
	}
	if err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(temp, final) != nil {
		os.Remove(temp)
		http.Error(w, "backup finalization failed", 500)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "manifest": manifest})
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(filepath.Join(h.dataDir, "backups"))
	items := []map[string]any{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, _ := entry.Info()
		items = append(items, map[string]any{"id": entry.Name(), "size": info.Size(), "createdAt": info.ModTime().UTC().Format(time.RFC3339Nano)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["createdAt"].(string) > items[j]["createdAt"].(string) })
	writeJSON(w, 200, map[string]any{"items": items})
}
func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/admin/v1/backups/")
	if filepath.Base(id) != id || !strings.HasSuffix(id, ".zip") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(h.dataDir, "backups", id)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="trestle-backup.zip"`)
	http.ServeFile(w, r, path)
}
func (h *Handler) preflight(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
	file, _, err := r.FormFile("backup")
	if err != nil {
		http.Error(w, "backup required", 400)
		return
	}
	defer file.Close()
	temp, err := os.CreateTemp(filepath.Join(h.dataDir, "backups"), "preflight-*.zip")
	if err != nil {
		http.Error(w, "preflight failed", 500)
		return
	}
	defer os.Remove(temp.Name())
	io.Copy(temp, file)
	temp.Close()
	reader, err := zip.OpenReader(temp.Name())
	if err != nil {
		http.Error(w, "invalid archive", 422)
		return
	}
	defer reader.Close()
	var manifest Manifest
	hasDB := false
	hasPortable := false
	issues := []string{}
	for _, item := range reader.File {
		if item.Name == "manifest.json" {
			stream, _ := item.Open()
			json.NewDecoder(io.LimitReader(stream, 1<<20)).Decode(&manifest)
			stream.Close()
		}
		if item.Name == "trestle.db" {
			hasDB = true
		}
		if item.Name == "portable.json" {
			hasPortable = true
		}
		if strings.Contains(item.Name, "..") || strings.HasPrefix(item.Name, "/") {
			issues = append(issues, "unsafe archive path")
		}
	}
	if manifest.Format != "trestle-backup-v1" {
		issues = append(issues, "unsupported backup format")
	}
	if manifest.SchemaVersion > store.CurrentVersion {
		issues = append(issues, "backup comes from a newer Trestle schema")
	}
	if !hasDB && !hasPortable {
		issues = append(issues, "database snapshot or portable archive missing")
	}
	writeJSON(w, 200, map[string]any{"valid": len(issues) == 0, "manifest": manifest, "issues": issues, "next": "Stop Trestle and use the documented offline restore procedure; live replacement is deliberately refused."})
}
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	collections := []map[string]any{}
	rows, _ := h.db.QueryContext(r.Context(), "SELECT id,name,kind,created_at,updated_at FROM _trestle_collections ORDER BY name")
	defer rows.Close()
	for rows.Next() {
		var id, name, kind, created, updated string
		rows.Scan(&id, &name, &kind, &created, &updated)
		fields := []map[string]any{}
		fieldRows, _ := h.db.QueryContext(r.Context(), "SELECT name,type,required,is_unique,default_json FROM _trestle_fields WHERE collection_id=? ORDER BY position", id)
		for fieldRows.Next() {
			var field, typ string
			var requiredRaw, uniqueRaw any
			var def sql.NullString
			fieldRows.Scan(&field, &typ, &requiredRaw, &uniqueRaw, &def)
			required, _ := h.db.Dialect().DecodeBoolean(requiredRaw)
			unique, _ := h.db.Dialect().DecodeBoolean(uniqueRaw)
			fields = append(fields, map[string]any{"name": field, "type": typ, "required": required, "unique": unique, "default": def.String})
		}
		fieldRows.Close()
		collections = append(collections, map[string]any{"name": name, "kind": kind, "fields": fields, "createdAt": created, "updatedAt": updated})
	}
	w.Header().Set("Content-Disposition", `attachment; filename="trestle-export.json"`)
	writeJSON(w, 200, map[string]any{"format": "trestle-export-v1", "schemaVersion": store.CurrentVersion, "exportedAt": h.now().UTC().Format(time.RFC3339Nano), "collections": collections, "secretTreatment": "secrets, password hashes, sessions, audit, jobs and integration credentials are excluded"})
}
func (h *Handler) importDryRun(w http.ResponseWriter, r *http.Request) {
	var document struct {
		Format        string `json:"format"`
		SchemaVersion int    `json:"schemaVersion"`
		Collections   []struct {
			Name   string `json:"name"`
			Fields []any  `json:"fields"`
		} `json:"collections"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&document) != nil {
		http.Error(w, "invalid export", 422)
		return
	}
	issues := []string{}
	if document.Format != "trestle-export-v1" {
		issues = append(issues, "unsupported export format")
	}
	if document.SchemaVersion > store.CurrentVersion {
		issues = append(issues, "export requires a newer schema")
	}
	seen := map[string]bool{}
	for _, collection := range document.Collections {
		if collection.Name == "" || seen[collection.Name] {
			issues = append(issues, "collection names must be present and unique")
		}
		seen[collection.Name] = true
	}
	writeJSON(w, 200, map[string]any{"valid": len(issues) == 0, "issues": issues, "collections": len(document.Collections), "mode": "dry-run", "writes": 0})
}
func addJSON(archive *zip.Writer, name string, value any) error {
	writer, err := archive.Create(name)
	if err != nil {
		return err
	}
	return json.NewEncoder(writer).Encode(value)
}
func addBytes(archive *zip.Writer, name string, value []byte) error {
	writer, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(value)
	return err
}
func addFile(archive *zip.Writer, name, path string) error {
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer source.Close()
	writer, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, source)
	return err
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
