package config

const (
	PricingTierRetail    = "retail"
	PricingTierWholesale = "wholesale"
)

// ResolvePlanPrice returns the charge amount and pricing tier for a service plan purchase.
// Reseller without wholesale configured falls back to retail.
func ResolvePlanPrice(plan Plan, isReseller bool) (amount int, pricingTier string) {
	if isReseller && plan.WholesalePrice != nil {
		return *plan.WholesalePrice, PricingTierWholesale
	}
	return plan.Price, PricingTierRetail
}
