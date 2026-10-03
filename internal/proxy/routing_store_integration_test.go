//go:build integration

package proxy

import (
	"context"
	"os"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/db"
)

func TestRoutingRuleStoreLoadsFromPostgres(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	store := NewRoutingRuleStore(pool)

	ruleSet, err := store.LoadRuleSet(ctx)
	if err != nil {
		t.Fatalf("load routing rules: %v", err)
	}

	rule, matched, err := ruleSet.Match(RouteRequest{
		Model:    "gpt-4o",
		UserID:   "test-user",
		Username: "test-user",
	})
	if err != nil {
		t.Fatalf("match routing rule: %v", err)
	}

	if !matched {
		t.Fatal("expected test-gpt4-routing to match")
	}

	if rule.Name != "test-gpt4-routing" {
		t.Fatalf("expected test-gpt4-routing, got %q", rule.Name)
	}

	if rule.Action != "route" {
		t.Fatalf("expected action route, got %q", rule.Action)
	}

	if rule.ProviderGroup != "test-group" {
		t.Fatalf("expected provider group test-group, got %q", rule.ProviderGroup)
	}
}
