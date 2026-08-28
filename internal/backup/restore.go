package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/trestle-dev/trestle/internal/store"
)

const maxRestoreBytes int64 = 16 << 30

// Restore extracts a Trestle archive into a new data directory. The caller must
// stop the server first; an existing target is always refused.
func Restore(ctx context.Context, archivePath, targetDir string) error {
	if archivePath == "" || targetDir == "" {
		return errors.New("backup and data directory are required")
	}
	if _, err := os.Lstat(targetDir); err == nil {
		return errors.New("restore target already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect restore target: %w", err)
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer reader.Close()
	parent := filepath.Dir(targetDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create restore parent: %w", err)
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(targetDir)+".restore-*")
	if err != nil {
		return fmt.Errorf("create restore staging: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := os.Chmod(staging, 0o700); err != nil {
		return err
	}
	var manifest Manifest
	hasManifest, hasDB, hasPortable := false, false, false
	var total int64
	for _, item := range reader.File {
		name, err := safeArchiveName(item.Name)
		if err != nil {
			return err
		}
		if item.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains symlink %q", item.Name)
		}
		if item.FileInfo().IsDir() {
			continue
		}
		if name != "manifest.json" && name != "trestle.db" && name != "portable.json" && !strings.HasPrefix(name, "files/") {
			return fmt.Errorf("unexpected archive entry %q", item.Name)
		}
		total += int64(item.UncompressedSize64)
		if total > maxRestoreBytes {
			return errors.New("backup expands beyond restore limit")
		}
		stream, err := item.Open()
		if err != nil {
			return fmt.Errorf("open %q: %w", item.Name, err)
		}
		if name == "manifest.json" {
			err = json.NewDecoder(io.LimitReader(stream, 1<<20)).Decode(&manifest)
			stream.Close()
			if err != nil {
				return errors.New("invalid backup manifest")
			}
			hasManifest = true
			continue
		}
		destination := filepath.Join(staging, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			stream.Close()
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, err = io.CopyN(output, stream, int64(item.UncompressedSize64))
		}
		closeErr := outputClose(output)
		stream.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("extract %q: %w", item.Name, err)
		}
		if name == "trestle.db" {
			hasDB = true
		}
		if name == "portable.json" {
			hasPortable = true
		}
	}
	if !hasManifest || manifest.Format != "trestle-backup-v1" {
		return errors.New("unsupported or missing backup manifest")
	}
	if manifest.SchemaVersion > store.CurrentVersion {
		return errors.New("backup comes from a newer Trestle schema")
	}
	if hasDB {
		verified, err := store.Open(ctx, staging)
		if err != nil {
			return fmt.Errorf("verify restored database: %w", err)
		}
		var integrity string
		err = verified.DB().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity)
		verified.Close()
		if err != nil || integrity != "ok" {
			return fmt.Errorf("restored database integrity failed: %s", integrity)
		}
	} else if hasPortable {
		restored, err := store.Open(ctx, staging)
		if err != nil {
			return fmt.Errorf("initialize restore destination: %w", err)
		}
		portableFile, err := os.Open(filepath.Join(staging, "portable.json"))
		if err != nil {
			restored.Close()
			return err
		}
		if err := Import(ctx, restored.DB(), restored.Dialect(), portableFile); err != nil {
			portableFile.Close()
			restored.Close()
			return fmt.Errorf("restore portable archive: %w", err)
		}
		portableFile.Close()
		var collectionsCount int
		if err := restored.DB().QueryRowContext(ctx, "SELECT count(*) FROM _trestle_collections").Scan(&collectionsCount); err != nil {
			restored.Close()
			return fmt.Errorf("verify restored archive: %w", err)
		}
		restored.Close()
	} else {
		return errors.New("database snapshot or portable archive missing")
	}
	if err := os.Rename(staging, targetDir); err != nil {
		return fmt.Errorf("publish restored data: %w", err)
	}
	return nil
}

func safeArchiveName(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return name, nil
}

func outputClose(file *os.File) error {
	if file == nil {
		return nil
	}
	return file.Close()
}
