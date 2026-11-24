package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgconn"
	"remnawave-tg-shop-bot/internal/config"
)

// --- A2: the session signing secret must not be the Telegram bot token ---

func TestResolveSessionSigningKeyNeverUsesBotTokenDirectly(t *testing.T) {
	const botToken = "123456:bot-token-that-leaked"

	derived, usedFallback, err := ResolveSessionSigningKey("", botToken)
	if err != nil {
		t.Fatalf("ResolveSessionSigningKey() fallback error = %v", err)
	}
	if !usedFallback {
		t.Fatal("ResolveSessionSigningKey() usedFallback = false, want true when SESSION_SIGNING_SECRET is unset")
	}
	if string(derived) == botToken {
		t.Fatal("ResolveSessionSigningKey() returned the raw bot token as the signing key")
	}
	if len(derived) != 32 {
		t.Fatalf("ResolveSessionSigningKey() derived key length = %d, want 32 (HMAC-SHA256 output)", len(derived))
	}

	configured, usedFallback, err := ResolveSessionSigningKey("  dedicated-secret  ", botToken)
	if err != nil {
		t.Fatalf("ResolveSessionSigningKey() configured error = %v", err)
	}
	if usedFallback {
		t.Fatal("ResolveSessionSigningKey() usedFallback = true, want false when SESSION_SIGNING_SECRET is set")
	}
	if string(configured) != "dedicated-secret" {
		t.Fatalf("ResolveSessionSigningKey() configured key = %q, want trimmed dedicated secret", string(configured))
	}
}

func TestConfigureStateStoresDerivesDistinctPurposeSubkeys(t *testing.T) {
	previousAuthSessions := authSessions
	previousInitDataExchanges := initDataExchanges
	previousRedirectGrants := redirectGrants
	t.Cleanup(func() {
		authSessions = previousAuthSessions
		initDataExchanges = previousInitDataExchanges
		redirectGrants = previousRedirectGrants
	})

	if err := ConfigureStateStores(nil, []byte("base-session-secret")); err != nil {
		t.Fatalf("ConfigureStateStores() error = %v", err)
	}

	redirectStore, ok := redirectGrants.(*signedRedirectGrantStore)
	if !ok {
		t.Fatalf("redirectGrants type = %T, want *signedRedirectGrantStore", redirectGrants)
	}
	sessionStore, ok := authSessions.(*signedAuthSessionStore)
	if !ok {
		t.Fatalf("authSessions type = %T, want *signedAuthSessionStore", authSessions)
	}
	if string(sessionStore.secret) == string(redirectStore.secret) {
		t.Fatal("auth session and redirect grant stores share one signing key, want distinct purpose subkeys")
	}
	if string(sessionStore.secret) == "base-session-secret" || string(redirectStore.secret) == "base-session-secret" {
		t.Fatal("ConfigureStateStores() used the base secret directly, want derived purpose subkeys")
	}

	// An auth-session token must not verify as a redirect grant, and vice versa.
	token, _, err := sessionStore.create(context.Background(), 99, "alice")
	if err != nil {
		t.Fatalf("authSessions.create() error = %v", err)
	}
	if _, err := redirectStore.consume(token); err == nil {
		t.Fatal("redirectGrantStore.consume() accepted an auth session token, want rejection across purpose keys")
	}

	grantToken, err := redirectStore.issue("happ://add/https://sub.wavypremium.xyz/sub", "https://sub.wavypremium.xyz/sub")
	if err != nil {
		t.Fatalf("redirectGrants.issue() error = %v", err)
	}
	if _, err := sessionStore.authenticate(context.Background(), grantToken); err == nil {
		t.Fatal("authSessions.authenticate() accepted a redirect grant token, want rejection across purpose keys")
	}
}

// --- A5: bearer session expiry must be real and bounded ---

