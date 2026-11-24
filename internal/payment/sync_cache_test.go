package payment

import (
	"testing"
	"time"
)

// TestSyncCacheTTLForKeepsShortLifetimeNearExpiry is P6: a key that is about to
// expire must still be re-read often, because that is when the customer sees the
// state change.
func TestSyncCacheTTLForKeepsShortLifetimeNearExpiry(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	nearExpiry := []KeyStats{
		{ID: 1, ExpireAt: now.Add(90 * time.Minute)},
		{ID: 2, ExpireAt: now.Add(30 * 24 * time.Hour)},
	}

	if got := syncCacheTTLFor(nearExpiry, now); got != syncCacheTTL {
		t.Fatalf("syncCacheTTLFor(near expiry) = %s, want the short TTL %s", got, syncCacheTTL)
	}

	alreadyExpired := []KeyStats{{ID: 3, ExpireAt: now.Add(-48 * time.Hour)}}
	if got := syncCacheTTLFor(alreadyExpired, now); got != syncCacheTTL {
		t.Fatalf("syncCacheTTLFor(expired key) = %s, want the short TTL %s", got, syncCacheTTL)
	}

	unknownExpiry := []KeyStats{{ID: 4}}
	if got := syncCacheTTLFor(unknownExpiry, now); got != syncCacheTTL {
		t.Fatalf("syncCacheTTLFor(no expiry) = %s, want the short TTL %s", got, syncCacheTTL)
	}
}

// TestSyncCacheTTLForExtendsLifetimeFarFromExpiry is the other half of P6: with
// every key weeks away from expiry, the snapshot may live longer.
func TestSyncCacheTTLForExtendsLifetimeFarFromExpiry(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	farFromExpiry := []KeyStats{
		{ID: 1, ExpireAt: now.Add(20 * 24 * time.Hour)},
		{ID: 2, ExpireAt: now.Add(60 * 24 * time.Hour)},
	}

	got := syncCacheTTLFor(farFromExpiry, now)
	if got != syncCacheFarFromExpiryTTL {
		t.Fatalf("syncCacheTTLFor(far from expiry) = %s, want %s", got, syncCacheFarFromExpiryTTL)
	}
	if got <= syncCacheTTL {
		t.Fatalf("far-from-expiry TTL = %s, want longer than the near-expiry TTL %s", got, syncCacheTTL)
	}
}

// TestSyncCacheTTLForEmptySnapshotStaysShort keeps the conservative choice for a
// customer whose snapshot carries no keys at all.
func TestSyncCacheTTLForEmptySnapshotStaysShort(t *testing.T) {
	now := time.Now()
	if got := syncCacheTTLFor(nil, now); got != syncCacheTTL {
		t.Fatalf("syncCacheTTLFor(nil) = %s, want the short TTL %s", got, syncCacheTTL)
	}
}

// TestBeginSyncFetchSharesOneReadAcrossBurst is P6's dedup half: while one read
// is in flight, further callers join it instead of starting their own.
func TestBeginSyncFetchSharesOneReadAcrossBurst(t *testing.T) {
	svc := &PaymentService{}
	const customerID int64 = 42

	leaderState, isLeader := svc.beginSyncFetch(customerID)
	if !isLeader {
		t.Fatal("first beginSyncFetch() was not elected leader")
	}

	followerState, followerIsLeader := svc.beginSyncFetch(customerID)
	if followerIsLeader {
		t.Fatal("second beginSyncFetch() started its own read instead of joining the first")
	}
	if followerState != leaderState {
		t.Fatal("follower joined a different in-flight read")
	}

	svc.finishSyncFetch(customerID, leaderState, []KeyStats{{ID: 7}}, nil)

	select {
	case <-followerState.done:
	default:
		t.Fatal("finishSyncFetch() did not release waiting callers")
	}
	if len(followerState.keys) != 1 || followerState.keys[0].ID != 7 {
		t.Fatalf("shared result = %+v, want the leader's keys", followerState.keys)
	}

	// Once settled, the slot is free so the next cold cache starts a fresh read.
	if _, nextIsLeader := svc.beginSyncFetch(customerID); !nextIsLeader {
		t.Fatal("beginSyncFetch() after settlement was not elected leader; the in-flight slot leaked")
	}
}
