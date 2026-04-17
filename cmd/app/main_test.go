package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"remnawave-tg-shop-bot/internal/gemini"
	"strings"
	"testing"
)

type stubAnalyzer struct {
	readiness gemini.AnalyzerReadiness
}

func (s stubAnalyzer) AnalyzePaymentScreenshot(context.Context, []byte, string, []gemini.ConfiguredProvider) (*gemini.PaymentInfo, error) {
	return nil, nil
}

func (s stubAnalyzer) Readiness(context.Context) gemini.AnalyzerReadiness {
	return s.readiness
}

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

func TestFullHealthHandlerFailsWhenAnalyzerIsDegraded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/readyz", nil)
	rec := httptest.NewRecorder()

	fullHealthHandler(nil, nil, stubAnalyzer{
		readiness: gemini.AnalyzerReadiness{
			Status:  "degraded",
			Primary: "openrouter",
			Providers: map[string]string{
				"openrouter": "error: unauthorized",
			},
		},
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("fullHealthHandler() status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"vision_analyzer":"degraded"`) {
		t.Fatalf("fullHealthHandler() body missing degraded analyzer state: %s", body)
	}
	if !strings.Contains(body, `"status":"fail"`) {
		t.Fatalf("fullHealthHandler() body missing fail status: %s", body)
	}
}
