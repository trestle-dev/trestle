package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	Counts          map[string]int64 `json:"counts"`
	WritesPerformed bool             `json:"writesPerformed"`
}

func openStore(ctx context.Context, provider store.Provider, dataDir, url string) (*store.Store, error) {
	if dataDir == "" {
		dir, err := os.MkdirTemp("", "trestle-migrate-*")
		if err != nil {
			return nil, err
		}
		dataDir = dir
		defer os.RemoveAll(dir)
	}
	return store.OpenWith(ctx, store.Options{DataDir: dataDir, Provider: provider, URL: url, MaxOpen: 4, MaxIdle: 1, ConnectTimeout: 10 * time.Second})
}

func openSource(ctx context.Context, opts MigrateOptions) (*store.Store, error) {
	s, err := openStore(ctx, opts.SourceProvider, opts.SourceDir, opts.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	if s.Diagnostics().SchemaVersion != store.CurrentVersion {
		s.Close()
		return nil, errors.New("source database is not at the current schema version; upgrade it first so the migration never writes to it")
	}
	return s, nil
}

func openTarget(ctx context.Context, opts MigrateOptions) (*store.Store, error) {
	s, err := openStore(ctx, opts.TargetProvider, opts.TargetDir, opts.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("open target: %w", err)
	}
	return s, nil
}

// Migrate moves a database between providers using the portable logical
// archive. The source is only read (it must be at the current schema version);
// the target must be an initialized empty database. A dry run exports and
// validates without writing to the target. The report carries counts and a
// content checksum for pre-cutover verification.
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
	source, err := openSource(ctx, opts)
	if err != nil {
		return MigrateReport{}, err
	}
	defer source.Close()

	var buf bytes.Buffer
	if err := Export(ctx, source.DB(), source.Dialect(), &buf); err != nil {
		return MigrateReport{}, fmt.Errorf("export source: %w", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	checksum := hex.EncodeToString(sum[:])

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
	target, err := openTarget(ctx, opts)
	if err != nil {
		return MigrateReport{}, err
	}
	defer target.Close()
	if err := Import(ctx, target.DB(), target.Dialect(), io.LimitReader(bytes.NewReader(buf.Bytes()), 1<<30)); err != nil {
		return MigrateReport{}, fmt.Errorf("import target: %w", err)
	}
	var importedRecords int
	collections, err := target.DB().QueryContext(ctx, "SELECT id FROM _trestle_collections")
	if err != nil {
		return MigrateReport{}, fmt.Errorf("verify target: %w", err)
	}
	var colIDs []string
	for collections.Next() {
		var colID string
		if err := collections.Scan(&colID); err != nil {
			collections.Close()
			return MigrateReport{}, err
		}
		colIDs = append(colIDs, colID)
	}
	if err := collections.Close(); err != nil {
		return MigrateReport{}, err
	}
	for _, colID := range colIDs {
		var count int
		if err := target.DB().QueryRowContext(ctx, "SELECT count(*) FROM "+quoteCollectionTable(colID)).Scan(&count); err != nil {
			return MigrateReport{}, fmt.Errorf("verify target records: %w", err)
		}
		importedRecords += count
	}
	if importedRecords != records {
		return MigrateReport{}, fmt.Errorf("target record count %d does not match source %d", importedRecords, records)
	}
	report.WritesPerformed = true
	return report, nil
}

func quoteCollectionTable(collectionID string) string {
	return `"` + collections.PhysicalTableName(collectionID) + `"`
}
