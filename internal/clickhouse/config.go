package clickhouse

import (
	"os"
	"strconv"
)

// Config holds ClickHouse connection settings.
//
// Retention is configured via ClickHouse TTL directives in the migration
// (migrations/clickhouse/001_create_tables.sql), not here. Capture modes and
// sampling are configured via the observability package (OBSERVE_* env vars).
type Config struct {
	Host    string
	Port    int
	Enabled bool
}

// DefaultConfig returns sensible defaults. Observability is disabled by
// default and enabled when CLICKHOUSE_HOST is set.
func DefaultConfig() Config {
	return Config{
		Host:    "localhost",
		Port:    9000,
		Enabled: false,
	}
}

// LoadFromEnv returns a Config populated from environment variables, falling
// back to DefaultConfig values.
//
//	CLICKHOUSE_HOST    — ClickHouse server host (enables observability when set)
//	CLICKHOUSE_PORT    — native protocol port (default 9000)
//	CLICKHOUSE_ENABLED — explicit on/off override
func LoadFromEnv() Config {
	c := DefaultConfig()
	if v := os.Getenv("CLICKHOUSE_HOST"); v != "" {
		c.Host = v
		c.Enabled = true
	}
	if v := os.Getenv("CLICKHOUSE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Port = n
		}
	}
	if v := os.Getenv("CLICKHOUSE_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Enabled = b
		}
	}
	return c
}
