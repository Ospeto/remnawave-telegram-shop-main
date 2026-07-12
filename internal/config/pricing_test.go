package config

import "testing"

func TestResolvePlanPrice_NonResellerRetail(t *testing.T) {
	plan := Plan{Price: 5000, WholesalePrice: intPtr(4000)}
	amount, tier := ResolvePlanPrice(plan, false)
	if amount != 5000 || tier != PricingTierRetail {
		t.Fatalf("got amount=%d tier=%s", amount, tier)
	}
}

func TestResolvePlanPrice_ResellerWithWholesale(t *testing.T) {
	plan := Plan{Price: 5000, WholesalePrice: intPtr(4000)}
	amount, tier := ResolvePlanPrice(plan, true)
	if amount != 4000 || tier != PricingTierWholesale {
		t.Fatalf("got amount=%d tier=%s", amount, tier)
	}
}

func TestResolvePlanPrice_ResellerWithoutWholesaleFallsBackRetail(t *testing.T) {
	plan := Plan{Price: 5000}
	amount, tier := ResolvePlanPrice(plan, true)
	if amount != 5000 || tier != PricingTierRetail {
		t.Fatalf("got amount=%d tier=%s", amount, tier)
	}
}

func TestNormalizePlans_RejectsWholesaleAboveRetail(t *testing.T) {
	_, err := normalizePlans([]Plan{{
		Label: "A", Days: 30, Price: 5000, SortOrder: 0, Active: true,
		WholesalePrice: intPtr(6000),
	}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizePlans_RejectsNonPositiveWholesale(t *testing.T) {
	_, err := normalizePlans([]Plan{{
		Label: "A", Days: 30, Price: 5000, SortOrder: 0, Active: true,
		WholesalePrice: intPtr(0),
	}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizePlans_AcceptsValidWholesale(t *testing.T) {
	plans, err := normalizePlans([]Plan{{
		Label: "A", Days: 30, Price: 5000, SortOrder: 0, Active: true,
		WholesalePrice: intPtr(4000),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].WholesalePrice == nil || *plans[0].WholesalePrice != 4000 {
		t.Fatalf("wholesale not preserved: %+v", plans[0].WholesalePrice)
	}
}

func intPtr(v int) *int { return &v }
