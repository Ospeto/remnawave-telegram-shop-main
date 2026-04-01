package api

import (
	"net/http/httptest"
	"testing"
)

func TestGetIPUsesForwardedForFromTrustedProxy(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "172.18.0.2:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.8, 172.18.0.2")

	if got := getIP(req); got != "198.51.100.8" {
		t.Fatalf("getIP() = %q, want %q", got, "198.51.100.8")
	}
}

func TestGetIPFallsBackToRemoteAddrWhenSplitFails(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "not-a-host-port"

	if got := getIP(req); got != "not-a-host-port" {
		t.Fatalf("getIP() = %q, want %q", got, "not-a-host-port")
	}
}
