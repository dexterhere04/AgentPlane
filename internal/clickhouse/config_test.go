package clickhouse

import "testing"

func TestDefaultConfigDisabled(t *testing.T) {
	c := DefaultConfig()
	if c.Enabled {
		t.Fatalf("DefaultConfig should be disabled by default")
	}
	if c.Port != 9000 {
		t.Fatalf("DefaultConfig port = %d, want 9000", c.Port)
	}
}

func TestLoadFromEnvEnablesOnHost(t *testing.T) {
	t.Setenv("CLICKHOUSE_HOST", "example.com")
	t.Setenv("CLICKHOUSE_PORT", "9999")
	c := LoadFromEnv()
	if !c.Enabled {
		t.Fatalf("expected enabled when CLICKHOUSE_HOST is set")
	}
	if c.Host != "example.com" {
		t.Fatalf("host = %q, want example.com", c.Host)
	}
	if c.Port != 9999 {
		t.Fatalf("port = %d, want 9999", c.Port)
	}
}

func TestLoadFromEnvDisabledOverride(t *testing.T) {
	t.Setenv("CLICKHOUSE_HOST", "example.com")
	t.Setenv("CLICKHOUSE_ENABLED", "false")
	c := LoadFromEnv()
	if c.Enabled {
		t.Fatalf("expected disabled when CLICKHOUSE_ENABLED=false")
	}
}