func TestAuthSessionStorePreservesTokenUntilRefreshWindow(t *testing.T) {
	store := newSignedAuthSessionStore([]byte("test-secret"), time.Hour)

	token, expiresAt, err := store.create(context.Background(), 42, "alice")
	if err != nil {
		t.Fatalf("create() error = %v", err)
	}

	session, err := store.authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	if session.Token != token {
		t.Fatalf("authenticate() rotated a token with most of its life left: got %q want %q", session.Token, token)
	}
	if !session.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("authenticate() changed expiry of a fresh token: got %v want %v", session.ExpiresAt, expiresAt)
	}
	if session.Refreshed {
		t.Fatal("authenticate() reported a refresh for a token with most of its life left")
	}
	if session.IssuedAt.IsZero() {
		t.Fatal("authenticate() lost the original issuance time")
	}

	// Repeated use inside the refresh window must not extend the session.
	for i := 0; i < 3; i++ {
		if _, err := store.authenticate(context.Background(), token); err != nil {
			t.Fatalf("authenticate() repeated call %d error = %v", i, err)
		}
	}
	last, err := store.authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("authenticate() final call error = %v", err)
	}
	if last.ExpiresAt.After(expiresAt) {
		t.Fatalf("authenticate() extended expiry on a fresh token: original=%v now=%v", expiresAt, last.ExpiresAt)
	}
}

func TestAuthSessionStoreRefreshesOnlyNearExpiry(t *testing.T) {
	store := newSignedAuthSessionStore([]byte("test-secret"), time.Hour)
	issuedAt := time.Now().Add(-45 * time.Minute)
	if _, err := store.signSessionToken(42, "alice", issuedAt, issuedAt.Add(45*time.Minute)); err != nil {
		t.Fatalf("signSessionToken() error = %v", err)
	}
	baseToken, err := store.signSessionToken(42, "alice", issuedAt, issuedAt.Add(49*time.Minute))
	if err != nil {
		t.Fatalf("signSessionToken() error = %v", err)
	}

	session, err := store.authenticate(context.Background(), baseToken)
	if err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	if !session.Refreshed {
		t.Fatal("authenticate() did not refresh a token inside the refresh window")
	}
	if session.Token == baseToken {
		t.Fatal("authenticate() reported a refresh but kept the old token")
	}
	if !session.IssuedAt.Equal(time.Unix(issuedAt.Unix(), 0)) {
		t.Fatalf("authenticate() issued_at = %v, want the original issuance time %v", session.IssuedAt, issuedAt)
	}
	if deadline := sessionRenewalDeadline(session.IssuedAt); session.ExpiresAt.After(deadline) {
		t.Fatalf("authenticate() refreshed expiry %v past the absolute maximum %v", session.ExpiresAt, deadline)
	}

	// A refreshed token must itself be usable, preserving expiry and issuance.
	again, err := store.authenticate(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("authenticate() refreshed token error = %v", err)
	}
	if again.TelegramID != 42 {
		t.Fatalf("authenticate() refreshed session telegram_id = %d, want 42", again.TelegramID)
	}
	if again.Refreshed {
		t.Fatal("authenticate() refreshed a token whose remaining life is above the refresh threshold")
	}
}

