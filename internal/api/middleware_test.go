package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetIPUsesForwardedForFromTrustedProxy(t *testing.T) {
	t.Setenv("TRUST_PRIVATE_PROXY_HEADERS", "true")

	// A11: only the hop our proxy appended (the last element) is trustworthy;
	// the client can put anything it likes to the left of it.
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "172.18.0.2:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.8, 172.18.0.2")

	if got := getIP(req); got != "172.18.0.2" {
		t.Fatalf("getIP() = %q, want the proxy-appended hop %q", got, "172.18.0.2")
	}
}

func TestGetIPIgnoresClientSuppliedForwardedForHops(t *testing.T) {
	t.Setenv("TRUST_PRIVATE_PROXY_HEADERS", "true")

	// A spoofed prefix must not become a fresh rate-limit bucket.
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "172.18.0.2:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.8, 172.18.0.2")

	if got := getIP(req); got != "172.18.0.2" {
		t.Fatalf("getIP() = %q, want the last forwarded hop %q", got, "172.18.0.2")
	}
}

func TestGetIPUsesRealIPHeaderFromTrustedProxy(t *testing.T) {
	t.Setenv("TRUST_PRIVATE_PROXY_HEADERS", "true")

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "172.18.0.2:12345"
	req.Header.Set("X-Real-IP", "198.51.100.8")

	if got := getIP(req); got != "198.51.100.8" {
		t.Fatalf("getIP() = %q, want X-Real-IP %q", got, "198.51.100.8")
	}
}

func TestGetIPIgnoresPrivateProxyHeadersByDefault(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "172.18.0.2:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.8, 172.18.0.2")

	if got := getIP(req); got != "172.18.0.2" {
		t.Fatalf("getIP() = %q, want %q", got, "172.18.0.2")
	}
}

func TestGetIPFallsBackToRemoteAddrWhenSplitFails(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "not-a-host-port"

	if got := getIP(req); got != "not-a-host-port" {
		t.Fatalf("getIP() = %q, want %q", got, "not-a-host-port")
	}
}

func TestLastForwardedIPSkipsInvalidTrailingEntries(t *testing.T) {
	if got := lastForwardedIP("198.51.100.8, 172.18.0.2,"); got != "172.18.0.2" {
		t.Fatalf("lastForwardedIP() = %q, want %q", got, "172.18.0.2")
	}
	if got := lastForwardedIP(""); got != "" {
		t.Fatalf("lastForwardedIP(\"\") = %q, want empty", got)
	}
	if got := lastForwardedIP("not-an-ip"); got != "" {
		t.Fatalf("lastForwardedIP() = %q, want empty for a malformed header", got)
	}
}

// TestRateLimiterExemptsStaticAssetsAndHealthChecks is the A11 regression test:
// throttling /assets/* or the health probes breaks page loads and makes the
// container look unhealthy under load, without protecting any abuse surface.
func TestRateLimiterExemptsStaticAssetsAndHealthChecks(t *testing.T) {
	exemptPaths := []string{"/assets/index-abc123.js", "/assets/index-abc123.css", "/livez", "/readyz", "/healthcheck"}

	limiter := NewRateLimiter(0.0001, 1)
	hits := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	})
	handler := limiter.Middleware(next)

	for _, path := range exemptPaths {
		for i := 0; i < 5; i++ {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", path, nil)
			req.RemoteAddr = "198.51.100.8:12345"
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s request %d = %d, want 200 (exempt from the limiter)", path, i+1, rec.Code)
			}
		}
	}

	if hits != len(exemptPaths)*5 {
		t.Fatalf("downstream hits = %d, want %d", hits, len(exemptPaths)*5)
	}

	// A real API path is still throttled, so the exemption stays surgical.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/plans", nil)
	req.RemoteAddr = "198.51.100.8:12345"
	handler.ServeHTTP(rec, req)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second /api/plans request = %d, want 429", rec.Code)
	}
}

func TestIsRateLimitExempt(t *testing.T) {
	cases := map[string]bool{
		"/assets/index-abc123.js": true,
		"/assets/":                true,
		"/livez":                  true,
		"/readyz":                 true,
		"/healthcheck":            true,
		"/api/plans":              false,
		"/":                       false,
		"/assets":                 false,
		"/assetsfoo":              false,
	}

	for path, want := range cases {
		if got := isRateLimitExempt(path); got != want {
			t.Fatalf("isRateLimitExempt(%q) = %v, want %v", path, got, want)
		}
	}
}
