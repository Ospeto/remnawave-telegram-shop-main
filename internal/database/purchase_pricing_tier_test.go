package database

import (
	"strings"
	"testing"
)

func TestPurchaseHasPricingTierField(t *testing.T) {
	p := Purchase{PricingTier: "wholesale"}
	if p.PricingTier != "wholesale" {
		t.Fatalf("got %q", p.PricingTier)
	}
}

func TestBuildPurchaseInsertIncludesPricingTier(t *testing.T) {
	p := &Purchase{
		Amount:      4000,
		CustomerID:  1,
		Currency:    "MMK",
		Status:      PurchaseStatusPending,
		InvoiceType: InvoiceTypeMobileBanking,
		PricingTier: "wholesale",
	}
	sql, args, err := buildPurchaseInsert(p).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "pricing_tier") {
		t.Fatalf("insert SQL missing pricing_tier: %s", sql)
	}
	found := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "wholesale" {
			found = true
		}
	}
	if !found {
		t.Fatalf("insert args missing wholesale tier: %#v", args)
	}
}

func TestPurchaseColumnsIncludePricingTier(t *testing.T) {
	found := false
	for _, c := range purchaseColumns {
		if c == "pricing_tier" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("purchaseColumns missing pricing_tier")
	}
}
