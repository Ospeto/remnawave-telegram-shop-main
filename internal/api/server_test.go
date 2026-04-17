package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestRequestFingerprintIgnoresIPChanges(t *testing.T) {
	reqA := httptest.NewRequest("GET", "http://example.com/", nil)
	reqA.RemoteAddr = "203.0.113.7:12345"
	reqA.Header.Set("User-Agent", "TelegramBot (iOS)")

	reqB := httptest.NewRequest("GET", "http://example.com/", nil)
	reqB.RemoteAddr = "198.51.100.9:54321"
	reqB.Header.Set("User-Agent", "TelegramBot (iOS)")

	if gotA, gotB := requestFingerprint(reqA), requestFingerprint(reqB); gotA != gotB {
		t.Fatalf("requestFingerprint() should stay stable across IP changes, got %q vs %q", gotA, gotB)
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
		{name: "crypto disabled", input: "crypto", wantErr: true},
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

func TestRedirectHelpers(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantValid  bool
		wantSubURL string
	}{
		{
			name:       "happ deep link",
			target:     "happ://add/https://example.com/sub",
			wantValid:  true,
			wantSubURL: "https://example.com/sub",
		},
		{
			name:       "happproxy deep link",
			target:     "happproxy://add/https://example.com/sub",
			wantValid:  true,
			wantSubURL: "https://example.com/sub",
		},
		{
			name:       "unsupported scheme",
			target:     "https://example.com/sub",
			wantValid:  false,
			wantSubURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupportedRedirectTarget(tt.target); got != tt.wantValid {
				t.Fatalf("isSupportedRedirectTarget(%q) = %v, want %v", tt.target, got, tt.wantValid)
			}
			if got := extractRedirectSubscriptionURL(tt.target); got != tt.wantSubURL {
				t.Fatalf("extractRedirectSubscriptionURL(%q) = %q, want %q", tt.target, got, tt.wantSubURL)
			}
		})
	}
}

func TestInitDataExchangeGuardAllowsReuseWhileInitDataIsStillValid(t *testing.T) {
	guard := newMemoryInitDataExchangeGuard()
	expiresAt := time.Now().Add(time.Minute)

	if err := guard.consume(context.Background(), "binding-key", expiresAt); err != nil {
		t.Fatalf("consume() first call error = %v", err)
	}
	if err := guard.consume(context.Background(), "binding-key", expiresAt); err != nil {
		t.Fatalf("consume() second call error = %v, want idempotent success", err)
	}
}

func TestAuthSessionStoreAuthenticatesAndRefreshes(t *testing.T) {
	store := newSignedAuthSessionStore([]byte("test-secret"), 30*time.Minute)

	token, expiresAt, err := store.create(context.Background(), 42, "alice", "ua:test")
	if err != nil {
		t.Fatalf("create() error = %v", err)
	}
	if token == "" {
		t.Fatal("create() returned empty token")
	}

	session, err := store.authenticate(context.Background(), token, "ua:test")
	if err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	if session.TelegramID != 42 || session.Username != "alice" {
		t.Fatalf("authenticate() returned unexpected session: %+v", session)
	}
	if session.Token == token {
		t.Fatal("authenticate() did not rotate bearer token")
	}

	refreshed, err := store.authenticate(context.Background(), session.Token, "ua:test")
	if err != nil {
		t.Fatalf("authenticate() second call error = %v", err)
	}
	if !refreshed.ExpiresAt.After(expiresAt) {
		t.Fatalf("authenticate() did not refresh expiry: original=%v refreshed=%v", expiresAt, refreshed.ExpiresAt)
	}

	if _, err := store.authenticate(context.Background(), session.Token, "ua:other"); err == nil {
		t.Fatal("authenticate() fingerprint mismatch error = nil, want rejection")
	}
}

func TestValidateInitDataRejectsStaleSessionExchange(t *testing.T) {
	initData := testTelegramInitData(t, "bot-token", time.Now().Add(-10*time.Minute), 42, "alice")

	if _, _, _, _, err := validateInitData(initData, "bot-token", 5*time.Minute); err == nil {
		t.Fatal("validateInitData() stale session-exchange window error = nil, want rejection")
	}
}

func TestRedirectGrantStoreValidatesSignedToken(t *testing.T) {
	store := newSignedRedirectGrantStore([]byte("test-secret"), 2*time.Minute)

	token, err := store.issue("happ://add/https://example.com/sub", "https://example.com/sub")
	if err != nil {
		t.Fatalf("issue() error = %v", err)
	}

	grant, err := store.consume(token)
	if err != nil {
		t.Fatalf("consume() error = %v", err)
	}
	if grant.Target != "happ://add/https://example.com/sub" || grant.SubscriptionURL != "https://example.com/sub" {
		t.Fatalf("consume() returned unexpected grant: %+v", grant)
	}
}

func TestRedirectGrantStoreRejectsExpiredToken(t *testing.T) {
	store := newSignedRedirectGrantStore([]byte("test-secret"), time.Second)

	token, err := store.issue("happ://add/https://example.com/sub", "https://example.com/sub")
	if err != nil {
		t.Fatalf("issue() error = %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	if _, err := store.consume(token); err == nil {
		t.Fatal("consume() expired token error = nil, want rejection")
	}
}

func TestConfigureStateStoresRejectsEmptySigningSecret(t *testing.T) {
	previousAuthSessions := authSessions
	previousInitDataExchanges := initDataExchanges
	previousRedirectGrants := redirectGrants
	t.Cleanup(func() {
		authSessions = previousAuthSessions
		initDataExchanges = previousInitDataExchanges
		redirectGrants = previousRedirectGrants
	})

	if err := ConfigureStateStores(nil, ""); err == nil {
		t.Fatal("ConfigureStateStores() error = nil, want missing secret rejection")
	}
}

func TestRegisterHandlersReturns404ForUnknownAPIPaths(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v", repoRoot, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore cwd error = %v", err)
		}
	})

	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/does-not-exist", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown API route status = %d, want %d (body=%q)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func testTelegramInitData(t *testing.T, botToken string, authDate time.Time, userID int64, username string) string {
	t.Helper()

	userJSON, err := json.Marshal(map[string]any{
		"id":       userID,
		"username": username,
	})
	if err != nil {
		t.Fatalf("json.Marshal(user) error = %v", err)
	}

	values := map[string]string{
		"auth_date": strconv.FormatInt(authDate.Unix(), 10),
		"query_id":  "AAHdF6IQAAAAAN0XohDhrOrc",
		"user":      string(userJSON),
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, values[key]))
	}
	dataCheckString := strings.Join(parts, "\n")

	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))
	secret := secretKey.Sum(nil)

	signer := hmac.New(sha256.New, secret)
	signer.Write([]byte(dataCheckString))
	values["hash"] = hex.EncodeToString(signer.Sum(nil))

	query := url.Values{}
	for key, value := range values {
		query.Set(key, value)
	}
	return query.Encode()
}
