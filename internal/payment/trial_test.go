package payment

import (
	"context"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func TestCustomerHasTrialHistory(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	link := "https://sub.example.com/a"

	tests := []struct {
		name     string
		customer *database.Customer
		keyCount int
		want     bool
	}{
		{name: "fresh customer", customer: &database.Customer{}, keyCount: 0, want: false},
		{name: "trial used", customer: &database.Customer{TrialUsedAt: &now}, keyCount: 0, want: true},
		{name: "legacy link", customer: &database.Customer{SubscriptionLink: &link}, keyCount: 0, want: true},
		{name: "existing keys", customer: &database.Customer{}, keyCount: 1, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := customerHasTrialHistory(tt.customer, tt.keyCount); got != tt.want {
				t.Fatalf("customerHasTrialHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrialEligibleForCustomerUsesDurableMarker(t *testing.T) {
	restore := config.SetTrialConfigForTesting(7, 10)
	defer restore()

	service := &PaymentService{}
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)

	eligible, err := service.trialEligibleForCustomer(context.Background(), &database.Customer{ID: 1})
	if err != nil {
		t.Fatalf("trialEligibleForCustomer() error = %v", err)
	}
	if !eligible {
		t.Fatal("trialEligibleForCustomer() = false, want true for fresh customer")
	}

	eligible, err = service.trialEligibleForCustomer(context.Background(), &database.Customer{ID: 1, TrialUsedAt: &now})
	if err != nil {
		t.Fatalf("trialEligibleForCustomer() error = %v", err)
	}
	if eligible {
		t.Fatal("trialEligibleForCustomer() = true, want false for used trial")
	}
}

func TestTrialUsernamePrefersTypedContextKey(t *testing.T) {
	ctx := context.WithValue(context.Background(), UsernameCtxKey, "typed-user")
	if got := trialUsername(ctx, 42); got != "typed-user" {
		t.Fatalf("trialUsername() = %q, want %q", got, "typed-user")
	}

	ctx = context.WithValue(context.Background(), "username", "string-user")
	if got := trialUsername(ctx, 42); got != "string-user" {
		t.Fatalf("trialUsername() = %q, want %q", got, "string-user")
	}

	if got := trialUsername(context.Background(), 42); got != "42" {
		t.Fatalf("trialUsername() = %q, want %q", got, "42")
	}
}
