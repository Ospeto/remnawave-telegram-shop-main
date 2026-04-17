package promo

import (
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestBuildCreateSpecValidatesAndComputesExpiry(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)

	spec, err := BuildCreateSpec(CreateParams{
		Code:            "sale50",
		DiscountPercent: 50,
		DurationDays:    10,
		MaxUses:         100,
	}, now)
	if err != nil {
		t.Fatalf("BuildCreateSpec() error = %v", err)
	}

	if spec.Code != "sale50" {
		t.Fatalf("BuildCreateSpec() code = %q, want %q", spec.Code, "sale50")
	}
	if spec.ValidUntil != now.Add(10*24*time.Hour) {
		t.Fatalf("BuildCreateSpec() validUntil = %v, want %v", spec.ValidUntil, now.Add(10*24*time.Hour))
	}
}

func TestParseBotCreateFieldsUsesSameValidationRules(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)

	spec, err := ParseBotCreateFields([]string{"sale50", "50%", "10days", "100code"}, now)
	if err != nil {
		t.Fatalf("ParseBotCreateFields() error = %v", err)
	}

	if spec.DiscountPercent != 50 {
		t.Fatalf("ParseBotCreateFields() discount = %d, want %d", spec.DiscountPercent, 50)
	}
	if spec.DurationDays != 10 {
		t.Fatalf("ParseBotCreateFields() duration = %d, want %d", spec.DurationDays, 10)
	}
	if spec.MaxUses != 100 {
		t.Fatalf("ParseBotCreateFields() maxUses = %d, want %d", spec.MaxUses, 100)
	}
}

func TestBuildCreateSpecRejectsInvalidDiscount(t *testing.T) {
	_, err := BuildCreateSpec(CreateParams{
		Code:            "sale50",
		DiscountPercent: 0,
		DurationDays:    10,
		MaxUses:         100,
	}, time.Now())
	if err == nil {
		t.Fatal("BuildCreateSpec() error = nil, want invalid discount rejection")
	}
}

func TestStatusForCodeMatchesExistingAdminBotSemantics(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)

	if got := StatusForCode(database.PromoCode{
		Code:            "ACTIVE",
		DiscountPercent: 25,
		MaxUses:         10,
		UsedCount:       2,
		ValidUntil:      now.Add(24 * time.Hour),
	}, now); got != StatusActive {
		t.Fatalf("StatusForCode(active) = %q, want %q", got, StatusActive)
	}

	if got := StatusForCode(database.PromoCode{
		Code:            "EXPIRED",
		DiscountPercent: 25,
		MaxUses:         10,
		UsedCount:       2,
		ValidUntil:      now.Add(-time.Hour),
	}, now); got != StatusExpired {
		t.Fatalf("StatusForCode(expired) = %q, want %q", got, StatusExpired)
	}

	if got := StatusForCode(database.PromoCode{
		Code:            "FULL",
		DiscountPercent: 25,
		MaxUses:         10,
		UsedCount:       10,
		ValidUntil:      now.Add(24 * time.Hour),
	}, now); got != StatusExhausted {
		t.Fatalf("StatusForCode(exhausted) = %q, want %q", got, StatusExhausted)
	}
}
