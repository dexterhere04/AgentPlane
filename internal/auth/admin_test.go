package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminMiddlewareQuery(t *testing.T) {
	const adminToken = "secret-admin-token"

	tests := []struct {
		name           string
		configured     string
		target         string
		header         string
		wantStatus     int
		wantNextCalled bool
	}{
		{
			name:           "valid token in header",
			configured:     adminToken,
			target:         "/events",
			header:         "Bearer " + adminToken,
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "valid token in query",
			configured:     adminToken,
			target:         "/events?token=" + adminToken,
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "wrong token in header",
			configured:     adminToken,
			target:         "/events",
			header:         "Bearer wrong-token",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "wrong token in query",
			configured:     adminToken,
			target:         "/events?token=wrong-token",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "no token",
			configured:     adminToken,
			target:         "/events",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "empty configured token and empty request token fails closed",
			configured:     "",
			target:         "/events",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
			})

			handler := AdminMiddlewareQuery(tt.configured, next)

			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			if nextCalled != tt.wantNextCalled {
				t.Fatalf("expected nextCalled=%v, got %v", tt.wantNextCalled, nextCalled)
			}
		})
	}
}
