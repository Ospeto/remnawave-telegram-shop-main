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
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	telegramSessionExchangeMaxAge = 5 * time.Minute
	telegramSessionTTL            = 2 * time.Hour
	authSessionPurpose            = "auth_session"
	sessionTokenHeader            = "X-Session-Token"
	sessionExpiresHeader          = "X-Session-Expires-At"
)

var (
	errAuthSessionExpired      = errors.New("session expired")
	errAuthSessionFingerprint  = errors.New("session fingerprint mismatch")
	errInitDataAlreadyConsumed = errors.New("initData already consumed")
)

type authSession struct {
	Token       string
	TelegramID  int64
	Username    string
	Fingerprint string
	ExpiresAt   time.Time
}

type authSessionStore interface {
	create(ctx context.Context, telegramID int64, username, fingerprint string) (string, time.Time, error)
	authenticate(ctx context.Context, token, fingerprint string) (authSession, error)
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

func (s *memoryAuthSessionStore) create(_ context.Context, telegramID int64, username, fingerprint string) (string, time.Time, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	expiresAt := time.Now().Add(s.ttl)
	s.sessions[token] = authSession{
		Token:       token,
		TelegramID:  telegramID,
		Username:    username,
		Fingerprint: fingerprint,
		ExpiresAt:   expiresAt,
	}
	return token, expiresAt, nil
}

func (s *memoryAuthSessionStore) authenticate(_ context.Context, token, fingerprint string) (authSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[token]
	if !ok {
		return authSession{}, errAuthSessionExpired
	}
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, token)
		return authSession{}, errAuthSessionExpired
	}
	if session.Fingerprint != fingerprint {
		return authSession{}, errAuthSessionFingerprint
	}

	session.ExpiresAt = time.Now().Add(s.ttl)
	s.sessions[token] = session
	return session, nil
}

type signedAuthSessionStore struct {
	secret []byte
	ttl    time.Duration
}

type signedAuthSessionPayload struct {
	Purpose     string `json:"purpose"`
	TelegramID  int64  `json:"telegram_id"`
	Username    string `json:"username,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Nonce       string `json:"nonce"`
	ExpiresAt   int64  `json:"exp"`
}

func newSignedAuthSessionStore(secret []byte, ttl time.Duration) *signedAuthSessionStore {
	return &signedAuthSessionStore{
		secret: append([]byte(nil), secret...),
		ttl:    ttl,
	}
}

func (s *signedAuthSessionStore) create(_ context.Context, telegramID int64, username, fingerprint string) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.ttl)
	token, err := s.signSessionToken(telegramID, username, fingerprint, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *signedAuthSessionStore) authenticate(_ context.Context, token, fingerprint string) (authSession, error) {
	payload, err := s.verifySessionToken(token)
	if err != nil {
		return authSession{}, err
	}
	if payload.Fingerprint != fingerprint {
		return authSession{}, errAuthSessionFingerprint
	}

	expiresAt := time.Now().Add(s.ttl)
	refreshedToken, err := s.signSessionToken(payload.TelegramID, payload.Username, payload.Fingerprint, expiresAt)
	if err != nil {
		return authSession{}, err
	}

	return authSession{
		Token:       refreshedToken,
		TelegramID:  payload.TelegramID,
		Username:    payload.Username,
		Fingerprint: payload.Fingerprint,
		ExpiresAt:   expiresAt,
	}, nil
}

func (s *signedAuthSessionStore) signSessionToken(telegramID int64, username, fingerprint string, expiresAt time.Time) (string, error) {
	nonce, err := randomToken(8)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(signedAuthSessionPayload{
		Purpose:     authSessionPurpose,
		TelegramID:  telegramID,
		Username:    username,
		Fingerprint: fingerprint,
		Nonce:       nonce,
		ExpiresAt:   expiresAt.Unix(),
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
	if time.Now().After(time.Unix(payload.ExpiresAt, 0)) {
		return nil, errAuthSessionExpired
	}
	return &payload, nil
}

type initDataExchangeStore interface {
	consume(ctx context.Context, bindingKey string, expiresAt time.Time) error
}

type memoryInitDataExchangeGuard struct {
	mu       sync.Mutex
	consumed map[string]time.Time
}

func newMemoryInitDataExchangeGuard() *memoryInitDataExchangeGuard {
	return &memoryInitDataExchangeGuard{
		consumed: make(map[string]time.Time),
	}
}

func (g *memoryInitDataExchangeGuard) consume(_ context.Context, bindingKey string, expiresAt time.Time) error {
	if bindingKey == "" {
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for key, expiry := range g.consumed {
		if now.After(expiry) {
			delete(g.consumed, key)
		}
	}

	if existingExpiry, exists := g.consumed[bindingKey]; exists && now.Before(existingExpiry) {
		return errInitDataAlreadyConsumed
	}

	g.consumed[bindingKey] = expiresAt
	return nil
}

type dbInitDataExchangeGuard struct {
	pool *pgxpool.Pool
}

func newDBInitDataExchangeGuard(pool *pgxpool.Pool) *dbInitDataExchangeGuard {
	return &dbInitDataExchangeGuard{pool: pool}
}

func (g *dbInitDataExchangeGuard) consume(ctx context.Context, bindingKey string, expiresAt time.Time) error {
	if bindingKey == "" {
		return nil
	}

	tx, err := g.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `DELETE FROM telegram_init_data_exchange WHERE binding_key = $1 AND expires_at <= NOW()`, bindingKey); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM telegram_init_data_exchange WHERE expires_at <= NOW()`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO telegram_init_data_exchange (binding_key, expires_at) VALUES ($1, $2)`, bindingKey, expiresAt); err != nil {
		if isUniqueConstraintViolation(err) {
			return errInitDataAlreadyConsumed
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func ConfigureStateStores(pool *pgxpool.Pool, signingSecret string) error {
	secret := []byte(strings.TrimSpace(signingSecret))
	if len(secret) == 0 {
		return fmt.Errorf("missing token signing secret")
	}
	authSessions = newSignedAuthSessionStore(secret, telegramSessionTTL)
	redirectGrants = newSignedRedirectGrantStore(secret, redirectGrantTTL)
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
