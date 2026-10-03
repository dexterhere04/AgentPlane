package proxy

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(nil); err == nil {
		t.Fatal("nil upstream must be rejected")
	}
	if err := r.Register(&Upstream{Name: "x"}); err == nil {
		t.Fatal("nil provider must be rejected")
	}
	if err := r.Register(&Upstream{Provider: &stubProvider{}}); err == nil {
		t.Fatal("empty name must be rejected")
	}

	a := NewUpstream("a", &stubProvider{})
	b := NewUpstream("b", &stubProvider{})
	if err := r.Register(a); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(b); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(NewUpstream("a", &stubProvider{})); !errors.Is(err, ErrDuplicateUpstream) {
		t.Fatalf("want ErrDuplicateUpstream, got %v", err)
	}

	if got, ok := r.Get("a"); !ok || got != a {
		t.Fatal("Get(a) failed")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("Get should miss unknown name")
	}
	if names := r.Names(); len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("names=%v, want registration order [a b]", names)
	}

	if !r.Unregister("a") || r.Unregister("a") {
		t.Fatal("Unregister should succeed once")
	}
	if names := r.Names(); len(names) != 1 || names[0] != "b" || r.Len() != 1 {
		t.Fatalf("names=%v after unregister", names)
	}
}

func TestRegisterProviderUsesProviderName(t *testing.T) {
	r := NewRegistry()
	u, err := r.RegisterProvider(&stubProvider{}, WithWeight(3))
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "stub" || u.Weight != 3 {
		t.Fatalf("got name=%q weight=%d", u.Name, u.Weight)
	}
}

func TestCandidatesFiltersProviderGroup(t *testing.T) {
	r := NewRegistry()

	groupA := NewUpstream(
		"a",
		&stubProvider{},
		WithProviderGroup("group-a"),
		WithModels("gpt-*"),
	)

	groupB := NewUpstream(
		"b",
		&stubProvider{},
		WithProviderGroup("group-b"),
		WithModels("gpt-*"),
	)

	if err := r.Register(groupA); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(groupB); err != nil {
		t.Fatal(err)
	}

	got := r.Candidates("gpt-4o", "group-a")

	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}

	if got[0].Name != "a" {
		t.Fatalf("expected group-a upstream, got %q", got[0].Name)
	}
}

func TestSetEnabledUnknown(t *testing.T) {
	r := NewRegistry()
	if err := r.SetEnabled("ghost", false); !errors.Is(err, ErrUpstreamNotFound) {
		t.Fatalf("want ErrUpstreamNotFound, got %v", err)
	}
}

func TestCandidatesFiltering(t *testing.T) {
	r := NewRegistry()
	openai := NewUpstream("openai", &stubProvider{}, WithModels("gpt-*"))
	anthropic := NewUpstream("anthropic", &stubProvider{}, WithModels("claude-*"))
	fallback := NewUpstream("any", &stubProvider{}) // no model list = serves all
	for _, u := range []*Upstream{openai, anthropic, fallback} {
		if err := r.Register(u); err != nil {
			t.Fatal(err)
		}
	}

	names := func(us []*Upstream) []string {
		var out []string
		for _, u := range us {
			out = append(out, u.Name)
		}
		return out
	}
	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	if got := names(r.Candidates("gpt-4o", "")); !eq(got, []string{"openai", "any"}) {
		t.Fatalf("gpt-4o candidates=%v", got)
	}
	if got := names(r.Candidates("claude-3", "")); !eq(got, []string{"anthropic", "any"}) {
		t.Fatalf("claude-3 candidates=%v", got)
	}

	if err := r.SetEnabled("openai", false); err != nil {
		t.Fatal(err)
	}
	if got := names(r.Candidates("gpt-4o", "")); !eq(got, []string{"any"}) {
		t.Fatalf("after disable: %v", got)
	}

	// Trip the fallback's breaker: it must drop out of candidates too.
	trip := NewCircuitBreaker(BreakerConfig{FailureThreshold: 1, Cooldown: time.Hour})
	trip.RecordFailure()
	fallback.Breaker = trip
	if got := r.Candidates("gpt-4o", ""); len(got) != 0 {
		t.Fatalf("expected no candidates, got %v", names(got))
	}
}

func TestRegistryConcurrentUse(t *testing.T) {
	r := NewRegistry()
	base := NewUpstream("base", &stubProvider{})
	if err := r.Register(base); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			_ = r.Register(NewUpstream(fmt.Sprintf("u%d", i), &stubProvider{}))
		}(i)
		go func() {
			defer wg.Done()
			_ = r.Candidates("gpt-4o", "")
			_ = r.Statuses()
		}()
		go func(i int) {
			defer wg.Done()
			_ = r.SetEnabled("base", i%2 == 0)
		}(i)
	}
	wg.Wait()

	if r.Len() != 21 {
		t.Fatalf("Len=%d want 21", r.Len())
	}
}
