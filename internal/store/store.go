package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const CurrentVersion = 8

type Store struct {
	db   *sql.DB
	path string
}

type migration struct {
	version   int
	name, sql string
}

var migrations = []migration{{1, "system foundation", `
CREATE TABLE _trestle_schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL
) STRICT;
CREATE TABLE _trestle_system_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE _trestle_audit (
  id INTEGER PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  actor_kind TEXT NOT NULL,
  actor_id TEXT,
  action TEXT NOT NULL,
  target TEXT,
  outcome TEXT NOT NULL,
  request_id TEXT
) STRICT;
`}, {2, "administrator identity", `
CREATE TABLE _trestle_admins (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  disabled_at TEXT
) STRICT;
CREATE TABLE _trestle_admin_sessions (
  id TEXT PRIMARY KEY,
  admin_id TEXT NOT NULL REFERENCES _trestle_admins(id) ON DELETE CASCADE,
  token_hash BLOB NOT NULL UNIQUE,
  csrf_hash BLOB NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
) STRICT;
CREATE INDEX _trestle_admin_sessions_admin ON _trestle_admin_sessions(admin_id);
`}, {3, "collection metadata", `
CREATE TABLE _trestle_collections (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE,
  kind TEXT NOT NULL CHECK(kind IN ('base')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE _trestle_fields (
  id TEXT PRIMARY KEY,
  collection_id TEXT NOT NULL REFERENCES _trestle_collections(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  required INTEGER NOT NULL CHECK(required IN (0,1)),
  is_unique INTEGER NOT NULL CHECK(is_unique IN (0,1)),
  default_json TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(collection_id,name),
  UNIQUE(collection_id,position)
) STRICT;
`}, {4, "record idempotency", `
CREATE TABLE _trestle_record_idempotency (
  collection_id TEXT NOT NULL REFERENCES _trestle_collections(id) ON DELETE CASCADE,
  idempotency_key TEXT NOT NULL,
  record_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(collection_id,idempotency_key)
) STRICT;
`}, {5, "application user identity", `
CREATE TABLE _trestle_app_users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  verified_at TEXT,
  disabled_at TEXT,
  created_at TEXT NOT NULL
) STRICT;
CREATE TABLE _trestle_app_sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES _trestle_app_users(id) ON DELETE CASCADE,
  refresh_hash BLOB NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  replaced_by TEXT
) STRICT;
CREATE INDEX _trestle_app_sessions_user ON _trestle_app_sessions(user_id);
`}, {6, "scoped backend credentials", `
CREATE TABLE _trestle_credentials (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK(kind IN ('service','personal')),
  name TEXT NOT NULL,
  owner_admin_id TEXT REFERENCES _trestle_admins(id) ON DELETE CASCADE,
  secret_hash BLOB NOT NULL UNIQUE,
  scopes TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT,
  revoked_at TEXT,
  last_used_at TEXT
) STRICT;
CREATE INDEX _trestle_credentials_kind ON _trestle_credentials(kind);
`}, {7, "collection access rules", `
CREATE TABLE _trestle_app_access (
  token_hash BLOB PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES _trestle_app_sessions(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES _trestle_app_users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL
) STRICT;
CREATE TABLE _trestle_collection_rules (
  collection_id TEXT NOT NULL REFERENCES _trestle_collections(id) ON DELETE CASCADE,
  operation TEXT NOT NULL CHECK(operation IN ('list','view','create','update','delete')),
  expression TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(collection_id,operation)
) STRICT;
`}, {8, "local file storage", `
CREATE TABLE _trestle_files (
  id TEXT PRIMARY KEY,
  storage_key TEXT NOT NULL UNIQUE,
  original_name TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size INTEGER NOT NULL CHECK(size >= 0),
  sha256 TEXT NOT NULL,
  collection_name TEXT,
  record_id TEXT,
  created_at TEXT NOT NULL
) STRICT;
CREATE INDEX _trestle_files_record ON _trestle_files(collection_name,record_id);
`}}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure data directory: %w", err)
	}
	path := filepath.Join(dataDir, "trestle.db")
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	s := &Store{db: db, path: path}
	if err := s.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) initialize(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		return errors.New("sqlite foreign keys are not enabled")
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > CurrentVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, CurrentVersion)
	}
	for _, m := range migrations {
		if m.version > version {
			if err := s.apply(ctx, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) apply(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.version, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO _trestle_schema_migrations(version,name,applied_at) VALUES(?,?,?)", m.version, m.name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record migration %d: %w", m.version, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return fmt.Errorf("set schema version %d: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", m.version, err)
	}
	return nil
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Path() string                   { return s.path }
func (s *Store) DB() *sql.DB                    { return s.db }
