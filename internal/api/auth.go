package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	telegramSessionExchangeMaxAge = 5 * time.Minute
	telegramSessionTTL            = 2 * time.Hour
	// sessionAbsoluteMaxLifetime bounds the total life of one Mini App session,
	// measured from the original initData exchange. Once it is reached the
	// bearer token is refused and the client must exchange fresh initData, so a
	// leaked token has a bounded exposure window instead of an unbounded one.
	sessionAbsoluteMaxLifetime = 24 * time.Hour
	// sessionRefreshDivisor is the remaining-life fraction below which an
	// authenticated request rotates the bearer token instead of reusing it.
	sessionRefreshDivisor = 4
	authSessionPurpose    = "auth_session"
	sessionTokenHeader    = "X-Session-Token"
	sessionExpiresHeader  = "X-Session-Expires-At"

	// sessionSigningFallbackLabel is the KDF label used to derive a session
	// signing key from the Telegram bot token when SESSION_SIGNING_SECRET is not
	// configured. The bot token itself is never used as a key.
	sessionSigningFallbackLabel = "wavy/session/v1"
	// authSessionKeyLabel and redirectGrantKeyLabel derive distinct subkeys from
	// the configured session signing secret, so an auth-session token can never
	// stand in for a redirect grant (or the other way around).
	authSessionKeyLabel   = "wavy/session/auth-session/v1"
	redirectGrantKeyLabel = "wavy/session/redirect-grant/v1"
)

var (
	errAuthSessionExpired          = errors.New("session expired")
	errAuthSessionLifetimeExceeded = errors.New("session lifetime exceeded")
	errInitDataAlreadyConsumed     = errors.New("initData already consumed")
	errInitDataExchangeUnbound     = errors.New("initData exchange without binding key")
)

type authSession struct {
	Token      string
	TelegramID int64
	Username   string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	Refreshed  bool
}

type authSessionStore interface {
	create(ctx context.Context, telegramID int64, username string) (string, time.Time, error)
	authenticate(ctx context.Context, token string) (authSession, error)
}

// sessionRenewalDeadline returns the absolute deadline a session may be renewed
// to: the original issuance plus the absolute maximum lifetime.
func sessionRenewalDeadline(issuedAt time.Time) time.Time {
	return issuedAt.Add(sessionAbsoluteMaxLifetime)
}

type memoryAuthSessionStore struct {
	mu       sync.Mutex
	sessions map[string]authSession
	ttl      time.Duration
}

func newMemoryAuthSessionStore(ttl time.Duration) *memoryAuthSessionStore {
	return &memoryAuthSessionStore{
		sessions: make(map[string]authSession),
		ttl:      ttl,
	}
}

func (s *memoryAuthSessionStore) create(_ context.Context, telegramID int64, username string) (string, time.Time, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(s.ttl)
	s.sessions[token] = authSession{
		Token:      token,
		TelegramID: telegramID,
		Username:   username,
		IssuedAt:   now,
		ExpiresAt:  expiresAt,
	}
	return token, expiresAt, nil
}

func (s *memoryAuthSessionStore) authenticate(_ context.Context, token string) (authSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[token]
	if !ok {
		return authSession{}, errAuthSessionExpired
	}
	now := time.Now()
	if !now.Before(session.ExpiresAt) {
		delete(s.sessions, token)
		return authSession{}, errAuthSessionExpired
	}
	if now.After(sessionRenewalDeadline(session.IssuedAt)) {
		delete(s.sessions, token)
		return authSession{}, errAuthSessionLifetimeExceeded
	}
	if session.ExpiresAt.Sub(now) >= s.ttl/sessionRefreshDivisor {
		return session, nil
	}

	newToken, err := randomToken(32)
	if err != nil {
		return authSession{}, err
	}
	expiresAt := now.Add(s.ttl)
	if deadline := sessionRenewalDeadline(session.IssuedAt); expiresAt.After(deadline) {
		expiresAt = deadline
	}
	delete(s.sessions, token)
	session.Token = newToken
	session.ExpiresAt = expiresAt
	session.Refreshed = true
	s.sessions[newToken] = session
	return session, nil
}

type signedAuthSessionStore struct {
	secret []byte
	ttl    time.Duration
}

type signedAuthSessionPayload struct {
	Purpose    string `json:"purpose"`
	TelegramID int64  `json:"telegram_id"`
	Username   string `json:"username,omitempty"`
	IssuedAt   int64  `json:"iat"`
	Nonce      string `json:"nonce"`
	ExpiresAt  int64  `json:"exp"`
}

func newSignedAuthSessionStore(secret []byte, ttl time.Duration) *signedAuthSessionStore {
	return &signedAuthSessionStore{
		secret: append([]byte(nil), secret...),
		ttl:    ttl,
	}
}