func TestAuthSessionStoreRefusesRefreshPastAbsoluteMaximum(t *testing.T) {
	store := newSignedAuthSessionStore([]byte("test-secret"), time.Hour)

	issuedAt := time.Now().Add(-(sessionAbsoluteMaxLifetime + 2*time.Minute))
	token, err := store.signSessionToken(42, "alice", issuedAt, issuedAt.Add(sessionAbsoluteMaxLifetime+30*time.Minute))
	if err != nil {
		t.Fatalf("signSessionToken() error = %v", err)
	}

	if _, err := store.authenticate(context.Background(), token); !errors.Is(err, errAuthSessionLifetimeExceeded) {
		t.Fatalf("authenticate() past absolute maximum error = %v, want %v", err, errAuthSessionLifetimeExceeded)
	}

	// Signing a token without an issuance time (the old payload shape) must not
	// authenticate, so a pre-upgrade token cannot outlive the new bound.
	legacyPayload, err := json.Marshal(map[string]any{
		"purpose":     authSessionPurpose,
		"telegram_id": 42,
		"fingerprint": "telegram-webapp",
		"nonce":       "abc",
		"exp":         time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("json.Marshal(legacy payload) error = %v", err)
	}
	legacyToken, err := signStateToken([]byte("test-secret"), legacyPayload)
	if err != nil {
		t.Fatalf("signStateToken() error = %v", err)
	}
	if _, err := store.authenticate(context.Background(), legacyToken); err == nil {
		t.Fatal("authenticate() accepted a payload without issued_at, want rejection")
	}
}

// --- A6: initData must be single-use, and only a real second use is refused ---

func TestMemoryInitDataGuardRefusesReplayInsideFreshnessWindow(t *testing.T) {
	guard := newMemoryInitDataExchangeGuard()
	expiresAt := time.Now().Add(5 * time.Minute)

	if err := guard.consume(context.Background(), "initdata-hash", expiresAt); err != nil {
		t.Fatalf("consume() first call error = %v, want success for a fresh launch", err)
	}
	if err := guard.consume(context.Background(), "initdata-hash", expiresAt); !errors.Is(err, errInitDataAlreadyConsumed) {
		t.Fatalf("consume() replay error = %v, want %v", err, errInitDataAlreadyConsumed)
	}
	if err := guard.consume(context.Background(), "", expiresAt); !errors.Is(err, errInitDataExchangeUnbound) {
		t.Fatalf("consume() empty binding key error = %v, want %v", err, errInitDataExchangeUnbound)
	}
}

func TestInitDataExchangeGuardAllowsFreshLaunchAfterExpiry(t *testing.T) {
	guard := newMemoryInitDataExchangeGuard()

	// A legitimate new Telegram launch arrives with a different initData hash.
	freshExpiry := time.Now().Add(time.Minute)
	if err := guard.consume(context.Background(), "hash-launch-1", freshExpiry); err != nil {
		t.Fatalf("consume() first launch error = %v", err)
	}
	if err := guard.consume(context.Background(), "hash-launch-2", freshExpiry); err != nil {
		t.Fatalf("consume() second fresh launch error = %v, want success for distinct initData", err)
	}

	// Bookkeeping whose own expiry has already passed is pruned, so a replay
	// guard never outlives the freshness window it is protecting.
	if err := guard.consume(context.Background(), "hash-launch-3", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("consume() with an already-past expiry error = %v", err)
	}
	if err := guard.consume(context.Background(), "hash-launch-3", freshExpiry); err != nil {
		t.Fatalf("consume() after the stale window expired error = %v, want success once the stale key is pruned", err)
	}
	if err := guard.consume(context.Background(), "hash-launch-3", freshExpiry); !errors.Is(err, errInitDataAlreadyConsumed) {
		t.Fatalf("consume() immediate replay error = %v, want %v", err, errInitDataAlreadyConsumed)
	}
}

// TestInitDataInsertOutcomeRequiresExactlyOneRow pins the A6 database path that
// cannot be exercised without a live Postgres: an insert that affected zero rows
// means a live row already claimed the binding key, which is a replay.
func TestInitDataInsertOutcomeRequiresExactlyOneRow(t *testing.T) {
	tests := []struct {
		name    string
		tag     pgconn.CommandTag
		err     error
		wantErr error
	}{
		{name: "one inserted row", tag: pgconn.CommandTag([]byte("INSERT 0 1"))},
		{name: "zero rows (live collision)", tag: pgconn.CommandTag([]byte("INSERT 0 0")), wantErr: errInitDataAlreadyConsumed},
		{
			name:    "unique violation",
			tag:     pgconn.CommandTag(nil),
			err:     &pgconn.PgError{Code: "23505"},
			wantErr: errInitDataAlreadyConsumed,
		},
		{
			name: "other database error",
			tag:  pgconn.CommandTag(nil),
			err:  errors.New("connection reset"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := initDataInsertOutcome(tt.tag, tt.err)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("initDataInsertOutcome() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.err != nil {
				if err == nil {
					t.Fatal("initDataInsertOutcome() swallowed a database error")
				}
				return
			}
			if err != nil {
				t.Fatalf("initDataInsertOutcome() error = %v, want nil for a single inserted row", err)
			}
		})
	}
}

// TestCreateSessionConsumesInitDataExactlyOnce drives the real exchange handler
// end to end: one signed initData must mint exactly one bearer session, and the
// Mini App's 401 retry path (which re-exchanges the same initData) must be
// refused rather than silently granted a second session.
func TestCreateSessionConsumesInitDataExactlyOnce(t *testing.T) {
	previousAuthSessions := authSessions
	previousInitDataExchanges := initDataExchanges
	handler := &APIHandler{}
	initData := testTelegramInitData(t, config.TelegramToken(), time.Now(), 4242, "alice")
	t.Cleanup(func() {
		authSessions = previousAuthSessions
		initDataExchanges = previousInitDataExchanges
	})

	authSessions = newSignedAuthSessionStore([]byte("test-session-key"), telegramSessionTTL)
	initDataExchanges = newMemoryInitDataExchangeGuard()

	exchange := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
		req.Header.Set("Authorization", "twa "+initData)
		rec := httptest.NewRecorder()
		handler.CreateSession(rec, req)
		return rec
	}

	first := exchange()
	if first.Code != http.StatusOK {
		t.Fatalf("CreateSession() first exchange status = %d, want %d (body=%q)", first.Code, http.StatusOK, first.Body.String())
	}
	var response SessionExchangeResponse
	if err := json.NewDecoder(strings.NewReader(first.Body.String())).Decode(&response); err != nil {
		t.Fatalf("decode first exchange response error = %v", err)
	}
	if response.Token == "" {
		t.Fatal("CreateSession() first exchange returned an empty bearer token")
	}

	second := exchange()
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("CreateSession() replayed initData status = %d, want %d (body=%q)", second.Code, http.StatusUnauthorized, second.Body.String())
	}
	if strings.Contains(second.Body.String(), response.Token) {
		t.Fatal("CreateSession() replay leaked the first session token")
	}
}

