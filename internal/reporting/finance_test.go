package reporting

import (
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func emptyMetrics() FinanceMetrics { return FinanceMetrics{} }

func TestBuildFinanceReport_NetEqualsGrossMinusRefunds(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, loc)
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          now,
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart:              "2026-07-12",
			Currency:                 "MMK",
			PeriodServiceRevenue:     1000,
			PeriodCashCollected:      800,
			PeriodWalletTopUps:       200,
			PeriodWalletSpend:        200,
			PeriodServicePurchases:   2,
			PeriodNewKeyPurchases:    1,
			PeriodExtensionPurchases: 1,
			RevenueCategory:          "new_key",
			PaymentMethod:            "kbz",
			ServiceRevenue:           600,
			CashCollected:            600,
			TotalPurchases:           1,
		}, {
			PeriodStart:              "2026-07-12",
			Currency:                 "MMK",
			RevenueCategory:          "extension",
			PaymentMethod:            "wallet",
			ServiceRevenue:           400,
			WalletSpend:              400,
			TotalPurchases:           1,
			PeriodServiceRevenue:     1000,
			PeriodCashCollected:      800,
			PeriodWalletTopUps:       200,
			PeriodWalletSpend:        200,
			PeriodServicePurchases:   2,
			PeriodNewKeyPurchases:    1,
			PeriodExtensionPurchases: 1,
		}},
		RefundRows: []database.RefundPeriodRow{{
			PeriodStart: "2026-07-12",
			Currency:    "MMK",
			RefundTotal: 100,
			RefundCount: 1,
		}},
		RangeUniqueCustomers: 2,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.GrossServiceRevenue != 1000 {
		t.Fatalf("gross=%v", report.Current.GrossServiceRevenue)
	}
	if report.Current.Refunds != 100 {
		t.Fatalf("refunds=%v", report.Current.Refunds)
	}
	if report.Current.NetServiceRevenue != 900 {
		t.Fatalf("net=%v", report.Current.NetServiceRevenue)
	}
	if report.Current.UniqueCustomers != 2 {
		t.Fatalf("customers=%d", report.Current.UniqueCustomers)
	}
	if report.Current.AverageOrderValue != 500 {
		t.Fatalf("aov=%v", report.Current.AverageOrderValue)
	}
	if !report.InProgress {
		t.Fatal("current day must be in progress")
	}
	if report.Delta != nil && report.Prior == nil {
		t.Fatal("delta requires prior")
	}
}

func TestBuildFinanceReport_PriorDeltaAndTrendOrder(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, loc)
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          now,
		CurrentStart: time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PriorStart:   time.Date(2026, 7, 10, 0, 0, 0, 0, loc),
		PriorEnd:     time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-11", Currency: "MMK", PeriodServiceRevenue: 500, PeriodServicePurchases: 1, ServiceRevenue: 500, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 1000, PeriodServicePurchases: 2, ServiceRevenue: 1000, TotalPurchases: 2, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		RangeUniqueCustomers: 2,
		PriorPurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-10", Currency: "MMK", PeriodServiceRevenue: 500, PeriodServicePurchases: 1, ServiceRevenue: 500, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		PriorUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Prior == nil || report.Prior.GrossServiceRevenue != 500 {
		t.Fatalf("prior=%+v", report.Prior)
	}
	// Current window spans two day buckets (500+1000=1500); prior is 500 → absolute delta 1000.
	if report.Delta == nil || report.Delta.GrossServiceRevenue.Absolute != 1000 {
		t.Fatalf("delta=%+v", report.Delta)
	}
	if len(report.Trend) < 2 {
		t.Fatalf("trend len=%d", len(report.Trend))
	}
	if report.Trend[0].PeriodStart > report.Trend[1].PeriodStart {
		t.Fatal("trend must be ascending")
	}
}

func TestBuildFinanceReport_CustomInclusiveRange(t *testing.T) {
	loc := YangonLocation()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodCustom,
		Now:          time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CustomFrom:   &from,
		CustomTo:     &to,
		CurrentStart: from,
		CurrentEnd:   time.Date(2026, 2, 1, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "2026-01-15", Currency: "MMK",
			PeriodServiceRevenue: 100, PeriodServicePurchases: 1,
			ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
		RangeUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.RangeStart != "2026-01-01" || report.RangeEnd != "2026-01-31" {
		t.Fatalf("range %s..%s", report.RangeStart, report.RangeEnd)
	}
	if report.InProgress {
		t.Fatal("historical custom range must not be in progress")
	}
}
