package proxy

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	ch "github.com/dexterhere04/AgentPlane/internal/clickhouse"
	"github.com/dexterhere04/AgentPlane/internal/observability"
)

// TestFailoverPersistedToClickHouse runs a real failover and reads the result
// back from ClickHouse. It is skipped unless CLICKHOUSE_TEST_HOST is set
// (CLICKHOUSE_TEST_PORT defaults to 9000; credentials come from ch.Credentials).
func TestFailoverPersistedToClickHouse(t *testing.T) {
	host := os.Getenv("CLICKHOUSE_TEST_HOST")
	if host == "" {
		t.Skip("CLICKHOUSE_TEST_HOST not set; skipping ClickHouse integration test")
	}
	port := 9000
	if p := os.Getenv("CLICKHOUSE_TEST_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("bad CLICKHOUSE_TEST_PORT %q: %v", p, err)
		}
		port = n
	}

	client, err := ch.New(host, port)
	if err != nil {
		t.Fatalf("connecting to ClickHouse: %v", err)
	}

	// Init() reads the schema from a path relative to the working directory
	// and silently skips if it is missing, so run it from the repo root and
	// fail loudly if the file isn't there.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if _, err := os.Stat("migrations/clickhouse/001_create_tables.sql"); err != nil {
		t.Fatalf("schema file not found from repo root: %v", err)
	}

	adapter := observability.NewClickHouseAdapter(client)
	if err := adapter.Init(); err != nil {
		t.Fatalf("applying schema: %v", err)
	}

	prev := observability.DefaultStore
	observability.SetStore(adapter)
	t.Cleanup(func() { observability.SetStore(prev) })

	requestID := fmt.Sprintf("req-failover-ch-%d", time.Now().UnixNano())
	runFailover(t, requestID)

	// Inserts happen in goroutines; poll until the failover row shows up.
	var (
		failover     uint8
		from, to     string
		attempt      uint32
		found        bool
		lastQueryErr error
	)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !found {
		rows, err := client.Query(context.Background(), `
			SELECT failover, failover_from, failover_to, attempt
			FROM agentplane.traces
			WHERE trace_id = ? AND failover = 1`, requestID)
		if err != nil {
			lastQueryErr = err
		} else {
			if rows.Next() {
				if err := rows.Scan(&failover, &from, &to, &attempt); err != nil {
					lastQueryErr = err
				} else {
					found = true
				}
			}
			rows.Close()
		}
		if !found {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !found {
		t.Fatalf("no failover trace row for %s in ClickHouse (last query error: %v)", requestID, lastQueryErr)
	}
	if from != "primary" || to != "secondary" || attempt != 2 {
		t.Errorf("stored row: %q -> %q attempt=%d, want primary -> secondary attempt=2", from, to, attempt)
	}

	// The query behind the dashboard card should now see the transition too.
	q := observability.NewAnalyticsQueries().FailoverTransitionsQuery(1)
	rows, err := client.Query(context.Background(), q)
	if err != nil {
		t.Fatalf("FailoverTransitionsQuery: %v", err)
	}
	defer rows.Close()
	seen := false
	for rows.Next() {
		var f, tt string
		var n uint64
		if err := rows.Scan(&f, &tt, &n); err != nil {
			t.Fatalf("scanning FailoverTransitionsQuery row: %v", err)
		}
		if f == "primary" && tt == "secondary" && n >= 1 {
			seen = true
		}
	}
	if !seen {
		t.Error("FailoverTransitionsQuery did not return the primary -> secondary transition")
	}
}
