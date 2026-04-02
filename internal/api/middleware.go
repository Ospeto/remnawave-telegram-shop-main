package api

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	// cleanupInterval is how often the cleanup goroutine runs.
	cleanupInterval = 10 * time.Minute
	// idleTimeout is how long an IP must be idle before its entry is evicted.
	idleTimeout = 15 * time.Minute
)

// ipEntry tracks a rate limiter and when the IP was last seen.
type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter manages per-IP rate limits with automatic memory cleanup.
type RateLimiter struct {
	ips   map[string]*ipEntry
	mu    sync.Mutex
	rate  rate.Limit
	burst int
}

// NewRateLimiter creates a new rate limiter.
// r: requests per second allowed per IP
// b: burst size allowed per IP
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{
		ips:   make(map[string]*ipEntry),
		rate:  r,
		burst: b,
	}
	go rl.cleanupLoop()
	return rl
}

// GetLimiter returns the rate limiter for the given IP, creating one if needed.
func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, exists := rl.ips[ip]
	if !exists {
		e = &ipEntry{limiter: rate.NewLimiter(rl.rate, rl.burst)}
		rl.ips[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// cleanupLoop periodically removes entries for IPs that haven't been seen
// for idleTimeout, preventing the map from growing unboundedly.
func (rl *RateLimiter) cleanupLoop() {
	for {
		time.Sleep(cleanupInterval)
		rl.mu.Lock()
		before := len(rl.ips)
		cutoff := time.Now().Add(-idleTimeout)
		for ip, e := range rl.ips {
			if e.lastSeen.Before(cutoff) {
				delete(rl.ips, ip)
			}
		}
		slog.Debug("RateLimiter cleanup", "before", before, "after", len(rl.ips), "evicted", before-len(rl.ips))
		rl.mu.Unlock()
	}
}

// Middleware wraps an http.Handler with per-IP rate limiting.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getIP(r)
		if !rl.GetLimiter(ip).Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getIP(r *http.Request) string {
	peerIP := r.RemoteAddr
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peerIP = ip
	}

	// Trust forwarded headers only when traffic comes from loopback/private
	// networks (reverse proxy sidecars / same Docker network).
	if parsedPeer := net.ParseIP(peerIP); parsedPeer != nil && isTrustedProxyIP(parsedPeer) {
		if forwarded := firstForwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
			if parsedForwarded := net.ParseIP(xrip); parsedForwarded != nil {
				return parsedForwarded.String()
			}
		}
	}

	return peerIP
}

func firstForwardedIP(xff string) string {
	if xff == "" {
		return ""
	}

	parts := strings.Split(xff, ",")
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		if parsed := net.ParseIP(candidate); parsed != nil {
			return parsed.String()
		}
	}
	return ""
}

func isTrustedProxyIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PRIVATE_PROXY_HEADERS")), "true") && ip.IsPrivate()
}
