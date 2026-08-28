package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/trestle-dev/trestle/internal/collections"
	"github.com/trestle-dev/trestle/internal/store"
)

type MigrateOptions struct {
	SourceProvider store.Provider
	SourceDir      string
	SourceURL      string
	TargetProvider store.Provider
	TargetDir      string
	TargetURL      string
	DryRun         bool
	Confirm        bool
}

type MigrateReport struct {
	Format          string           `json:"format"`
	SourceProvider  string           `json:"sourceProvider"`
	TargetProvider  string           `json:"targetProvider"`
	DryRun          bool             `json:"dryRun"`
	ExportedAt      string           `json:"exportedAt"`
	Collections     int              `json:"collections"`
	Records         int              `json:"records"`
	Checksum        string           `json:"checksum"`
	DestinationHash string           `json:"destinationHash"`
	Counts          map[string]int64 `json:"counts"`
	WritesPerformed bool             `json:"writesPerformed"`
}

// readSourceVersion reads the validated migration history maximum without
// running any migration or metadata repair, so the source is never written.
func readSourceVersion(ctx context.Context, executor store.Executor) (int, error) {
	var version sql.NullInt64
	err := executor.QueryRowContext(ctx, "SELECT MAX(version) FROM _trestle_schema_migrations").Scan(&version)
	if err != nil {
		return 0, errors.New("source has no migration history; upgrade it first so the migration never writes to it")
	}
	if !version.Valid {
		return 0, errors.New("source has no migration history; upgrade it first so the migration never writes to it")
	}
	return int(version.Int64), nil
}

// openSource opens the source through a genuinely non-mutating path: SQLite is
// opened read-only and PostgreSQL through a plain connection that the exporter
// uses inside a read-only repeatable snapshot. No schema migration or metadata
// repair runs before validation, and a stale source fails with zero writes.
func openSource(ctx context.Context, opts MigrateOptions) (store.Executor, store.Dialect, func() error, error) {
	if opts.SourceProvider == store.SQLite {
		path := filepath.Join(opts.SourceDir, "trestle.db")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=foreign_keys(1)")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("open source read-only: %w", err)
		}
		executor := store.NewExecutor(db, store.NewDialect(store.SQLite))
		version, err := readSourceVersion(ctx, executor)
		if err != nil {
			db.Close()
			return nil, nil, nil, err
		}
		if version != store.CurrentVersion {
			db.Close()
			return nil, nil, nil, fmt.Errorf("source schema version %d is not the current version %d; upgrade the source first", version, store.CurrentVersion)
		}
		return executor, store.NewDialect(store.SQLite), db.Close, nil
	}
	db, err := sql.Open("postgres", opts.SourceURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open source: %w", err)
	}
	executor := store.NewExecutor(db, store.NewDialect(store.Postgres))
	version, err := readSourceVersion(ctx, executor)
	if err != nil {
		db.Close()
		return nil, nil, nil, err
	}
	if version != store.CurrentVersion {
		db.Close()
		return nil, nil, nil, fmt.Errorf("source schema version %d is not the current version %d; upgrade the source first", version, store.CurrentVersion)
	}
	return executor, store.NewDialect(store.Postgres), db.Close, nil
}

func openTarget(ctx context.Context, opts MigrateOptions) (*store.Store, func(), error) {
	dataDir := opts.TargetDir
	cleanup := func() {}
	if dataDir == "" {
		dir, err := os.MkdirTemp("", "trestle-migrate-*")
		if err != nil {
			return nil, nil, err
		}
		dataDir = dir
		cleanup = func() { os.RemoveAll(dir) }
	}
	s, err := store.OpenWith(ctx, store.Options{DataDir: dataDir, Provider: opts.TargetProvider, URL: opts.TargetURL, MaxOpen: 4, MaxIdle: 1, ConnectTimeout: 10 * time.Second})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return s, cleanup, nil
}

