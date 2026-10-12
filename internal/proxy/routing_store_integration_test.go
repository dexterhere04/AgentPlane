//go:build integration

package proxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/db"
	"github.com/jackc/pgx/v5"
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
	t.Cleanup(func() {
		pool.Close()
	})

	store := NewRoutingRuleStore(pool)

	name := fmt.Sprintf("integration-routing-%d", time.Now().UnixNano())
	created, err := store.Create(ctx, RoutingRule{
		Name:          name,
		Priority:      0,
		ModelMatcher:  []byte(`{"equals":"gpt-4o"}`),
		RoleMatcher:   []byte(`{"equals":"member"}`),
		UserMatcher:   []byte(`{"usernames":["store-test-user"]}`),
		Action:        "route",
		ProviderGroup: "test",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create routing rule: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(context.Background(), created.ID); err != nil {
			t.Errorf("delete test rule: %v", err)
		}
	})

	ruleSet, err := store.LoadRuleSet(ctx)
	if err != nil {
		t.Fatalf("load routing rules: %v", err)
	}

	rule, matched, err := ruleSet.Match(RouteRequest{
		Model:    "gpt-4o",
		Username: "store-test-user",
		Roles:    []string{"member"},
	})
	if err != nil {
		t.Fatalf("match routing rule: %v", err)
	}
	if !matched {
		t.Fatal("expected integration routing rule to match")
	}
	if rule.Name != name {
		t.Fatalf("expected %q, got %q", name, rule.Name)
	}
	if rule.Action != "route" {
		t.Fatalf("expected action route, got %q", rule.Action)
	}
	if rule.ProviderGroup != "test" {
		t.Fatalf("expected provider group test, got %q", rule.ProviderGroup)
	}
}

func TestRoutingRuleStoreCRUD(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	store := NewRoutingRuleStore(pool)
	name := fmt.Sprintf("integration-crud-%d", time.Now().UnixNano())

	input := RoutingRule{
		Name:          name,
		Priority:      50,
		ModelMatcher:  []byte(`{"equals":"gpt-4o"}`),
		RoleMatcher:   []byte(`{"equals":"member"}`),
		UserMatcher:   []byte(`{"usernames":["crud-test-user"]}`),
		Action:        "route",
		ProviderGroup: "test",
		Enabled:       true,
	}

	created, err := store.Create(ctx, input)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(context.Background(), created.ID); err != nil &&
			err != pgx.ErrNoRows {
			t.Errorf("cleanup rule: %v", err)
		}
	})

	if _, err := store.Create(ctx, input); err == nil {
		t.Fatal("expected duplicate name to be rejected")
	}

	got, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != name {
		t.Fatalf("Get name = %q, want %q", got.Name, name)
	}

	rules, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, rule := range rules {
		if rule.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("List did not include created rule")
	}

	updatedInput := *created
	updatedInput.Name = name + "-updated"
	updatedInput.Priority = 51
	updatedInput.ProviderGroup = "test"

	updated, err := store.Update(ctx, created.ID, updatedInput)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != updatedInput.Name || updated.Priority != 51 {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	if err := store.SetEnabled(ctx, created.ID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	got, err = store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after disable: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected rule to be disabled")
	}

	if err := store.SetEnabled(ctx, created.ID, true); err != nil {
		t.Fatalf("enable: %v", err)
	}

	invalid := input
	invalid.Name = name + "-invalid"
	invalid.ModelMatcher = []byte(`{"equals":`)
	if _, err := store.Create(ctx, invalid); err == nil {
		t.Fatal("expected invalid matcher to be rejected")
	}

	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, created.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Get after delete: got %v, want pgx.ErrNoRows", err)
	}

	if _, err := store.Update(ctx, "00000000-0000-0000-0000-000000000001", input); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Update nonexistent ID: got %v, want pgx.ErrNoRows", err)
	}
	if err := store.Delete(ctx, "00000000-0000-0000-0000-000000000001"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Delete nonexistent ID: got %v, want pgx.ErrNoRows", err)
	}
}
