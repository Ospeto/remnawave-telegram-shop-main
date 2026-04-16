package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	redirectGrantTTL     = 24 * time.Hour
	redirectGrantPurpose = "redirect_grant"
)

type redirectGrant struct {
	Target          string
	SubscriptionURL string
	ExpiresAt       time.Time
}

type redirectGrantStore interface {
	issue(target, subscriptionURL string) (string, error)
	consume(token string) (redirectGrant, error)
}

type signedRedirectGrantStore struct {
	secret []byte
	ttl    time.Duration
}

type signedRedirectGrantPayload struct {
	Purpose         string `json:"purpose"`
	Target          string `json:"target"`
	SubscriptionURL string `json:"subscription_url"`
	ExpiresAt       int64  `json:"exp"`
}

func newSignedRedirectGrantStore(secret []byte, ttl time.Duration) *signedRedirectGrantStore {
	return &signedRedirectGrantStore{
		secret: append([]byte(nil), secret...),
		ttl:    ttl,
	}
}

func (s *signedRedirectGrantStore) issue(target, subscriptionURL string) (string, error) {
	expiresAt := time.Now().Add(s.ttl)
	payload, err := json.Marshal(signedRedirectGrantPayload{
		Purpose:         redirectGrantPurpose,
		Target:          target,
		SubscriptionURL: subscriptionURL,
		ExpiresAt:       expiresAt.Unix(),
	})
	if err != nil {
		return "", err
	}
	return signStateToken(s.secret, payload)
}

func (s *signedRedirectGrantStore) consume(token string) (redirectGrant, error) {
	var payload signedRedirectGrantPayload
	if err := verifyStateToken(s.secret, token, &payload); err != nil {
		return redirectGrant{}, fmt.Errorf("redirect token not found")
	}
	if payload.Purpose != redirectGrantPurpose {
		return redirectGrant{}, fmt.Errorf("redirect token not found")
	}
	grant := redirectGrant{
		Target:          payload.Target,
		SubscriptionURL: payload.SubscriptionURL,
		ExpiresAt:       time.Unix(payload.ExpiresAt, 0),
	}
	if time.Now().After(grant.ExpiresAt) {
		return redirectGrant{}, fmt.Errorf("redirect token expired")
	}
	if !isSupportedRedirectTarget(grant.Target) {
		return redirectGrant{}, fmt.Errorf("redirect token not found")
	}
	if !isAllowedRedirectSubscriptionURL(grant.SubscriptionURL) {
		return redirectGrant{}, fmt.Errorf("redirect token not found")
	}
	if extractRedirectSubscriptionURL(grant.Target) != grant.SubscriptionURL {
		return redirectGrant{}, fmt.Errorf("redirect token not found")
	}
	return grant, nil
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

var redirectGrants redirectGrantStore = newSignedRedirectGrantStore([]byte("wavy-dev-state-secret"), redirectGrantTTL)