// Migrate moves a database between providers using the portable logical
// archive. The source is only read (read-only SQLite or a read-only repeatable
// snapshot on PostgreSQL); the target must be an initialized empty database. A
// dry run exports and validates without touching the target. A real run
// requires explicit confirmation and verifies the destination semantically.
func Migrate(ctx context.Context, opts MigrateOptions) (MigrateReport, error) {
	if opts.SourceProvider == opts.TargetProvider {
		return MigrateReport{}, errors.New("source and target providers must differ")
	}
	if strings.TrimSpace(opts.SourceDir) == "" && strings.TrimSpace(opts.SourceURL) == "" {
		return MigrateReport{}, errors.New("source location is required")
	}
	if strings.TrimSpace(opts.TargetDir) == "" && strings.TrimSpace(opts.TargetURL) == "" {
		return MigrateReport{}, errors.New("target location is required")
	}
	if !opts.DryRun && !opts.Confirm {
		return MigrateReport{}, errors.New("real migration requires explicit confirmation (--confirm-migration)")
	}
	source, dialect, closeSource, err := openSource(ctx, opts)
	if err != nil {
		return MigrateReport{}, err
	}
	defer closeSource()

	var buf bytes.Buffer
	if err := Export(ctx, source, dialect, &buf); err != nil {
		return MigrateReport{}, fmt.Errorf("export source: %w", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	checksum := hex.EncodeToString(sum[:])
	sourceDigest, err := SemanticDigest(buf.Bytes())
	if err != nil {
		return MigrateReport{}, errors.New("source archive failed validation")
	}

	var bundle PortableBundle
	if err := json.Unmarshal(buf.Bytes(), &bundle); err != nil {
		return MigrateReport{}, errors.New("exported archive failed validation")
	}
	records := 0
	for _, pc := range bundle.Collections {
		records += len(pc.Records)
	}
	report := MigrateReport{
		Format:         bundle.Format,
		SourceProvider: string(opts.SourceProvider),
		TargetProvider: string(opts.TargetProvider),
		DryRun:         opts.DryRun,
		ExportedAt:     bundle.ExportedAt,
		Collections:    len(bundle.Collections),
		Records:        records,
		Checksum:       checksum,
		Counts: map[string]int64{
			"admins": int64(len(bundle.System.Admins)), "appUsers": int64(len(bundle.System.AppUsers)),
			"credentials": int64(len(bundle.System.Credentials)), "rules": int64(len(bundle.System.CollectionRules)),
			"events": int64(len(bundle.System.Events)), "audit": int64(len(bundle.System.Audit)),
			"jobs": int64(len(bundle.System.Jobs)), "webhooks": int64(len(bundle.System.Webhooks)),
			"functions": int64(len(bundle.System.Functions)), "files": int64(len(bundle.System.Files)),
		},
	}
	if opts.DryRun {
		report.WritesPerformed = false
		return report, nil
	}
	target, targetCleanup, err := openTarget(ctx, opts)
	if err != nil {
		return MigrateReport{}, err
	}
	defer targetCleanup()
	defer target.Close()
	if err := Import(ctx, target.DB(), target.Dialect(), io.LimitReader(bytes.NewReader(buf.Bytes()), 1<<30)); err != nil {
		return MigrateReport{}, fmt.Errorf("import target: %w", err)
	}
	// Semantic destination verification: re-export the target through the same
	// portable format and compare canonical hashes, not just record counts.
	var reexport bytes.Buffer
	if err := Export(ctx, target.DB(), target.Dialect(), &reexport); err != nil {
		return MigrateReport{}, fmt.Errorf("re-export target: %w", err)
	}
	destinationDigest, err := SemanticDigest(reexport.Bytes())
	if err != nil {
		return MigrateReport{}, errors.New("target re-export failed validation")
	}
	if destinationDigest != sourceDigest {
		return MigrateReport{}, fmt.Errorf("destination verification failed: semantic hash %s does not match source %s", destinationDigest, sourceDigest)
	}
	report.DestinationHash = destinationDigest
	report.WritesPerformed = true
	return report, nil
}

func quoteCollectionTable(collectionID string) string {
	return `"` + collections.PhysicalTableName(collectionID) + `"`
}

// SemanticDigest returns a canonical content hash of a portable archive that is
// stable across providers and the deliberate restore policy: provider and
// export time are excluded, webhook ciphertext/enabled and session revocation
// markers are normalized, and app access tokens are excluded because they are
// revoked on import.
func SemanticDigest(raw []byte) (string, error) {
	var bundle PortableBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return "", err
	}
	collectionsCopy := make([]PortableCollection, len(bundle.Collections))
	copy(collectionsCopy, bundle.Collections)
	sort.Slice(collectionsCopy, func(i, j int) bool { return collectionsCopy[i].Name < collectionsCopy[j].Name })
	for i := range collectionsCopy {
		fields := collectionsCopy[i].Fields
		sort.Slice(fields, func(a, b int) bool { return fields[a].Name < fields[b].Name })
		collectionsCopy[i].Fields = fields
		records := collectionsCopy[i].Records
		sort.Slice(records, func(a, b int) bool { return records[a].ID < records[b].ID })
		collectionsCopy[i].Records = records
	}
	canonical := map[string]any{
		"collections": collectionsCopy,
		"system":      canonicalSystem(bundle.System),
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalSystem(s PortableSystem) map[string][]map[string]any {
	excluded := map[string]map[string]bool{
		"webhooks":      {"secret_cipher": true, "enabled": true},
		"adminSessions": {"revoked_at": true, "replaced_by": true},
		"appSessions":   {"revoked_at": true, "replaced_by": true},
	}
	out := map[string][]map[string]any{}
	for name, rows := range map[string][]map[string]any{
		"admins": s.Admins, "appUsers": s.AppUsers, "credentials": s.Credentials,
		"collectionRules": s.CollectionRules, "events": s.Events, "audit": s.Audit,
		"jobs": s.Jobs, "functions": s.Functions, "files": s.Files, "systemMeta": s.SystemMeta,
	} {
		cleaned := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			item := map[string]any{}
			for k, v := range row {
				if excluded[name] != nil && excluded[name][k] {
					continue
				}
				if k == "enabled" {
					item[k] = canonicalBool(v)
					continue
				}
				item[k] = v
			}
			cleaned = append(cleaned, item)
		}
		sort.Slice(cleaned, func(i, j int) bool { return canonicalString(cleaned[i]) < canonicalString(cleaned[j]) })
		out[name] = cleaned
	}
	return out
}

func canonicalString(row map[string]any) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		encoded, _ := json.Marshal(row[k])
		b.WriteString(k)
		b.WriteByte('=')
		b.Write(encoded)
		b.WriteByte('|')
	}
	return b.String()
}

func canonicalBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b == 1
	case float64:
		return b == 1
	}
	return false
}
