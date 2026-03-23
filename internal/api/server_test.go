package api

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRenderRedirectPageEscapesInjectedTarget(t *testing.T) {
	target := `happ://add/" onmouseover="alert(1)`
	page, err := renderRedirectPage(target, "https://example.com/sub")
	if err != nil {
		t.Fatalf("renderRedirectPage() error = %v", err)
	}

	if strings.Contains(page, `onmouseover="alert(1)`) {
		t.Fatalf("renderRedirectPage() leaked raw attribute injection: %s", page)
	}

	if !strings.Contains(page, `happ://add/`) {
		t.Fatalf("renderRedirectPage() did not include target URL")
	}
}

func TestGetIPIgnoresForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "203.0.113.7:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	if got := getIP(req); got != "203.0.113.7" {
		t.Fatalf("getIP() = %q, want %q", got, "203.0.113.7")
	}
}

func TestParsePaymentMethod(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: "mobile_banking"},
		{name: "crypto", input: "crypto", want: "crypto"},
		{name: "wallet", input: "wallet", want: "wallet_payment"},
		{name: "topup", input: "wallet_topup", want: "wallet_topup"},
		{name: "unknown", input: "bogus", wantErr: true},
		{name: "trimmed", input: " wallet ", want: "wallet_payment"},
		{name: "alias", input: "mobile_banking", want: "mobile_banking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePaymentMethod(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parsePaymentMethod(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePaymentMethod(%q) error = %v", tt.input, err)
			}
			if string(got) != tt.want {
				t.Fatalf("parsePaymentMethod(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderRedirectPageUsesEncodedJSStrings(t *testing.T) {
	target := `happ://add/abc?x="y"`
	page, err := renderRedirectPage(target, "https://example.com/sub")
	if err != nil {
		t.Fatalf("renderRedirectPage() error = %v", err)
	}

	if !strings.Contains(page, `var deepLink = `) {
		t.Fatalf("redirect page missing deepLink JS")
	}

	escapedTarget := url.QueryEscape(target)
	if strings.Contains(page, escapedTarget) {
		t.Fatalf("renderRedirectPage() used query-escaped URL in output, expected JS string")
	}
}