func (s *signedAuthSessionStore) create(_ context.Context, telegramID int64, username string) (string, time.Time, error) {
	// The signed payload stores whole Unix seconds, so issuance and expiry are
	// truncated here: the token a caller receives and the token the store later
	// verifies then carry exactly the same instants.
	issuedAt := time.Now().Truncate(time.Second)
	expiresAt := issuedAt.Add(s.ttl)
	token, err := s.signSessionToken(telegramID, username, issuedAt, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// authenticate verifies the caller's bearer token and returns the session it
// represents. The caller's own token and expiry are preserved; a token is only
// rotated when its remaining life drops below a quarter of the TTL, and never
// past the absolute maximum lifetime measured from the original exchange.
func (s *signedAuthSessionStore) authenticate(_ context.Context, token string) (authSession, error) {
	payload, err := s.verifySessionToken(token)
	if err != nil {
		return authSession{}, err
	}

	now := time.Now()
	issuedAt := time.Unix(payload.IssuedAt, 0)
	if now.After(sessionRenewalDeadline(issuedAt)) {
		return authSession{}, errAuthSessionLifetimeExceeded
	}

	expiresAt := time.Unix(payload.ExpiresAt, 0)
	session := authSession{
		Token:      token,
		TelegramID: payload.TelegramID,
		Username:   payload.Username,
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
	}
	if expiresAt.Sub(now) >= s.ttl/sessionRefreshDivisor {
		return session, nil
	}

	renewedExpiry := now.Add(s.ttl)
	if deadline := sessionRenewalDeadline(issuedAt); renewedExpiry.After(deadline) {
		renewedExpiry = deadline
	}
	refreshedToken, err := s.signSessionToken(payload.TelegramID, payload.Username, issuedAt, renewedExpiry)
	if err != nil {
		return authSession{}, err
	}
	session.Token = refreshedToken
	session.ExpiresAt = renewedExpiry
	session.Refreshed = true
	return session, nil
}

func (s *signedAuthSessionStore) signSessionToken(telegramID int64, username string, issuedAt, expiresAt time.Time) (string, error) {
	nonce, err := randomToken(8)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(signedAuthSessionPayload{
		Purpose:    authSessionPurpose,
		TelegramID: telegramID,
		Username:   username,
		IssuedAt:   issuedAt.Unix(),
		Nonce:      nonce,
		ExpiresAt:  expiresAt.Unix(),
	})
	if err != nil {
		return "", err
	}
	return signStateToken(s.secret, payload)
}

func (s *signedAuthSessionStore) verifySessionToken(token string) (*signedAuthSessionPayload, error) {
	var payload signedAuthSessionPayload
	if err := verifyStateToken(s.secret, token, &payload); err != nil {
		return nil, errAuthSessionExpired
	}
	if payload.Purpose != authSessionPurpose {
		return nil, errAuthSessionExpired
	}
	// A session token always carries its original issuance time; a token
	// without one cannot be bounded by the absolute maximum lifetime.
	if payload.IssuedAt <= 0 {
		return nil, errAuthSessionExpired
	}
	if payload.ExpiresAt <= payload.IssuedAt {
		return nil, errAuthSessionExpired
	}
	if !time.Now().Before(time.Unix(payload.ExpiresAt, 0)) {
		return nil, errAuthSessionExpired
	}
	return &payload, nil
}

type initDataExchangeStore interface {
	consume(ctx context.Context, bindingKey string, expiresAt time.Time) error
}

// memoryInitDataExchangeGuard is the in-memory single-use guard for Telegram
// initData, used when no database pool is configured.
type memoryInitDataExchangeGuard struct {
	mu       sync.Mutex
	consumed map[string]time.Time
}

func newMemoryInitDataExchangeGuard() *memoryInitDataExchangeGuard {
	return &memoryInitDataExchangeGuard{
		consumed: make(map[string]time.Time),
	}
}

// consume claims one initData binding key exactly once. A collision inside the
// freshness window is a replay of captured initData, not an idempotent retry,
// so it reports errInitDataAlreadyConsumed.
func (g *memoryInitDataExchangeGuard) consume(_ context.Context, bindingKey string, expiresAt time.Time) error {
	if bindingKey == "" {
		return errInitDataExchangeUnbound
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for key, expiry := range g.consumed {
		if !now.Before(expiry) {
			delete(g.consumed, key)
		}
	}

	if existingExpiry, exists := g.consumed[bindingKey]; exists && now.Before(existingExpiry) {
		return errInitDataAlreadyConsumed
	}

	g.consumed[bindingKey] = expiresAt
	return nil
}

const initDataExchangeCleanupInterval = 5 * time.Minute

type dbInitDataExchangeGuard struct {
	pool *pgxpool.Pool

	cleanupMu     sync.Mutex
	lastCleanedAt time.Time
}

func newDBInitDataExchangeGuard(pool *pgxpool.Pool) *dbInitDataExchangeGuard {
	return &dbInitDataExchangeGuard{pool: pool}
}

// consume inserts the initData binding key exactly once. It never updates an
// existing row, so a live collision affects zero rows and reports
// errInitDataAlreadyConsumed: that is a replay of captured initData, not an
// idempotent retry. Rows left behind by initData that is no longer fresh are
// removed by the throttled cleanup above.
func (g *dbInitDataExchangeGuard) consume(ctx context.Context, bindingKey string, expiresAt time.Time) error {
	if bindingKey == "" {
		return errInitDataExchangeUnbound
	}

	// Expired-row cleanup is a full-table scan, so it runs at most once every
	// few minutes instead of on every login.
	if g.shouldCleanupExpiredRows() {
		if _, err := g.pool.Exec(ctx, `DELETE FROM telegram_init_data_exchange WHERE expires_at <= NOW()`); err != nil {
			slog.Warn("Failed to clean expired initData exchange rows", "error", err)
		}
	}

	tag, err := g.pool.Exec(ctx, `
		INSERT INTO telegram_init_data_exchange (binding_key, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (binding_key) DO NOTHING
	`, bindingKey, expiresAt)
	return initDataInsertOutcome(tag, err)
}

// initDataInsertOutcome turns the result of the insert-only guard statement into
// success or a replay: exactly one inserted row reports success, and zero rows
// means a live row already claimed that initData binding key.
func initDataInsertOutcome(tag pgconn.CommandTag, err error) error {
	if err != nil {
		if isUniqueConstraintViolation(err) {
			return errInitDataAlreadyConsumed
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return errInitDataAlreadyConsumed
	}
	return nil
}

// shouldCleanupExpiredRows reports whether the throttled expired-row delete is
// due, claiming the slot so concurrent logins do not all run it.
func (g *dbInitDataExchangeGuard) shouldCleanupExpiredRows() bool {
	g.cleanupMu.Lock()
	defer g.cleanupMu.Unlock()

	now := time.Now()
	if !g.lastCleanedAt.IsZero() && now.Sub(g.lastCleanedAt) < initDataExchangeCleanupInterval {
		return false
	}
	g.lastCleanedAt = now
	return true
}

// ResolveSessionSigningKey resolves the base key material for Mini App session
// and redirect-grant tokens. A configured SESSION_SIGNING_SECRET is used as is;
// when it is absent the key is derived from the Telegram bot token so a deploy
// without the new variable still starts, and the caller logs a warning. The bot
// token is never used as a signing key directly, and no token is ever verified
// against more than one key.
func ResolveSessionSigningKey(configuredSecret, botToken string) (key []byte, usedFallback bool, err error) {
	if configured := strings.TrimSpace(configuredSecret); configured != "" {
		return []byte(configured), false, nil
	}
	token := strings.TrimSpace(botToken)
	if token == "" {
		return nil, false, fmt.Errorf("missing SESSION_SIGNING_SECRET and no TELEGRAM_TOKEN to derive one from")
	}
	derived, err := hmacSHA256Key([]byte(token), sessionSigningFallbackLabel)
	if err != nil {
		return nil, false, err
	}
	return derived, true, nil
}

func hmacSHA256Key(key []byte, label string) ([]byte, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("missing token signing secret")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(label))
	return mac.Sum(nil), nil
}

// derivePurposeKey derives one purpose-bound subkey from the base session
// signing key. Each state store gets its own subkey, so an auth-session token
// can never verify as a redirect grant even if its payload claimed to be one.
func derivePurposeKey(base []byte, label string) ([]byte, error) {
	return hmacSHA256Key(base, label)
}

func ConfigureStateStores(pool *pgxpool.Pool, signingKey []byte) error {
	authKey, err := derivePurposeKey(signingKey, authSessionKeyLabel)
	if err != nil {
		return err
	}
	redirectKey, err := derivePurposeKey(signingKey, redirectGrantKeyLabel)
	if err != nil {
		return err
	}
	authSessions = newSignedAuthSessionStore(authKey, telegramSessionTTL)
	redirectGrants = newSignedRedirectGrantStore(redirectKey, redirectGrantTTL)
	if pool != nil {
		initDataExchanges = newDBInitDataExchangeGuard(pool)
		return nil
	}
	initDataExchanges = newMemoryInitDataExchangeGuard()
	return nil
}

func signStateToken(secret, payload []byte) (string, error) {
	if len(secret) == 0 {
		return "", fmt.Errorf("missing token signing secret")
	}
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := hmac.New(sha256.New, secret)
	signature.Write([]byte(payloadEncoded))
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature.Sum(nil))
	return payloadEncoded + "." + signatureEncoded, nil
}

func verifyStateToken(secret []byte, token string, dest any) error {
	if len(secret) == 0 {
		return fmt.Errorf("missing token signing secret")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid token format")
	}

	payloadEncoded, signatureEncoded := parts[0], parts[1]
	signature := hmac.New(sha256.New, secret)
	signature.Write([]byte(payloadEncoded))
	expected := signature.Sum(nil)
	provided, err := base64.RawURLEncoding.DecodeString(signatureEncoded)
	if err != nil {
		return err
	}
	if !hmac.Equal(expected, provided) {
		return fmt.Errorf("invalid token signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}

func isUniqueConstraintViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

var (
	authSessions      authSessionStore      = newSignedAuthSessionStore(nil, telegramSessionTTL)
	initDataExchanges initDataExchangeStore = newMemoryInitDataExchangeGuard()
)
