package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const databaseBootstrapFile = "database.json"

type DatabaseBootstrap struct {
	Provider        string        `json:"provider"`
	URL             string        `json:"url,omitempty"`
	MaxOpen         int           `json:"maxOpen"`
	MaxIdle         int           `json:"maxIdle"`
	ConnectTimeout  time.Duration `json:"connectTimeout"`
	ConnMaxLifetime time.Duration `json:"connMaxLifetime"`
}

func (d DatabaseBootstrap) validate() error {
	cfg := Defaults()
	cfg.DatabaseProvider, cfg.DatabaseURL = d.Provider, d.URL
	cfg.DatabaseMaxOpen, cfg.DatabaseMaxIdle = d.MaxOpen, d.MaxIdle
	cfg.DatabaseConnectTimeout, cfg.DatabaseConnMaxLifetime = d.ConnectTimeout, d.ConnMaxLifetime
	return cfg.Validate()
}

func PersistDatabaseBootstrap(dataDir string, value DatabaseBootstrap) error {
	if err := value.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return fmt.Errorf("secure data directory: %w", err)
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.New("encode database bootstrap")
	}
	path := filepath.Join(dataDir, databaseBootstrapFile)
	temporary := path + ".new"
	if err := os.WriteFile(temporary, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write database bootstrap: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("secure database bootstrap: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish database bootstrap: %w", err)
	}
	return nil
}

func ReadDatabaseBootstrap(dataDir string) (DatabaseBootstrap, bool, error) {
	path := filepath.Join(dataDir, databaseBootstrapFile)
	encoded, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DatabaseBootstrap{}, false, nil
	}
	if err != nil {
		return DatabaseBootstrap{}, false, fmt.Errorf("read database bootstrap: %w", err)
	}
	var value DatabaseBootstrap
	if err := json.Unmarshal(encoded, &value); err != nil {
		return DatabaseBootstrap{}, false, errors.New("stored database configuration is invalid")
	}
	if err := value.validate(); err != nil {
		return DatabaseBootstrap{}, false, fmt.Errorf("stored database configuration: %w", err)
	}
	return value, true, nil
}
