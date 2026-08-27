package store

import "fmt"

// Provider identifies a database implementation without exposing connection
// material to handlers or diagnostics.
type Provider string

const (
	SQLite   Provider = "sqlite"
	Postgres Provider = "postgres"
)

func ParseProvider(value string) (Provider, error) {
	switch Provider(value) {
	case SQLite:
		return SQLite, nil
	case Postgres:
		return Postgres, nil
	default:
		return "", fmt.Errorf("database provider must be sqlite or postgres")
	}
}

type Diagnostics struct {
	Provider      Provider `json:"provider"`
	SchemaVersion int      `json:"schemaVersion"`
	MaxOpen       int      `json:"maxOpenConnections"`
}