// TestBearerTokenIsNotResignedOnEveryRequest is the A5 regression: authenticating
// a token that still has most of its life left must not mint a new one, so a
// request loop can no longer keep a leaked token alive forever.
func TestBearerTokenIsNotResignedOnEveryRequest(t *testing.T) {
	previousAuthSessions := authSessions
	t.Cleanup(func() { authSessions = previousAuthSessions })

	authSessions = newSignedAuthSessionStore([]byte("test-session-key"), telegramSessionTTL)

	token, _, err := authSessions.create(context.Background(), 7, "alice")
	if err != nil {
		t.Fatalf("authSessions.create() error = %v", err)
	}

	protected := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var firstToken, lastToken string
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("authMiddleware() call %d status = %d, want %d", i, rec.Code, http.StatusOK)
		}
		advertised := rec.Header().Get(sessionTokenHeader)
		if advertised == "" {
			t.Fatalf("authMiddleware() call %d did not advertise a session token", i)
		}
		if i == 0 {
			firstToken = advertised
		}
		lastToken = advertised
	}

	if firstToken != token {
		t.Fatalf("authMiddleware() re-signed the token on the first request: got %q want %q", firstToken, token)
	}
	if lastToken != token {
		t.Fatalf("authMiddleware() re-signed the token across repeated requests: got %q want %q", lastToken, token)
	}
}

// TestAuthSessionPayloadCarriesNoConstantFingerprint pins the A5 cleanup: the
// fingerprint that used to bind sessions to nothing must be gone from the
// signed payload, so nothing can be mistaken for a real device binding.
func TestAuthSessionPayloadCarriesNoConstantFingerprint(t *testing.T) {
	store := newSignedAuthSessionStore([]byte("test-secret"), time.Hour)
	token, _, err := store.create(context.Background(), 11, "alice")
	if err != nil {
		t.Fatalf("create() error = %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("token parts = %d, want 2", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode payload error = %v", err)
	}
	if strings.Contains(string(raw), "telegram-webapp") || strings.Contains(string(raw), "fingerprint") {
		t.Fatalf("auth session payload still carries a fingerprint binding: %s", string(raw))
	}

	var decoded signedAuthSessionPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	if decoded.IssuedAt == 0 {
		t.Fatal("auth session payload is missing issued_at")
	}
}
