package dashboard

import (
	"strings"
	"testing"
)

func TestObservabilityHTMLContract(t *testing.T) {
	for _, expected := range []string{
		"href=\"/dashboard\"", "href=\"/observability\"", "var filterState=",
		"/api/observability/health", "/api/observability/overview", "/api/observability/series",
		"/api/observability/breakdowns", "/api/observability/failures", "/api/observability/guardrails",
		"/api/observability/traces", "Authorization", "data_state",
		"disabled", "hash_only", "sampled", "Full capture is stored",
	} {
		if !strings.Contains(ObservabilityHTML, expected) {
			t.Errorf("observability page does not include %q", expected)
		}
	}
	for _, forbidden := range []string{"localStorage", "sessionStorage", "console.log", "prompt_blob", "response_blob"} {
		if strings.Contains(ObservabilityHTML, forbidden) {
			t.Errorf("observability page includes forbidden reference %q", forbidden)
		}
	}
}

func TestLiveDashboardNavigationAdded(t *testing.T) {
	if !strings.Contains(HTML, `href="/observability"`) || !strings.Contains(HTML, `href="/dashboard"`) {
		t.Fatal("live dashboard navigation does not link both views")
	}
	if !strings.Contains(HTML, "new EventSource('/events')") || !strings.Contains(HTML, "function sendChatMessage()") ||
		!strings.Contains(HTML, `id="chat-api-key"`) || !strings.Contains(HTML, "headers.Authorization='Bearer '+apiKey") {
		t.Fatal("live dashboard SSE or authenticated chat behavior is missing")
	}
	if !strings.Contains(HTML, "escHtml(evt.message||'')") {
		t.Fatal("live event messages are not rendered as escaped text")
	}
}
