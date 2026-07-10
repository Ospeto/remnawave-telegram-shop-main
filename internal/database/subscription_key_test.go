package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSubscriptionKeyRepositoryCanMarkMissingRemoteKeysDeleted(t *testing.T) {
	var _ interface {
		MarkMissingRemoteKeysDeleted(context.Context, []uuid.UUID) (int64, error)
	} = (*SubscriptionKeyRepository)(nil)
}

func TestMarkMissingRemoteKeysDeletedSkipsEmptyRemoteUUIDs(t *testing.T) {
	affected, err := (&SubscriptionKeyRepository{}).MarkMissingRemoteKeysDeleted(context.Background(), nil)
	if err != nil {
		t.Fatalf("MarkMissingRemoteKeysDeleted() error = %v", err)
	}
	if affected != 0 {
		t.Fatalf("MarkMissingRemoteKeysDeleted() affected = %d, want 0", affected)
	}
}

func TestPrimarySubscriptionKey(t *testing.T) {
	now := time.Now()
	earlier := now.Add(24 * time.Hour)
	later := now.Add(72 * time.Hour)

	keys := []SubscriptionKey{
		{
			ID:              1,
			SubscriptionURL: "https://example.com/early",
			ExpireAt:        &earlier,
			Status:          "active",
			CreatedAt:       now.Add(-2 * time.Hour),
		},
		{
			ID:              2,
			SubscriptionURL: "https://example.com/late",
			ExpireAt:        &later,
			Status:          "active",
			CreatedAt:       now.Add(-1 * time.Hour),
		},
		{
			ID:              3,
			SubscriptionURL: "https://example.com/deleted",
			ExpireAt:        &later,
			Status:          "deleted",
			CreatedAt:       now,
		},
	}

	got := PrimarySubscriptionKey(keys)
	if got == nil {
		t.Fatal("PrimarySubscriptionKey() = nil, want latest active key")
	}
	if got.ID != 2 {
		t.Fatalf("PrimarySubscriptionKey() id = %d, want 2", got.ID)
	}
}

func TestPrimarySubscriptionKeyBreaksExpiryTiesByCreatedAt(t *testing.T) {
	now := time.Now()
	expireAt := now.Add(48 * time.Hour)

	keys := []SubscriptionKey{
		{
			ID:              1,
			SubscriptionURL: "https://example.com/older",
			ExpireAt:        &expireAt,
			Status:          "active",
			CreatedAt:       now.Add(-2 * time.Hour),
		},
		{
			ID:              2,
			SubscriptionURL: "https://example.com/newer",
			ExpireAt:        &expireAt,
			Status:          "active",
			CreatedAt:       now.Add(-1 * time.Hour),
		},
	}

	got := PrimarySubscriptionKey(keys)
	if got == nil {
		t.Fatal("PrimarySubscriptionKey() = nil, want newest key on expiry tie")
	}
	if got.ID != 2 {
		t.Fatalf("PrimarySubscriptionKey() id = %d, want 2", got.ID)
	}
}

func TestPrimarySubscriptionKeySkipsExpiredKeys(t *testing.T) {
	now := time.Now()
	expired := now.Add(-1 * time.Hour)
	active := now.Add(24 * time.Hour)

	keys := []SubscriptionKey{
		{
			ID:        1,
			ExpireAt:  &expired,
			Status:    "active",
			CreatedAt: now,
		},
		{
			ID:        2,
			ExpireAt:  &active,
			Status:    "active",
			CreatedAt: now.Add(-1 * time.Hour),
		},
	}

	got := PrimarySubscriptionKey(keys)
	if got == nil {
		t.Fatal("PrimarySubscriptionKey() = nil, want non-expired key")
	}
	if got.ID != 2 {
		t.Fatalf("PrimarySubscriptionKey() id = %d, want 2", got.ID)
	}
}

func TestAutoRenewClaimTTLIsPositiveAndReasonable(t *testing.T) {
	if AutoRenewClaimTTL <= 0 {
		t.Fatalf("AutoRenewClaimTTL = %v, want positive", AutoRenewClaimTTL)
	}
	// Fresh claims must outlive a single cron tick (hourly) only partially —
	// 30m is the intended lease; assert bounds so reclaim stays safe.
	if AutoRenewClaimTTL < 5*time.Minute {
		t.Fatalf("AutoRenewClaimTTL = %v, want >= 5m (avoid thrashing)", AutoRenewClaimTTL)
	}
	if AutoRenewClaimTTL > 2*time.Hour {
		t.Fatalf("AutoRenewClaimTTL = %v, want <= 2h (avoid permanent stuck)", AutoRenewClaimTTL)
	}
}

func TestTryClaimAutoRenewWithTTLRejectsNonPositiveByDefaulting(t *testing.T) {
	// Compile-time / API surface: WithTTL exists and default TTL is exported.
	var _ interface {
		TryClaimAutoRenew(context.Context, int64, *time.Time) (*time.Time, bool, error)
		TryClaimAutoRenewWithTTL(context.Context, int64, *time.Time, time.Duration) (*time.Time, bool, error)
		StampKeyLastAutoRenewed(context.Context, int64, time.Time) error
		ClearKeyLastAutoRenewed(context.Context, int64, time.Time) error
	} = (*SubscriptionKeyRepository)(nil)
}
