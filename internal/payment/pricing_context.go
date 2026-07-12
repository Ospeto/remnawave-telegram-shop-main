package payment

import (
	"context"

	"remnawave-tg-shop-bot/internal/config"
)

type pricingTierCtxKey struct{}

// WithPricingTier attaches retail|wholesale for the next CreatePurchase* call.
// Empty / missing defaults to retail.
func WithPricingTier(ctx context.Context, tier string) context.Context {
	if tier == "" {
		tier = config.PricingTierRetail
	}
	return context.WithValue(ctx, pricingTierCtxKey{}, tier)
}

func pricingTierFromContext(ctx context.Context) string {
	if ctx == nil {
		return config.PricingTierRetail
	}
	if v, ok := ctx.Value(pricingTierCtxKey{}).(string); ok && v != "" {
		return v
	}
	return config.PricingTierRetail
}

// PricingTierFromContext exposes the pricing tier attached via WithPricingTier
// for tests and API soft-inspection. Defaults to retail when unset.
func PricingTierFromContext(ctx context.Context) string {
	return pricingTierFromContext(ctx)
}
