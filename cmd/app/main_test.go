package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFullHealthHandlerReturnsDependencyAwarePayloadForPublicRequests(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/healthcheck", nil)
	req.RemoteAddr = "203.0.113.10:12345"

	rec := httptest.NewRecorder()
	fullHealthHandler(nil, nil, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("fullHealthHandler() status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"db":"disabled"`) {
		t.Fatalf("fullHealthHandler() public payload missing db readiness: %s", body)
	}
	if !strings.Contains(body, `"vision_analyzer":"disabled"`) {
		t.Fatalf("fullHealthHandler() public payload missing analyzer readiness: %s", body)
	}
}
