package collections

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/trestle-dev/trestle/internal/store"
)

func PhysicalTableName(collectionID string) string {
	sum := sha256.Sum256([]byte(collectionID))
	return "_trestle_data_" + hex.EncodeToString(sum[:8])
}
func physicalColumn(fieldID string) string {
	sum := sha256.Sum256([]byte(fieldID))
	return "f_" + hex.EncodeToString(sum[:8])
}
func PhysicalColumnName(fieldID string) string { return physicalColumn(fieldID) }
func quote(identifier string) string           { return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"` }

func createPhysical(ctx context.Context, tx store.Transaction, collectionID string, fields []Field) error {
	parts := []string{`_id TEXT PRIMARY KEY`, `_version INTEGER NOT NULL DEFAULT 1 CHECK(_version > 0)`, `_created TEXT NOT NULL`, `_updated TEXT NOT NULL`}
	for _, f := range fields {
		parts = append(parts, columnDDL(f))
	}
	_, err := tx.ExecContext(ctx, "CREATE TABLE "+quote(PhysicalTableName(collectionID))+" ("+strings.Join(parts, ",")+") STRICT")
	return err
}

func columnDDL(f Field) string {
	kind := "TEXT"
	switch f.Type {
	case "number":
		kind = "REAL"
	case "boolean":
		kind = "INTEGER CHECK(" + quote(physicalColumn(f.ID)) + " IN (0,1))"
	case "json":
		kind = "TEXT CHECK(json_valid(" + quote(physicalColumn(f.ID)) + "))"
	}
	part := quote(physicalColumn(f.ID)) + " " + kind
	if f.Required {
		part += " NOT NULL"
	}
	if f.Unique {
		part += " UNIQUE"
	}
	return part
}

func rebuildPhysical(ctx context.Context, tx store.Transaction, collectionID string, oldFields, newFields []Field) error {
	table := PhysicalTableName(collectionID)
	temporary := table + "_next"
	if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS "+quote(temporary)); err != nil {
		return err
	}
	parts := []string{`_id TEXT PRIMARY KEY`, `_version INTEGER NOT NULL DEFAULT 1 CHECK(_version > 0)`, `_created TEXT NOT NULL`, `_updated TEXT NOT NULL`}
	for _, f := range newFields {
		parts = append(parts, columnDDL(f))
	}
	if _, err := tx.ExecContext(ctx, "CREATE TABLE "+quote(temporary)+" ("+strings.Join(parts, ",")+") STRICT"); err != nil {
		return err
	}
	old := map[string]bool{}
	for _, f := range oldFields {
		old[f.ID] = true
	}
	to := []string{"_id", "_version", "_created", "_updated"}
	from := append([]string{}, to...)
	for _, f := range newFields {
		if old[f.ID] {
			column := physicalColumn(f.ID)
			to = append(to, quote(column))
			from = append(from, quote(column))
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO "+quote(temporary)+" ("+strings.Join(to, ",")+") SELECT "+strings.Join(from, ",")+" FROM "+quote(table)); err != nil {
		return fmt.Errorf("copy compatible record data: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE "+quote(table)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "ALTER TABLE "+quote(temporary)+" RENAME TO "+quote(table))
	return err
}

func destructiveChanges(oldFields, newFields []Field) []string {
	old := map[string]Field{}
	for _, f := range oldFields {
		old[f.ID] = f
	}
	seen := map[string]bool{}
	changes := []string{}
	for _, f := range newFields {
		if before, ok := old[f.ID]; ok {
			seen[f.ID] = true
			if before.Type != f.Type {
				changes = append(changes, "change type of "+before.Name)
			}
			if !before.Required && f.Required {
				changes = append(changes, "make "+f.Name+" required")
			}
		}
	}
	for _, f := range oldFields {
		if !seen[f.ID] {
			changes = append(changes, "remove "+f.Name)
		}
	}
	return changes
}
