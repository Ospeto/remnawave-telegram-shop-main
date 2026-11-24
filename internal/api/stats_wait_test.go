package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// TestStatsWaitReturnsImmediatelyWhenCached pins the common case: a warm cache
// must not add latency to the profile.
func TestStatsWaitReturnsImmediatelyWhenCached(t *testing.T) {
	h := &APIHandler{getCachedSyncKeys: func(int64) ([]syncKeyStats, bool) {
		return []syncKeyStats{{ID: 1}}, true
	}}

	start := time.Now()
	if ok := h.waitForFreshStats(context.Background(), 42); !ok {
		t.Fatal("waitForFreshStats() = false with a warm cache, want true")
	}
	if elapsed := time.Since(start); elapsed > statsWaitPollInterval*3 {
		t.Fatalf("waitForFreshStats() took %v with a warm cache, want no wait", elapsed)
	}
}

// TestStatsWaitStopsAtTheBudget covers a panel that never answers: the profile
// must still be returned, flagged, rather than blocking indefinitely.
func TestStatsWaitStopsAtTheBudget(t *testing.T) {
	h := &APIHandler{getCachedSyncKeys: func(int64) ([]syncKeyStats, bool) {
		return nil, false
	}}

	start := time.Now()
	if ok := h.waitForFreshStats(context.Background(), 42); ok {
		t.Fatal("waitForFreshStats() = true for a cache that never fills")
	}
	elapsed := time.Since(start)
	if elapsed < statsWaitBudget/2 {
		t.Fatalf("waitForFreshStats() gave up after %v, want about %v", elapsed, statsWaitBudget)
	}
	if elapsed > statsWaitBudget*3 {
		t.Fatalf("waitForFreshStats() overshot the budget: %v", elapsed)
	}
}

// TestStatsWaitReturnsWhenTheSnapshotArrives proves the wait actually shortens
// the window instead of always spending the whole budget.
func TestStatsWaitReturnsWhenTheSnapshotArrives(t *testing.T) {
	var calls int
	h := &APIHandler{getCachedSyncKeys: func(int64) ([]syncKeyStats, bool) {
		calls++
		if calls < 3 {
			return nil, false
		}
		return []syncKeyStats{{ID: 7}}, true
	}}

	start := time.Now()
	if ok := h.waitForFreshStats(context.Background(), 42); !ok {
		t.Fatal("waitForFreshStats() = false although the cache filled during the wait")
	}
	if elapsed := time.Since(start); elapsed >= statsWaitBudget {
		t.Fatalf("waitForFreshStats() = %v, want less than the full budget", elapsed)
	}
}

// TestGetMeFlagsPendingStatsInsteadOfReportingZeroUsage is the U4 regression: a
// cold cache must mark the response so the client re-reads it, not present an
// unknown traffic figure as a measured zero.
func TestGetMeFlagsPendingStatsInsteadOfReportingZeroUsage(t *testing.T) {
	customer := &database.Customer{ID: 9, TelegramID: 555}
	h := &APIHandler{
		findCustomerByTelegramID: func(context.Context, int64) (*database.Customer, error) {
			return customer, nil
		},
		getCachedSyncKeys: func(int64) ([]syncKeyStats, bool) { return nil, false },
		triggerSyncKeys:   func(context.Context, int64, int64) {},
		findPendingPurchase: func(context.Context, int64) (*database.Purchase, error) {
			return nil, nil
		},
	}

	body := decodeMeResponse(t, performGetMe(t, h, 555))

	if pending, ok := body["stats_pending"]; !ok || pending != true {
		t.Fatalf("stats_pending = %v (present: %v), want true on a cold cache", pending, ok)
	}
}

// TestGetMeOmitsPendingStatsWhenTheSnapshotIsKnown keeps a warm cache quiet.
func TestGetMeOmitsPendingStatsWhenTheSnapshotIsKnown(t *testing.T) {
	customer := &database.Customer{ID: 9, TelegramID: 555}
	h := &APIHandler{
		findCustomerByTelegramID: func(context.Context, int64) (*database.Customer, error) {
			return customer, nil
		},
		getCachedSyncKeys: func(int64) ([]syncKeyStats, bool) { return nil, true },
		findPendingPurchase: func(context.Context, int64) (*database.Purchase, error) {
			return nil, nil
		},
	}

	body := decodeMeResponse(t, performGetMe(t, h, 555))

	if _, ok := body["stats_pending"]; ok {
		t.Fatalf("stats_pending present on a warm cache: %v", body["stats_pending"])
	}
}

// performGetMe drives the handler the way the router does.
func performGetMe(t *testing.T, h *APIHandler, telegramID int64) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, telegramID))
	rec := httptest.NewRecorder()
	h.GetMe(rec, req)
	return rec
}
