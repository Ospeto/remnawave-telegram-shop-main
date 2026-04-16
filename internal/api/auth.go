package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const (
	telegramSessionExchangeMaxAge = 5 * time.Minute
	telegramSessionTTL            = 2 * time.Hour
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

type authSessionStore struct {
	mu       sync.Mutex
	sessions map[string]authSession
	ttl      time.Duration
}

func newAuthSessionStore(ttl time.Duration) *authSessionStore {
	return &authSessionStore{
		sessions: make(map[string]authSession),
		ttl:      ttl,
	}
}

func (s *authSessionStore) create(telegramID int64, username, fingerprint string) (string, time.Time, error) {
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

func (s *authSessionStore) authenticate(token, fingerprint string) (authSession, error) {
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

type initDataExchangeGuard struct {
	mu       sync.Mutex
	consumed map[string]time.Time
}

func newInitDataExchangeGuard() *initDataExchangeGuard {
	return &initDataExchangeGuard{
		consumed: make(map[string]time.Time),
	}
}

func (g *initDataExchangeGuard) consume(bindingKey string, expiresAt time.Time) error {
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

func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

var (
	authSessions      = newAuthSessionStore(telegramSessionTTL)
	initDataExchanges = newInitDataExchangeGuard()
)
