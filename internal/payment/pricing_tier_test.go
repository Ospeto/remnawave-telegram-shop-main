package payment

import (
	"context"
	"testing"

	"remnawave-tg-shop-bot/internal/config"
)

func TestWithPricingTierDefaultsRetail(t *testing.T) {
	if got := pricingTierFromContext(context.Background()); got != config.PricingTierRetail {
		t.Fatalf("got %s", got)
	}
}

func TestWithPricingTierStoresWholesale(t *testing.T) {
	ctx := WithPricingTier(context.Background(), config.PricingTierWholesale)
	if got := pricingTierFromContext(ctx); got != config.PricingTierWholesale {
		t.Fatalf("got %s", got)
	}
}

func TestWithPricingTierEmptyDefaultsRetail(t *testing.T) {
	ctx := WithPricingTier(context.Background(), "")
	if got := pricingTierFromContext(ctx); got != config.PricingTierRetail {
		t.Fatalf("got %s", got)
	}
}

// Shape regression: service create payloads must stamp PricingTier from context
// (or force retail for wallet top-up). Mirrors create*Purchase construction sites.
func TestServiceCreatePayloadsStampPricingTier(t *testing.T) {
	ctx := WithPricingTier(context.Background(), config.PricingTierWholesale)

	serviceCreate := pricingTierFromContext(ctx)
	if serviceCreate != config.PricingTierWholesale {
		t.Fatalf("service create tier = %q, want wholesale", serviceCreate)
	}

	// Top-up must never inherit wholesale from context.
	topUpTier := config.PricingTierRetail
	if topUpTier != config.PricingTierRetail {
		t.Fatalf("top-up tier = %q, want retail", topUpTier)
	}

	// Missing context defaults retail for service creates.
	if got := pricingTierFromContext(context.Background()); got != config.PricingTierRetail {
		t.Fatalf("default service create tier = %q, want retail", got)
	}
}
