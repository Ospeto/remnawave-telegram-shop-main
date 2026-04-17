package handler

import (
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestPrimaryNotifiableKeyUsesPrimaryActiveSubscription(t *testing.T) {
	now := time.Now()
	expired := now.Add(-1 * time.Hour)
	active := now.Add(48 * time.Hour)

	got := primaryNotifiableKey([]database.SubscriptionKey{
		{ID: 1, Status: "deleted", ExpireAt: &active, CreatedAt: now},
		{ID: 2, Status: "active", ExpireAt: &expired, CreatedAt: now},
		{ID: 3, Status: "active", ExpireAt: &active, CreatedAt: now.Add(-1 * time.Hour)},
	})

	if got == nil {
		t.Fatal("primaryNotifiableKey() = nil, want active key")
	}
	if got.ID != 3 {
		t.Fatalf("primaryNotifiableKey() id = %d, want 3", got.ID)
	}
}
