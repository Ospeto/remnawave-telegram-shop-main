package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFullHealthHandlerReturnsShallowPayloadForPublicRequests(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/healthcheck", nil)
	req.RemoteAddr = "203.0.113.10:12345"

	rec := httptest.NewRecorder()
	fullHealthHandler(nil, nil, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("fullHealthHandler() status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if strings.Contains(body, "vision_providers") || strings.Contains(body, "buildDate") || strings.Contains(body, "commit") {
		t.Fatalf("fullHealthHandler() public payload leaked internal probe details: %s", body)
	}
	if !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("fullHealthHandler() public payload missing status: %s", body)
	}
}
