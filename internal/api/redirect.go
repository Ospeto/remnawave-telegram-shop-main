package api

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const redirectGrantTTL = 2 * time.Minute

type redirectGrant struct {
	Target          string
	SubscriptionURL string
	ExpiresAt       time.Time
}

type redirectGrantStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	grants map[string]redirectGrant
}

func newRedirectGrantStore(ttl time.Duration) *redirectGrantStore {
	return &redirectGrantStore{
		ttl:    ttl,
		grants: make(map[string]redirectGrant),
	}
}

func (s *redirectGrantStore) issue(target, subscriptionURL string) (string, error) {
	token, err := randomToken(24)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(time.Now())
	s.grants[token] = redirectGrant{
		Target:          target,
		SubscriptionURL: subscriptionURL,
		ExpiresAt:       time.Now().Add(s.ttl),
	}
	return token, nil
}

func (s *redirectGrantStore) consume(token string) (redirectGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.cleanupLocked(now)

	grant, ok := s.grants[token]
	if !ok {
		return redirectGrant{}, fmt.Errorf("redirect token not found")
	}
	delete(s.grants, token)
	if now.After(grant.ExpiresAt) {
		return redirectGrant{}, fmt.Errorf("redirect token expired")
	}
	return grant, nil
}

func (s *redirectGrantStore) cleanupLocked(now time.Time) {
	for token, grant := range s.grants {
		if now.After(grant.ExpiresAt) {
			delete(s.grants, token)
		}
	}
}

func signedRedirectURLForTarget(target string) string {
	if strings.TrimSpace(target) == "" || !isSupportedRedirectTarget(target) {
		return ""
	}

	subscriptionURL := extractRedirectSubscriptionURL(target)
	if !isAllowedRedirectSubscriptionURL(subscriptionURL) {
		return ""
	}

	token, err := redirectGrants.issue(target, subscriptionURL)
	if err != nil {
		return ""
	}
	return "/redirect?token=" + url.QueryEscape(token)
}

func isAllowedRedirectSubscriptionURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && parsed.Host != ""
}

var redirectGrants = newRedirectGrantStore(redirectGrantTTL)
