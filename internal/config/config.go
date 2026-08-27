package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen            string
	DataDir           string
	ShutdownTimeout   time.Duration
	LogLevel          string
	StaticDir         string
	StorageBackend    string
	S3Endpoint        string
	S3Region          string
	S3Bucket          string
	S3AccessKey       string
	S3SecretKey       string
	AWSRegion         string
	AWSAccessKey      string
	AWSSecretKey      string
	TrustedProxies    []netip.Prefix
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

func Defaults() Config {
	return Config{Listen: "127.0.0.1:8090", DataDir: "./data", ShutdownTimeout: 10 * time.Second, LogLevel: "info", StorageBackend: "local", S3Region: "us-east-1", ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}

func Load(args []string, getenv func(string) string) (Config, error) {
	cfg := Defaults()
	if value := getenv("TRESTLE_LISTEN"); value != "" {
		cfg.Listen = value
	}
	if value := getenv("TRESTLE_DATA_DIR"); value != "" {
		cfg.DataDir = value
	}
	if value := getenv("TRESTLE_SHUTDOWN_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("TRESTLE_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = duration
	}
	if value := getenv("TRESTLE_LOG_LEVEL"); value != "" {
		cfg.LogLevel = value
	}
	if value := getenv("TRESTLE_STATIC_DIR"); value != "" {
		cfg.StaticDir = value
	}
	if value := getenv("TRESTLE_STORAGE_BACKEND"); value != "" {
		cfg.StorageBackend = value
	}
	cfg.S3Endpoint = getenv("TRESTLE_S3_ENDPOINT")
	if value := getenv("TRESTLE_S3_REGION"); value != "" {
		cfg.S3Region = value
	}
	cfg.S3Bucket = getenv("TRESTLE_S3_BUCKET")
	cfg.S3AccessKey = getenv("TRESTLE_S3_ACCESS_KEY")
	cfg.S3SecretKey = getenv("TRESTLE_S3_SECRET_KEY")
	cfg.AWSRegion = getenv("TRESTLE_AWS_REGION")
	cfg.AWSAccessKey = getenv("TRESTLE_AWS_ACCESS_KEY")
	cfg.AWSSecretKey = getenv("TRESTLE_AWS_SECRET_KEY")
	if value := getenv("TRESTLE_TRUSTED_PROXIES"); value != "" {
		prefixes, err := parsePrefixes(value)
		if err != nil {
			return Config{}, fmt.Errorf("TRESTLE_TRUSTED_PROXIES: %w", err)
		}
		cfg.TrustedProxies = prefixes
	}
	if value := getenv("TRESTLE_READ_HEADER_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("TRESTLE_READ_HEADER_TIMEOUT: %w", err)
		}
		cfg.ReadHeaderTimeout = duration
	}
	if value := getenv("TRESTLE_IDLE_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("TRESTLE_IDLE_TIMEOUT: %w", err)
		}
		cfg.IdleTimeout = duration
	}
	if value := getenv("TRESTLE_READ_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("TRESTLE_READ_TIMEOUT: %w", err)
		}
		cfg.ReadTimeout = duration
	}
	if value := getenv("TRESTLE_MAX_HEADER_BYTES"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("TRESTLE_MAX_HEADER_BYTES: %w", err)
		}
		cfg.MaxHeaderBytes = n
	}

	set := flag.NewFlagSet("trestle", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&cfg.Listen, "listen", cfg.Listen, "listen address")
	set.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "data directory")
	set.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	set.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "debug, info, warn, or error")
	set.StringVar(&cfg.StaticDir, "static-dir", cfg.StaticDir, "development static asset override")
	set.StringVar(&cfg.StorageBackend, "storage-backend", cfg.StorageBackend, "local or s3")
	var trustedProxies string
	if len(cfg.TrustedProxies) > 0 {
		trustedProxies = joinPrefixes(cfg.TrustedProxies)
	}
	set.StringVar(&trustedProxies, "trusted-proxies", trustedProxies, "comma-separated proxy CIDRs allowed to supply forwarded headers")
	set.DurationVar(&cfg.ReadHeaderTimeout, "read-header-timeout", cfg.ReadHeaderTimeout, "HTTP request header timeout")
	set.DurationVar(&cfg.ReadTimeout, "read-timeout", cfg.ReadTimeout, "HTTP request read timeout including request bodies")
	set.DurationVar(&cfg.IdleTimeout, "idle-timeout", cfg.IdleTimeout, "HTTP keep-alive idle timeout")
	set.IntVar(&cfg.MaxHeaderBytes, "max-header-bytes", cfg.MaxHeaderBytes, "maximum HTTP request header bytes")
	if err := set.Parse(args); err != nil {
		return Config{}, err
	}
	if set.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(set.Args(), " "))
	}
	prefixes, err := parsePrefixes(trustedProxies)
	if err != nil {
		return Config{}, fmt.Errorf("trusted proxies: %w", err)
	}
	cfg.TrustedProxies = prefixes
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DataDir) == "" {
		return errors.New("data directory must not be empty")
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen address: %w", err)
	}
	if host == "" {
		return errors.New("listen address must specify a host explicitly")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("listen port must be between 1 and 65535")
	}
	if c.ShutdownTimeout <= 0 || c.ShutdownTimeout > 5*time.Minute {
		return errors.New("shutdown timeout must be greater than zero and no more than 5m")
	}
	if c.ReadHeaderTimeout <= 0 || c.ReadHeaderTimeout > time.Minute {
		return errors.New("read header timeout must be greater than zero and no more than 1m")
	}
	if c.ReadTimeout <= 0 || c.ReadTimeout > time.Hour {
		return errors.New("read timeout must be greater than zero and no more than 1h")
	}
	if c.IdleTimeout <= 0 || c.IdleTimeout > 10*time.Minute {
		return errors.New("idle timeout must be greater than zero and no more than 10m")
	}
	if c.MaxHeaderBytes < 8<<10 || c.MaxHeaderBytes > 4<<20 {
		return errors.New("max header bytes must be between 8192 and 4194304")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("log level must be debug, info, warn, or error")
	}
	if c.StorageBackend != "local" && c.StorageBackend != "s3" {
		return errors.New("storage backend must be local or s3")
	}
	if c.StorageBackend == "s3" {
		if c.S3Endpoint == "" || c.S3Bucket == "" || c.S3AccessKey == "" || c.S3SecretKey == "" {
			return errors.New("S3 endpoint, bucket, access key, and secret key are required")
		}
		if !strings.HasPrefix(c.S3Endpoint, "https://") && !strings.HasPrefix(c.S3Endpoint, "http://127.0.0.1") && !strings.HasPrefix(c.S3Endpoint, "http://localhost") {
			return errors.New("S3 endpoint must use HTTPS except on loopback")
		}
	}
	return nil
}

func FromOS(args []string) (Config, error) { return Load(args, os.Getenv) }

func parsePrefixes(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var out []netip.Prefix
	for _, raw := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", strings.TrimSpace(raw))
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

func joinPrefixes(values []netip.Prefix) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = value.String()
	}
	return strings.Join(parts, ",")
}
