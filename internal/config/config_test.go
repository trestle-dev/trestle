package config

import (
	"net/netip"
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

func TestRejectsAmbiguousOrUnsafeValues(t *testing.T) {
	tests := [][]string{{"--listen", ":8090"}, {"--listen", "localhost:0"}, {"--data-dir", ""}, {"--shutdown-timeout", "0s"}, {"--shutdown-timeout", (6 * time.Minute).String()}, {"--log-level", "verbose"}}
	for _, args := range tests {
		if _, err := Load(args, func(string) string { return "" }); err == nil {
			t.Errorf("Load(%q) succeeded", args)
		}
	}
}
