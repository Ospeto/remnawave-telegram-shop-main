package api

import (
	"net/http/httptest"
	"testing"
)

func TestGetIPFallsBackToRemoteAddrWhenSplitFails(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "not-a-host-port"

	if got := getIP(req); got != "not-a-host-port" {
		t.Fatalf("getIP() = %q, want %q", got, "not-a-host-port")
	}
}
