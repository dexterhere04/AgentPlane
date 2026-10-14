package observability

import (
	"strings"
	"testing"
	"time"
)

func TestParseAnalyticsRange(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.FixedZone("test", 3600))
	tests := []struct {
		value string
		span  time.Duration
	}{
		{"15m", 15 * time.Minute},
		{"1h", time.Hour},
		{"6h", 6 * time.Hour},
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			from, to, err := ParseAnalyticsRange(test.value, now)
			if err != nil {
				t.Fatal(err)
			}
			if got := to.Sub(from); got != test.span {
				t.Fatalf("range span = %s, want %s", got, test.span)
			}
			if to.Location() != time.UTC {
				t.Fatalf("range end location = %s, want UTC", to.Location())
			}
		})
	}
	if _, _, err := ParseAnalyticsRange("30d", now); err == nil {
		t.Fatal("unsupported range was accepted")
	}
}

func TestAnalyticsFilterSQLBindsAllFilters(t *testing.T) {
	from := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	filter := AnalyticsFilter{
		From: from, To: to, Status: "error", Provider: "provider-value", Model: "model-value",
		Route: "/route-value", GuardrailAction: "blocked", UserID: "user-value",
		OrganizationID: "org-value", ProjectID: "project-value",
	}
	where, args := analyticsFilterSQL(filter, "t")
	if strings.Contains(where, "provider-value") || strings.Contains(where, "model-value") || strings.Contains(where, "user-value") {
		t.Fatalf("filter values were interpolated into SQL: %s", where)
	}
	if !strings.Contains(where, "t.is_error = 1") || !strings.Contains(where, "t.effective_guardrail_action = ?") {
		t.Fatalf("error/guardrail filter expressions missing: %s", where)
	}
	if len(args) != 9 {
		t.Fatalf("got %d bound args, want 9: %#v", len(args), args)
	}
	for _, value := range []string{"provider-value", "model-value", "/route-value", "user-value", "org-value", "project-value", "blocked"} {
		found := false
		for _, arg := range args {
			if arg == value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("filter value %q not bound", value)
		}
	}
}

func TestBucketExpressionUsesFixedIntervals(t *testing.T) {
	for _, test := range []struct {
		rangeValue AnalyticsRange
		label      string
	}{
		{Range15Minutes, "1m"}, {Range1Hour, "1m"}, {Range6Hours, "5m"},
		{Range24Hours, "15m"}, {Range7Days, "1h"},
	} {
		expression, label := bucketExpression(test.rangeValue)
		if label != test.label || !strings.Contains(expression, "INTERVAL") {
			t.Errorf("bucket for %s = (%q, %q), want fixed interval %q", test.rangeValue, expression, label, test.label)
		}
	}
}
