package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen          string
	DataDir         string
	ShutdownTimeout time.Duration
	LogLevel        string
	StaticDir       string
	StorageBackend  string
	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	AWSRegion       string
	AWSAccessKey    string
	AWSSecretKey    string
}

func Defaults() Config {
	return Config{Listen: "127.0.0.1:8090", DataDir: "./data", ShutdownTimeout: 10 * time.Second, LogLevel: "info", StorageBackend: "local", S3Region: "us-east-1"}
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

	set := flag.NewFlagSet("trestle", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&cfg.Listen, "listen", cfg.Listen, "listen address")
	set.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "data directory")
	set.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	set.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "debug, info, warn, or error")
	set.StringVar(&cfg.StaticDir, "static-dir", cfg.StaticDir, "development static asset override")
	set.StringVar(&cfg.StorageBackend, "storage-backend", cfg.StorageBackend, "local or s3")
	if err := set.Parse(args); err != nil {
		return Config{}, err
	}
	if set.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(set.Args(), " "))
	}
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
