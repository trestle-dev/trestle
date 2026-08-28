package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/trestle-dev/trestle/internal/store"
)

const maxRestoreBytes int64 = 16 << 30

// RestoreOptions selects the offline recovery destination provider. The
// default restores a SQLite snapshot or portable archive into a new SQLite data
// directory. Provider Postgres restores a portable archive into an initialized
// empty PostgreSQL database at the current schema version.
type RestoreOptions struct {
	Provider store.Provider
	URL      string
}

// Restore extracts a Trestle archive into a new data directory. The caller must
// stop the server first; an existing target is always refused.
func Restore(ctx context.Context, archivePath, targetDir string, options ...RestoreOptions) error {
	if archivePath == "" {
		return errors.New("backup archive is required")
	}
	if len(options) > 0 && options[0].Provider == store.Postgres {
		if options[0].URL == "" {
			return errors.New("postgres database URL is required")
		}
		return restoreLogicalToPostgres(ctx, archivePath, options[0].URL)
	}
	if targetDir == "" {
		return errors.New("data directory is required")
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
	_, portableBytes, hasDB, err := preflightArchive(reader)
	if err != nil {
		reader.Close()
		return err
	}
	_ = portableBytes
	parent := filepath.Dir(targetDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		reader.Close()
		return fmt.Errorf("create restore parent: %w", err)
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(targetDir)+".restore-*")
	if err != nil {
		reader.Close()
		return fmt.Errorf("create restore staging: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := os.Chmod(staging, 0o700); err != nil {
		reader.Close()
		return err
	}
	if hasDB {
		// Second pass: extract the SQLite snapshot and the local file objects.
		var total int64
		for _, item := range reader.File {
			if item.FileInfo().IsDir() {
				continue
			}
			name, err := safeArchiveName(item.Name)
			if err != nil {
				reader.Close()
				return err
			}
			if name != "trestle.db" && !strings.HasPrefix(name, "files/") {
				continue
			}
			total += int64(item.UncompressedSize64)
			if total > maxRestoreBytes {
				reader.Close()
				return errors.New("backup expands beyond restore limit")
			}
			stream, err := item.Open()
			if err != nil {
				reader.Close()
				return fmt.Errorf("open %q: %w", item.Name, err)
			}
			destination := filepath.Join(staging, filepath.FromSlash(name))
			if err = os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
				stream.Close()
				reader.Close()
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
				reader.Close()
				return fmt.Errorf("extract %q: %w", item.Name, err)
			}
		}
		reader.Close()
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
	} else {
		// Portable logical archive restored into a fresh SQLite data directory.
		reader.Close()
		restored, err := store.Open(ctx, staging)
		if err != nil {
			return fmt.Errorf("initialize restore destination: %w", err)
		}
		if err := Import(ctx, restored.DB(), restored.Dialect(), bytes.NewReader(portableBytes)); err != nil {
			restored.Close()
			return fmt.Errorf("restore portable archive: %w", err)
		}
		restored.Close()
	}
	if err := os.Rename(staging, targetDir); err != nil {
		return fmt.Errorf("publish restored data: %w", err)
	}
	return nil
}

// preflightArchive validates a backup archive before either restore provider
// opens or modifies its destination: manifest presence and format, future
// schema, safe names, symlink/duplicate/unexpected entries and the expansion
// boundary. It returns the manifest, the extracted portable archive bytes (if
// present) and whether a SQLite snapshot is present.
func preflightArchive(reader *zip.ReadCloser) (Manifest, []byte, bool, error) {
	var manifest Manifest
	hasManifest, hasDB := false, false
	var portableBytes []byte
	var total int64
	seen := map[string]bool{}
	for _, item := range reader.File {
		if seen[item.Name] {
			return Manifest{}, nil, false, fmt.Errorf("duplicate archive entry %q", item.Name)
		}
		seen[item.Name] = true
		name, err := safeArchiveName(item.Name)
		if err != nil {
			return Manifest{}, nil, false, err
		}
		if item.Mode()&os.ModeSymlink != 0 {
			return Manifest{}, nil, false, fmt.Errorf("archive contains symlink %q", item.Name)
		}
		if item.FileInfo().IsDir() {
			continue
		}
		if name != "manifest.json" && name != "trestle.db" && name != "portable.json" && !strings.HasPrefix(name, "files/") {
			return Manifest{}, nil, false, fmt.Errorf("unexpected archive entry %q", item.Name)
		}
		total += int64(item.UncompressedSize64)
		if total > maxRestoreBytes {
			return Manifest{}, nil, false, errors.New("backup expands beyond restore limit")
		}
		stream, err := item.Open()
		if err != nil {
			return Manifest{}, nil, false, fmt.Errorf("open %q: %w", item.Name, err)
		}
		if name == "manifest.json" {
			err = json.NewDecoder(io.LimitReader(stream, 1<<20)).Decode(&manifest)
			stream.Close()
			if err != nil {
				return Manifest{}, nil, false, errors.New("invalid backup manifest")
			}
			hasManifest = true
			continue
		}
		if name == "portable.json" {
			portableBytes, err = io.ReadAll(io.LimitReader(stream, 1<<30))
			stream.Close()
			if err != nil {
				return Manifest{}, nil, false, err
			}
			continue
		}
		if name == "trestle.db" {
			hasDB = true
		}
		stream.Close()
	}
	if !hasManifest || manifest.Format != "trestle-backup-v1" {
		return Manifest{}, nil, false, errors.New("unsupported or missing backup manifest")
	}
	if manifest.SchemaVersion > store.CurrentVersion {
		return Manifest{}, nil, false, errors.New("backup comes from a newer Trestle schema")
	}
	if !hasDB && len(portableBytes) == 0 {
		return Manifest{}, nil, false, errors.New("database snapshot or portable archive missing")
	}
	return manifest, portableBytes, hasDB, nil
}

// restoreLogicalToPostgres restores a portable logical archive into an
// initialized empty PostgreSQL database at the current schema version. The
// archive is fully preflighted first, and the destination is validated
// read-only before the transactional import, so a failed restore leaves the
// pre-existing initialized destination semantically unchanged.
func restoreLogicalToPostgres(ctx context.Context, archivePath, url string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	_, portableBytes, _, err := preflightArchive(reader)
	reader.Close()
	if err != nil {
		return err
	}
	if len(portableBytes) == 0 {
		return errors.New("portable archive missing from backup")
	}
	if err := validateEmptyPostgresDestination(ctx, url); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "trestle-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	db, err := store.OpenWith(ctx, store.Options{DataDir: dir, Provider: store.Postgres, URL: url, MaxOpen: 4, MaxIdle: 1, ConnectTimeout: 10 * time.Second})
	if err != nil {
		return fmt.Errorf("open restore destination: %w", err)
	}
	defer db.Close()
	if err := Import(ctx, db.DB(), db.Dialect(), bytes.NewReader(portableBytes)); err != nil {
		return fmt.Errorf("restore portable archive: %w", err)
	}
	return nil
}

// validateEmptyPostgresDestination checks, without running migrations or
// repairs, that the PostgreSQL destination is already initialized at the
// current Trestle schema and logically empty (no collections and no
// administrators). A failed restore therefore leaves it semantically unchanged.
func validateEmptyPostgresDestination(ctx context.Context, url string) error {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return fmt.Errorf("open restore destination: %w", err)
	}
	defer db.Close()
	executor := store.NewExecutor(db, store.NewDialect(store.Postgres))
	version, err := store.ValidateMigrationHistory(ctx, executor)
	if err != nil {
		return fmt.Errorf("restore destination is not an initialized Trestle database: %w", err)
	}
	if version != store.CurrentVersion {
		return fmt.Errorf("restore destination schema version %d is not current %d", version, store.CurrentVersion)
	}
	var collections, admins int
	if err := executor.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_collections").Scan(&collections); err != nil {
		return err
	}
	if err := executor.QueryRowContext(ctx, "SELECT count(*) FROM _trestle_admins").Scan(&admins); err != nil {
		return err
	}
	if collections != 0 || admins != 0 {
		return errors.New("restore destination is not empty")
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
