// This file adds database configuration to the existing internal/config
// package. Kept in its own file (like apikey_pepper.go) so it drops into
// internal/config/ without touching your existing config.go.
package config

import (
	"fmt"
	"os"
)

// databaseURLEnvVar is the environment variable holding the Postgres
// connection string, e.g.:
//
//	DATABASE_URL=postgres://user:password@localhost:5432/agentplane?sslmode=disable
const databaseURLEnvVar = "DATABASE_URL"

// DatabaseURL returns the Postgres connection string used to create the
// shared connection pool (see internal/db). It is read from the
// DATABASE_URL environment variable, following the same pattern as
// OpenAIKey() and KeyPepper().
//
// DatabaseURL returns an error if the environment variable is unset or
// empty, so the application fails fast at startup rather than trying to
// connect with an empty DSN.
func DatabaseURL() (string, error) {
	url := os.Getenv(databaseURLEnvVar)
	if url == "" {
		return "", fmt.Errorf("config: %s environment variable is not set", databaseURLEnvVar)
	}
	return url, nil
}
