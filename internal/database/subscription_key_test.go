package database

import (
	"testing"
	"time"
)

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
