package store

import (
	"context"
	"testing"
)

func TestProviderDiagnosticsDoNotExposeConnectionMaterial(t *testing.T) {
	s, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d := s.Diagnostics()
	if d.Provider != SQLite || d.SchemaVersion != CurrentVersion || d.MaxOpen != 1 {
		t.Fatalf("unexpected diagnostics: %#v", d)
	}
}

func TestParseProvider(t *testing.T) {
	for _, value := range []string{"sqlite", "postgres"} {
		if _, err := ParseProvider(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	if _, err := ParseProvider("mysql"); err == nil {
		t.Fatal("accepted unsupported provider")
	}
}
