package config

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrecedenceFlagsEnvironmentDefaults(t *testing.T) {
	env := map[string]string{"TRESTLE_LISTEN": "127.0.0.1:9000", "TRESTLE_LOG_LEVEL": "warn"}
	cfg, err := Load([]string{"--listen", "127.0.0.1:9001"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9001" || cfg.LogLevel != "warn" || cfg.DataDir != "./data" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestDatabaseConfigurationValidationAndPrecedence(t *testing.T) {
	env := map[string]string{"TRESTLE_DATABASE_PROVIDER": "postgres", "TRESTLE_DATABASE_URL": "postgres://user:secret@db.example/trestle?sslmode=require"}
	cfg, err := Load([]string{"--database-max-open", "24", "--database-max-idle", "4"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseProvider != "postgres" || cfg.DatabaseMaxOpen != 24 || !cfg.DatabaseExplicit {
		t.Fatalf("unexpected database config: %#v", cfg)
	}
	if _, err := Load([]string{"--database-provider", "postgres", "--database-url", "postgres://u:p@db.example/x?sslmode=disable"}, func(string) string { return "" }); err == nil {
		t.Fatal("accepted plaintext remote postgres")
	}
	if _, err := Load([]string{"--database-url", "postgres://u:p@localhost/x?sslmode=disable"}, func(string) string { return "" }); err == nil {
		t.Fatal("accepted postgres URL with sqlite provider")
	}
}

func TestDatabaseBootstrapIsAtomicOwnerOnlyAndRedacted(t *testing.T) {
	dir := t.TempDir()
	value := DatabaseBootstrap{Provider: "postgres", URL: "postgres://admin:mudblood@localhost/trestle?sslmode=disable", MaxOpen: 8, MaxIdle: 2, ConnectTimeout: 5 * time.Second, ConnMaxLifetime: time.Hour}
	if err := PersistDatabaseBootstrap(dir, value); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, databaseBootstrapFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions %o", info.Mode().Perm())
	}
	got, found, err := ReadDatabaseBootstrap(dir)
	if err != nil || !found || got.Provider != "postgres" {
		t.Fatalf("got=%#v found=%v err=%v", got, found, err)
	}
	redacted := RedactDatabaseURL(value.URL)
	if strings.Contains(redacted, "mudblood") || !strings.Contains(redacted, "redacted") {
		t.Fatalf("unsafe redaction: %s", redacted)
	}
}

func TestStoredDatabaseBootstrapMarksDatabaseConfigured(t *testing.T) {
	dir := t.TempDir()
	value := DatabaseBootstrap{Provider: "postgres", URL: "postgres://admin:mudblood@localhost/trestle?sslmode=disable", MaxOpen: 8, MaxIdle: 2, ConnectTimeout: 5 * time.Second, ConnMaxLifetime: time.Hour}
	if err := PersistDatabaseBootstrap(dir, value); err != nil {
		t.Fatal(err)
	}
	cfg, err := FromOS([]string{"--data-dir", dir})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DatabaseConfigured || cfg.DatabaseExplicit || cfg.DatabaseProvider != "postgres" {
		t.Fatalf("configured=%v explicit=%v provider=%q", cfg.DatabaseConfigured, cfg.DatabaseExplicit, cfg.DatabaseProvider)
	}
}

func TestTrustedProxyConfiguration(t *testing.T) {
	env := map[string]string{"TRESTLE_TRUSTED_PROXIES": "127.0.0.1/32,10.0.0.0/8"}
	cfg, err := Load(nil, func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[1] != netip.MustParsePrefix("10.0.0.0/8") {
		t.Fatalf("unexpected proxies: %v", cfg.TrustedProxies)
	}
	if _, err := Load([]string{"--trusted-proxies", "not-a-cidr"}, func(string) string { return "" }); err == nil {
		t.Fatal("invalid proxy accepted")
	}
}

func TestHTTPBoundaryValidation(t *testing.T) {
	for _, args := range [][]string{{"--read-header-timeout", "0s"}, {"--read-timeout", "61m"}, {"--idle-timeout", "11m"}, {"--max-header-bytes", "1024"}} {
		if _, err := Load(args, func(string) string { return "" }); err == nil {
			t.Fatalf("Load(%q) succeeded", args)
		}
	}
}

func TestDefaultListener(t *testing.T) {
	cfg, err := Load(nil, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:7333" {
		t.Fatalf("default listener = %q want 127.0.0.1:7333", cfg.Listen)
	}
}

func TestHostPortFlagsAcceptedButNotApplied(t *testing.T) {
	// The durable config layer accepts --host/--port so the command line parses,
	// but does not apply them: the CLI resolves the listener and overrides the
	// durable config in memory when an explicit host/port was selected.
	cfg, err := Load([]string{"--host", "0.0.0.0", "--port", "9000", "--data-dir", "./data"}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:7333" {
		t.Fatalf("durable listen must stay the default: %q", cfg.Listen)
	}
	// The legacy --listen flag keeps its durable-config behavior.
	cfg, err = Load([]string{"--listen", "127.0.0.1:9001"}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9001" {
		t.Fatalf("legacy --listen not applied: %q", cfg.Listen)
	}
}

func TestExplicitHostPortOverridesLegacyListenEnv(t *testing.T) {
	// An explicit --host/--port selection overrides a conflicting legacy
	// TRESTLE_LISTEN durable value, so a malformed legacy value cannot break the
	// explicit selection.
	env := map[string]string{"TRESTLE_LISTEN": "not-a-listen", "TRESTLE_LOG_LEVEL": "warn"}
	cfg, err := Load([]string{"--host", "0.0.0.0", "--port", "9000"}, func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("explicit host/port must override malformed TRESTLE_LISTEN: %v", err)
	}
	if cfg.Listen != "127.0.0.1:7333" {
		t.Fatalf("durable listen = %q want default", cfg.Listen)
	}
	// TRESTLE_HOST/TRESTLE_PORT environment selects the new form too.
	env = map[string]string{"TRESTLE_LISTEN": "not-a-listen", "TRESTLE_PORT": "9001"}
	cfg, err = Load(nil, func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("TRESTLE_PORT env must override malformed TRESTLE_LISTEN: %v", err)
	}
	if cfg.Listen != "127.0.0.1:7333" {
		t.Fatalf("durable listen = %q want default", cfg.Listen)
	}
	// Bare invocation with a malformed legacy value still fails (no fallback).
	if _, err := Load(nil, func(key string) string { return map[string]string{"TRESTLE_LISTEN": "not-a-listen"}[key] }); err == nil {
		t.Fatal("malformed TRESTLE_LISTEN on a bare invocation accepted")
	}
}

func TestRejectsAmbiguousOrUnsafeValues(t *testing.T) {
	tests := [][]string{{"--listen", ":7333"}, {"--listen", "localhost:0"}, {"--data-dir", ""}, {"--shutdown-timeout", "0s"}, {"--shutdown-timeout", (6 * time.Minute).String()}, {"--log-level", "verbose"}, {"--database-connect-timeout", "500ms"}}
	for _, args := range tests {
		if _, err := Load(args, func(string) string { return "" }); err == nil {
			t.Errorf("Load(%q) succeeded", args)
		}
	}
}
