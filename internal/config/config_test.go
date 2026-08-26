package config

import (
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

func TestRejectsAmbiguousOrUnsafeValues(t *testing.T) {
	tests := [][]string{{"--listen", ":8090"}, {"--listen", "localhost:0"}, {"--data-dir", ""}, {"--shutdown-timeout", "0s"}, {"--shutdown-timeout", (6 * time.Minute).String()}, {"--log-level", "verbose"}}
	for _, args := range tests {
		if _, err := Load(args, func(string) string { return "" }); err == nil {
			t.Errorf("Load(%q) succeeded", args)
		}
	}
}
