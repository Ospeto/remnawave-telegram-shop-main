package handler

import (
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/config"
)

func intPtr(v int) *int { return &v }

func TestPlanPriceLabel_ResellerWholesale(t *testing.T) {
	plan := config.Plan{Label: "A", Days: 30, Price: 5000, WholesalePrice: intPtr(4000)}
	label := planPriceLabel(plan, true)
	if !strings.Contains(label, "4000") && !strings.Contains(label, formatPrice(4000)) {
		t.Fatalf("label should show wholesale: %s", label)
	}
}

func TestPlanPriceLabel_NonResellerRetail(t *testing.T) {
	plan := config.Plan{Label: "A", Days: 30, Price: 5000, WholesalePrice: intPtr(4000)}
	label := planPriceLabel(plan, false)
	if !strings.Contains(label, formatPrice(5000)) {
		t.Fatalf("label should show retail: %s", label)
	}
	if strings.Contains(label, formatPrice(4000)) {
		t.Fatalf("non-reseller label should not show wholesale: %s", label)
	}
}

func TestPlanPriceLabel_ResellerWithoutWholesaleFallsBackRetail(t *testing.T) {
	plan := config.Plan{Label: "A", Days: 30, Price: 5000}
	label := planPriceLabel(plan, true)
	if !strings.Contains(label, formatPrice(5000)) {
		t.Fatalf("reseller without wholesale should show retail: %s", label)
	}
}
