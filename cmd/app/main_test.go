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

func TestVersionedMiniAppURLAddsBuildVersion(t *testing.T) {
	originalCommit, originalBuildDate, originalVersion := Commit, BuildDate, Version
	t.Cleanup(func() {
		Commit, BuildDate, Version = originalCommit, originalBuildDate, originalVersion
	})

	Commit = "1cfd86c"
	BuildDate = "2026-04-17T20:45:40Z"
	Version = "dev"

	got := versionedMiniAppURL("https://mini-92-112-127-10.sslip.io/")
	if got != "https://mini-92-112-127-10.sslip.io/?v=1cfd86c" {
		t.Fatalf("versionedMiniAppURL() = %q, want %q", got, "https://mini-92-112-127-10.sslip.io/?v=1cfd86c")
	}
}

func TestVersionedMiniAppURLPreservesExistingQuery(t *testing.T) {
	originalCommit, originalBuildDate, originalVersion := Commit, BuildDate, Version
	t.Cleanup(func() {
		Commit, BuildDate, Version = originalCommit, originalBuildDate, originalVersion
	})

	Commit = ""
	BuildDate = "2026-04-17T20:45:40Z"
	Version = "dev"

	got := versionedMiniAppURL("https://mini-92-112-127-10.sslip.io/plans?foo=bar")
	if got != "https://mini-92-112-127-10.sslip.io/plans?foo=bar&v=2026-04-17T20%3A45%3A40Z" {
		t.Fatalf("versionedMiniAppURL() = %q, want existing query preserved", got)
	}
}
