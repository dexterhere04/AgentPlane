package observability

import (
	"strings"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	script := `
-- Create database
CREATE DATABASE IF NOT EXISTS agentplane;

-- a table spanning multiple lines
CREATE TABLE IF NOT EXISTS agentplane.foo (
	id String
) ENGINE = MergeTree() ORDER BY id;

ALTER TABLE agentplane.foo ADD INDEX IF NOT EXISTS idx_id (id) TYPE bloom_filter GRANULARITY 1;
`
	stmts := splitSQLStatements(script)
	if len(stmts) != 3 {
		t.Fatalf("got %d statements, want 3: %#v", len(stmts), stmts)
	}
	if stmts[0] != "CREATE DATABASE IF NOT EXISTS agentplane" {
		t.Fatalf("stmt[0] = %q", stmts[0])
	}
	if !strings.Contains(stmts[1], "CREATE TABLE") {
		t.Fatalf("stmt[1] = %q", stmts[1])
	}
	if !strings.Contains(stmts[2], "ADD INDEX") {
		t.Fatalf("stmt[2] = %q", stmts[2])
	}
}

func TestSplitSQLStatementsEmpty(t *testing.T) {
	if got := splitSQLStatements(""); len(got) != 0 {
		t.Fatalf("expected no statements, got %#v", got)
	}
	if got := splitSQLStatements("-- only a comment"); len(got) != 0 {
		t.Fatalf("expected no statements, got %#v", got)
	}
}
