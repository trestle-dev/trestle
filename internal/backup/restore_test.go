package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trestle-dev/trestle/internal/store"
)

func TestBackupRestoreDrillPreservesDatabaseAndFiles(t *testing.T) {
	handler, csrf, cookie := testHandler(t)
	if _, err := handler.db.Exec("INSERT INTO _trestle_system_meta(key,value,updated_at) VALUES('restore-probe','preserved','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(handler.dataDir, "files", "objects", "probe.txt")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("restored file"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := authorizedRequest(t, handler, "POST", "/admin/v1/backups", `{}`, csrf, cookie)
	if response.Code != 201 {
		t.Fatalf("backup: %d %s", response.Code, response.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if err := Restore(context.Background(), filepath.Join(handler.dataDir, "backups", created.ID), target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "files", "objects", "probe.txt"))
	if err != nil || string(data) != "restored file" {
		t.Fatalf("restored file=%q err=%v", data, err)
	}
	var value string
	db, err := store.Open(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.DB().QueryRow("SELECT value FROM _trestle_system_meta WHERE key='restore-probe'").Scan(&value); err != nil || value != "preserved" {
		t.Fatalf("restored value=%q err=%v", value, err)
	}
}

func TestRestoreRejectsHostileAndCorruptArchives(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		entries  map[string][]byte
		want     string
	}{
		{"traversal", Manifest{Format: "trestle-backup-v1", SchemaVersion: 13}, map[string][]byte{"../escape": []byte("x"), "trestle.db": []byte("x")}, "unsafe archive path"},
		{"backslash", Manifest{Format: "trestle-backup-v1", SchemaVersion: 13}, map[string][]byte{`files\escape`: []byte("x"), "trestle.db": []byte("x")}, "unsafe archive path"},
		{"missing database", Manifest{Format: "trestle-backup-v1", SchemaVersion: 13}, nil, "database snapshot or portable archive missing"},
		{"future schema", Manifest{Format: "trestle-backup-v1", SchemaVersion: 999}, map[string][]byte{"trestle.db": []byte("x")}, "newer Trestle schema"},
		{"corrupt database", Manifest{Format: "trestle-backup-v1", SchemaVersion: 13}, map[string][]byte{"trestle.db": []byte("not sqlite")}, "verify restored database"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "hostile.zip")
			writeArchive(t, archive, tc.manifest, tc.entries)
			target := filepath.Join(t.TempDir(), "restore")
			err := Restore(context.Background(), archive, target)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Fatalf("failed restore published target: %v", statErr)
			}
		})
	}
}

func TestRestoreRefusesExistingTargetAndTruncatedZip(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "backup.zip")
	writeArchive(t, archive, Manifest{Format: "trestle-backup-v1", SchemaVersion: 13}, nil)
	if err := Restore(context.Background(), archive, target); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing target error=%v", err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	truncated := filepath.Join(dir, "truncated.zip")
	if err := os.WriteFile(truncated, data[:len(data)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Restore(context.Background(), truncated, filepath.Join(dir, "new")); err == nil || !strings.Contains(err.Error(), "open backup") {
		t.Fatalf("truncated error=%v", err)
	}
}

func TestBackupRefusesSymlinkedObjectWithoutPublishingArchive(t *testing.T) {
	handler, csrf, cookie := testHandler(t)
	external := filepath.Join(t.TempDir(), "external-secret")
	if err := os.WriteFile(external, []byte("must not enter backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	objectDir := filepath.Join(handler.dataDir, "files", "objects")
	if err := os.MkdirAll(objectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(objectDir, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	response := authorizedRequest(t, handler, "POST", "/admin/v1/backups", `{}`, csrf, cookie)
	if response.Code != 500 {
		t.Fatalf("backup status=%d body=%s", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(handler.dataDir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".zip") || strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("failed backup published %s", entry.Name())
		}
	}
}

func writeArchive(t *testing.T, path string, manifest Manifest, entries map[string][]byte) {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	manifestWriter, _ := archive.Create("manifest.json")
	if err := json.NewEncoder(manifestWriter).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	for name, data := range entries {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
