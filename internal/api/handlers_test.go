package api

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

func TestUploadScreenshotFailureResponsePreservesVerificationReason(t *testing.T) {
	result := &payment.VerificationResult{
		Success:   false,
		Reason:    "Screenshot verification is temporarily unavailable right now. Please try again later or contact support.",
		ReasonKey: "mobile_pay_failed_provider_auth",
	}

	resp := uploadScreenshotFailureResponse(result)

	if resp.Status != "failed" {
		t.Fatalf("uploadScreenshotFailureResponse() status = %q, want failed", resp.Status)
	}
	if resp.Message != result.Reason {
		t.Fatalf("uploadScreenshotFailureResponse() message = %q, want %q", resp.Message, result.Reason)
	}
	if resp.Reason != result.ReasonKey {
		t.Fatalf("uploadScreenshotFailureResponse() reason = %q, want %q", resp.Reason, result.ReasonKey)
	}
}

func TestCompactSubscriptionKeysForDisplayCollapsesDuplicateIdentityBySubscriptionURL(t *testing.T) {
	expire := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	subURL := "https://example.com/sub/abc"
	keys := []database.SubscriptionKey{
		{
			ID:              1,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:              2,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 5, 0, 0, time.UTC),
		},
	}

	compacted := compactSubscriptionKeysForDisplay(keys)
	if len(compacted) != 1 {
		t.Fatalf("compactSubscriptionKeysForDisplay() len = %d, want 1", len(compacted))
	}
	if compacted[0].ID != 2 {
		t.Fatalf("compactSubscriptionKeysForDisplay() kept ID %d, want newest ID 2", compacted[0].ID)
	}
}

func TestCompactSubscriptionKeysForDisplayKeepsDistinctKeysEvenIfPlanShapeMatches(t *testing.T) {
	expire := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	keys := []database.SubscriptionKey{
		{
			ID:              10,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: "https://example.com/sub/key-10",
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:              11,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: "https://example.com/sub/key-11",
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 1, 0, 0, time.UTC),
		},
		{
			ID:              12,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: "https://example.com/sub/key-12",
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 2, 0, 0, time.UTC),
		},
	}

	compacted := compactSubscriptionKeysForDisplay(keys)
	if len(compacted) != 3 {
		t.Fatalf("compactSubscriptionKeysForDisplay() len = %d, want 3", len(compacted))
	}
}

func TestCompactSubscriptionKeysForDisplayPrefersActiveRecordForSameIdentity(t *testing.T) {
	expireActive := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	expireExpired := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	subURL := "https://example.com/sub/xyz"

	keys := []database.SubscriptionKey{
		{
			ID:              20,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "expired",
			ExpireAt:        &expireExpired,
			CreatedAt:       time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:              21,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "active",
			ExpireAt:        &expireActive,
			CreatedAt:       time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
		},
	}

	compacted := compactSubscriptionKeysForDisplay(keys)
	if len(compacted) != 1 {
		t.Fatalf("compactSubscriptionKeysForDisplay() len = %d, want 1", len(compacted))
	}
	if compacted[0].Status != "active" || compacted[0].ID != 21 {
		t.Fatalf("compactSubscriptionKeysForDisplay() kept %q ID %d, want active ID 21", compacted[0].Status, compacted[0].ID)
	}
}
