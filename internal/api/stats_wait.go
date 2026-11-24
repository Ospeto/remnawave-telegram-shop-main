package api

import (
	"context"
	"time"
)

const (
	// statsWaitBudget is how long a cold /api/me waits for the first traffic
	// snapshot to arrive.
	//
	// U4: a cold cache used to be resolved only asynchronously, so the customer
	// saw "0 GB used" until they left the screen and came back. Waiting a short,
	// bounded time covers the common case (the panel answers in well under a
	// second) without letting a slow panel stall the profile.
	statsWaitBudget = 1500 * time.Millisecond

	// statsWaitPollInterval is how often the bounded wait re-reads the cache.
	statsWaitPollInterval = 100 * time.Millisecond
)

// waitForFreshStats blocks briefly until the customer's traffic snapshot is in
// the cache, and reports whether it arrived.
//
// A false result is not an error: the caller marks the response as
// stats_pending and the client re-reads the profile once, so a slow panel
// degrades into a slightly later figure instead of a permanently wrong zero.
func (h *APIHandler) waitForFreshStats(ctx context.Context, customerID int64) bool {
	if _, ok := h.cachedSyncStats(customerID); ok {
		return true
	}

	deadline := time.NewTimer(statsWaitBudget)
	defer deadline.Stop()
	ticker := time.NewTicker(statsWaitPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
			if _, ok := h.cachedSyncStats(customerID); ok {
				return true
			}
		}
	}
}
