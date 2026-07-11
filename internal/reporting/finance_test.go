package reporting

import (
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

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
	// Selected = today only; history window has 3 days of purchase rows for dense trend.
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          now,
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PriorStart:   time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		PriorEnd:     time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		TrendStart:   time.Date(2026, 7, 10, 0, 0, 0, 0, loc),
		TrendEnd:     time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-10", Currency: "MMK", PeriodServiceRevenue: 200, PeriodServicePurchases: 1, ServiceRevenue: 200, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-11", Currency: "MMK", PeriodServiceRevenue: 500, PeriodServicePurchases: 1, ServiceRevenue: 500, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 1000, PeriodServicePurchases: 2, ServiceRevenue: 1000, TotalPurchases: 2, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		RangeUniqueCustomers: 2,
		PriorPurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-11", Currency: "MMK", PeriodServiceRevenue: 500, PeriodServicePurchases: 1, ServiceRevenue: 500, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		PriorUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	// Current = selected day only (1000), not sum of history.
	if report.Current.GrossServiceRevenue != 1000 {
		t.Fatalf("current gross=%v want 1000 (selected day only)", report.Current.GrossServiceRevenue)
	}
	if report.Prior == nil || report.Prior.GrossServiceRevenue != 500 {
		t.Fatalf("prior=%+v", report.Prior)
	}
	// Delta = today − yesterday = 1000 − 500 = 500.
	if report.Delta == nil || report.Delta.GrossServiceRevenue.Absolute != 500 {
		t.Fatalf("delta=%+v", report.Delta)
	}
	if len(report.Trend) != 3 {
		t.Fatalf("trend len=%d want 3 dense days", len(report.Trend))
	}
	if report.Trend[0].PeriodStart > report.Trend[1].PeriodStart {
		t.Fatal("trend must be ascending")
	}
	if report.RangeStart != "2026-07-12" || report.RangeEnd != "2026-07-12" {
		t.Fatalf("range metadata must be selected period, got %s..%s", report.RangeStart, report.RangeEnd)
	}
}

func TestBuildFinanceReport_SelectedPeriodNotWindowSum(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, loc)
	// 3 days of data with periods=7 style history window — Current must be today only.
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          now,
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PriorStart:   time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		PriorEnd:     time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		TrendStart:   time.Date(2026, 7, 6, 0, 0, 0, 0, loc),
		TrendEnd:     time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-10", Currency: "MMK", PeriodServiceRevenue: 100, PeriodServicePurchases: 1, ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-11", Currency: "MMK", PeriodServiceRevenue: 200, PeriodServicePurchases: 1, ServiceRevenue: 200, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 300, PeriodServicePurchases: 1, ServiceRevenue: 300, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		RangeUniqueCustomers: 1,
		PriorPurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-11", Currency: "MMK", PeriodServiceRevenue: 200, PeriodServicePurchases: 1, ServiceRevenue: 200, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		PriorUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.GrossServiceRevenue != 300 {
		t.Fatalf("current=%v want 300 (today only, not 100+200+300)", report.Current.GrossServiceRevenue)
	}
	if report.Prior == nil || report.Prior.GrossServiceRevenue != 200 {
		t.Fatalf("prior=%+v want 200 (yesterday only)", report.Prior)
	}
	if report.Delta == nil || report.Delta.GrossServiceRevenue.Absolute != 100 {
		t.Fatalf("delta=%+v want 100", report.Delta)
	}
	// Dense trend: 7 day buckets even with only 3 activity days.
	if len(report.Trend) != 7 {
		t.Fatalf("trend len=%d want 7 dense buckets", len(report.Trend))
	}
	// Gap day 2026-07-09 (no purchases) must appear with zeros.
	var foundGap bool
	for _, b := range report.Trend {
		if b.PeriodStart == "2026-07-09" {
			foundGap = true
			if b.Metrics.GrossServiceRevenue != 0 || b.Metrics.NetServiceRevenue != 0 || b.Metrics.SuccessfulOrders != 0 {
				t.Fatalf("gap day metrics not zero: %+v", b.Metrics)
			}
		}
	}
	if !foundGap {
		t.Fatal("expected dense zero bucket for 2026-07-09")
	}
}

func TestBuildFinanceReport_DenseTrendZeroActivityDay(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, loc)
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          now,
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		TrendStart:   time.Date(2026, 7, 10, 0, 0, 0, 0, loc),
		TrendEnd:     time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-10", Currency: "MMK", PeriodServiceRevenue: 50, PeriodServicePurchases: 1, ServiceRevenue: 50, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			// gap on 2026-07-11
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 80, PeriodServicePurchases: 1, ServiceRevenue: 80, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		RangeUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Trend) != 3 {
		t.Fatalf("trend len=%d want 3", len(report.Trend))
	}
	if report.Trend[1].PeriodStart != "2026-07-11" {
		t.Fatalf("middle bucket=%s want 2026-07-11", report.Trend[1].PeriodStart)
	}
	if report.Trend[1].Metrics.GrossServiceRevenue != 0 || report.Trend[1].Metrics.Refunds != 0 || report.Trend[1].Metrics.SuccessfulOrders != 0 {
		t.Fatalf("gap metrics=%+v", report.Trend[1].Metrics)
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

func TestBuildFinanceReport_RejectsMixedCurrency(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 100, ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-12", Currency: "USD", PeriodServiceRevenue: 5, ServiceRevenue: 5, TotalPurchases: 1, PaymentMethod: "crypto", RevenueCategory: "new_key"},
		},
	}
	_, err := BuildFinanceReport(in)
	if !errors.Is(err, ErrMixedCurrency) {
		t.Fatalf("err=%v want ErrMixedCurrency", err)
	}
}

func TestBuildFinanceReport_RejectsMixedCurrencyAcrossRefunds(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 100, ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		RefundRows: []database.RefundPeriodRow{
			{PeriodStart: "2026-07-12", Currency: "USD", RefundTotal: 10, RefundCount: 1},
		},
	}
	_, err := BuildFinanceReport(in)
	if !errors.Is(err, ErrMixedCurrency) {
		t.Fatalf("err=%v want ErrMixedCurrency", err)
	}
}

func TestBuildFinanceReport_RejectsMixedCurrencyAcrossPrior(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PriorStart:   time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		PriorEnd:     time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 100, ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		PriorPurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-11", Currency: "THB", PeriodServiceRevenue: 50, ServiceRevenue: 50, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
	}
	_, err := BuildFinanceReport(in)
	if !errors.Is(err, ErrMixedCurrency) {
		t.Fatalf("err=%v want ErrMixedCurrency", err)
	}
}

func TestBuildFinanceReport_MalformedPeriodStart(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "not-a-date", Currency: "MMK",
			PeriodServiceRevenue: 100, PeriodServicePurchases: 1,
			ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
	}
	_, err := BuildFinanceReport(in)
	if !errors.Is(err, ErrInvalidPeriodStart) {
		t.Fatalf("err=%v want ErrInvalidPeriodStart", err)
	}
}

func TestBuildFinanceReport_MalformedPriorPurchasePeriodStart(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PriorStart:   time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		PriorEnd:     time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "2026-07-12", Currency: "MMK",
			PeriodServiceRevenue: 100, PeriodServicePurchases: 1,
			ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
		PriorPurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "bad-prior-date", Currency: "MMK",
			PeriodServiceRevenue: 50, PeriodServicePurchases: 1,
			ServiceRevenue: 50, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
		PriorUniqueCustomers: 1,
	}
	_, err := BuildFinanceReport(in)
	if !errors.Is(err, ErrInvalidPeriodStart) {
		t.Fatalf("err=%v want ErrInvalidPeriodStart", err)
	}
}

func TestBuildFinanceReport_MalformedPriorRefundPeriodStart(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PriorStart:   time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		PriorEnd:     time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "2026-07-12", Currency: "MMK",
			PeriodServiceRevenue: 100, PeriodServicePurchases: 1,
			ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
		PriorPurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "2026-07-11", Currency: "MMK",
			PeriodServiceRevenue: 50, PeriodServicePurchases: 1,
			ServiceRevenue: 50, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
		PriorRefundRows: []database.RefundPeriodRow{{
			PeriodStart: "not-a-refund-date",
			Currency:    "MMK",
			RefundTotal: 10,
			RefundCount: 1,
		}},
		PriorUniqueCustomers: 1,
	}
	_, err := BuildFinanceReport(in)
	if !errors.Is(err, ErrInvalidPeriodStart) {
		t.Fatalf("err=%v want ErrInvalidPeriodStart", err)
	}
}

func TestBuildFinanceReport_MalformedCurrentRefundPeriodStart(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "2026-07-12", Currency: "MMK",
			PeriodServiceRevenue: 100, PeriodServicePurchases: 1,
			ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
		RefundRows: []database.RefundPeriodRow{{
			PeriodStart: "bad-refund",
			Currency:    "MMK",
			RefundTotal: 5,
			RefundCount: 1,
		}},
	}
	_, err := BuildFinanceReport(in)
	if !errors.Is(err, ErrInvalidPeriodStart) {
		t.Fatalf("err=%v want ErrInvalidPeriodStart", err)
	}
}

func TestBuildFinanceReport_WalletTopUpCashNotServiceRevenue(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart:            "2026-07-12",
			Currency:               "MMK",
			InvoiceType:            string(database.InvoiceTypeWalletTopUp),
			RevenueCategory:        "wallet_topup",
			PaymentMethod:          "kbz",
			PeriodServiceRevenue:   0,
			PeriodCashCollected:    500,
			PeriodWalletTopUps:     500,
			PeriodServicePurchases: 0,
			ServiceRevenue:         0,
			CashCollected:          500,
			WalletTopUps:           500,
			TotalPurchases:         1,
		}},
		RangeUniqueCustomers: 0,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.GrossServiceRevenue != 0 {
		t.Fatalf("gross service=%v want 0", report.Current.GrossServiceRevenue)
	}
	if report.Current.CashCollected != 500 {
		t.Fatalf("cash=%v want 500", report.Current.CashCollected)
	}
	if report.Current.WalletTopUps != 500 {
		t.Fatalf("topups=%v want 500", report.Current.WalletTopUps)
	}
	if len(report.Categories) != 1 || report.Categories[0].Category != "wallet_topup" || report.Categories[0].Amount != 500 {
		t.Fatalf("categories=%+v", report.Categories)
	}
	if len(report.Methods) != 1 || report.Methods[0].ServiceRevenue != 0 || report.Methods[0].CashCollected != 500 {
		t.Fatalf("methods=%+v", report.Methods)
	}
}

func TestBuildFinanceReport_WalletSpendServiceRevenueNoCash(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart:            "2026-07-12",
			Currency:               "MMK",
			InvoiceType:            string(database.InvoiceTypeWalletPayment),
			RevenueCategory:        "extension",
			PaymentMethod:          "wallet",
			PeriodServiceRevenue:   300,
			PeriodCashCollected:    0,
			PeriodWalletSpend:      300,
			PeriodServicePurchases: 1,
			ServiceRevenue:         300,
			CashCollected:          0,
			WalletSpend:            300,
			TotalPurchases:         1,
		}},
		RangeUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.GrossServiceRevenue != 300 {
		t.Fatalf("gross=%v want 300", report.Current.GrossServiceRevenue)
	}
	if report.Current.CashCollected != 0 {
		t.Fatalf("cash=%v want 0", report.Current.CashCollected)
	}
	if report.Current.WalletSpend != 300 {
		t.Fatalf("wallet spend=%v want 300", report.Current.WalletSpend)
	}
	if len(report.Methods) != 1 || report.Methods[0].ServiceRevenue != 300 || report.Methods[0].CashCollected != 0 {
		t.Fatalf("methods=%+v", report.Methods)
	}
}

func TestBuildFinanceReport_UniqueCustomersFromRangeInput(t *testing.T) {
	loc := YangonLocation()
	// Selected = today; history rows may include other days — unique customers come from RangeUniqueCustomers for selected.
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		TrendStart:   time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		TrendEnd:     time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		// Bucket-level unique fields intentionally differ from range input; builder must ignore them.
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-11", Currency: "MMK", PeriodServiceRevenue: 100, PeriodServicePurchases: 1, PeriodUniqueCustomers: 9, ServiceRevenue: 100, TotalPurchases: 1, UniqueCustomers: 9, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 200, PeriodServicePurchases: 2, PeriodUniqueCustomers: 4, ServiceRevenue: 200, TotalPurchases: 2, UniqueCustomers: 4, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		RangeUniqueCustomers: 7,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.UniqueCustomers != 7 {
		t.Fatalf("unique customers=%d want RangeUniqueCustomers=7 (not bucket sum)", report.Current.UniqueCustomers)
	}
	// Categories/methods for selected day only.
	if report.Current.GrossServiceRevenue != 200 {
		t.Fatalf("current gross=%v want 200 (selected day)", report.Current.GrossServiceRevenue)
	}
}

func TestBuildFinanceReport_TwoDecimalNormalization(t *testing.T) {
	loc := YangonLocation()
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          time.Date(2026, 7, 12, 12, 0, 0, 0, loc),
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart:            "2026-07-12",
			Currency:               "MMK",
			PeriodServiceRevenue:   10.005,
			PeriodCashCollected:    10.005,
			PeriodServicePurchases: 1,
			ServiceRevenue:         10.005,
			CashCollected:          10.005,
			TotalPurchases:         1,
			PaymentMethod:          "kbz",
			RevenueCategory:        "new_key",
		}},
		RefundRows: []database.RefundPeriodRow{{
			PeriodStart: "2026-07-12",
			Currency:    "MMK",
			RefundTotal: 1.004,
			RefundCount: 1,
		}},
		RangeUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	// RoundMoney half-away-from-zero: 10.005→10.01, 1.004→1.00, net 9.01
	if report.Current.GrossServiceRevenue != 10.01 {
		t.Fatalf("gross=%v want 10.01", report.Current.GrossServiceRevenue)
	}
	if report.Current.Refunds != 1.00 {
		t.Fatalf("refunds=%v want 1.00", report.Current.Refunds)
	}
	if report.Current.NetServiceRevenue != 9.01 {
		t.Fatalf("net=%v want 9.01", report.Current.NetServiceRevenue)
	}
	if report.Current.CashCollected != 10.01 {
		t.Fatalf("cash=%v want 10.01", report.Current.CashCollected)
	}
	if report.Current.AverageOrderValue != 10.01 {
		t.Fatalf("aov=%v want 10.01", report.Current.AverageOrderValue)
	}
	if len(report.Categories) != 1 || report.Categories[0].Amount != 10.01 {
		t.Fatalf("category amount=%+v want 10.01", report.Categories)
	}
	if len(report.Methods) != 1 || report.Methods[0].ServiceRevenue != 10.01 || report.Methods[0].CashCollected != 10.01 {
		t.Fatalf("method money=%+v", report.Methods)
	}
	if len(report.Trend) != 1 {
		t.Fatalf("trend len=%d", len(report.Trend))
	}
	if report.Trend[0].Metrics.GrossServiceRevenue != 10.01 || report.Trend[0].Metrics.Refunds != 1.00 {
		t.Fatalf("trend metrics=%+v", report.Trend[0].Metrics)
	}
}

func TestFormatFinanceReportCSV_MatchesJSONTotals(t *testing.T) {
	report := FinanceReport{
		Period: "day", Timezone: "Asia/Yangon", Currency: "MMK",
		RangeStart: "2026-07-12", RangeEnd: "2026-07-12",
		Current: FinanceMetrics{
			GrossServiceRevenue: 1000, Refunds: 100, NetServiceRevenue: 900,
			CashCollected: 800, WalletTopUps: 200, WalletSpend: 200,
			SuccessfulOrders: 2, UniqueCustomers: 2, AverageOrderValue: 500,
			NewSubscriptions: 1, Extensions: 1,
		},
		Methods: []MethodBreakdown{{Method: "kbz", Transactions: 1, ServiceRevenue: 600, CashCollected: 600}},
		Categories: []CategoryBreakdown{{Category: "new_key", Orders: 1, Amount: 600}},
		Trend: []FinanceTrendBucket{{
			PeriodStart: "2026-07-12", PeriodEnd: "2026-07-12",
			Metrics:    FinanceMetrics{GrossServiceRevenue: 1000, Refunds: 100, NetServiceRevenue: 900},
			Categories: []CategoryBreakdown{{Category: "new_key", Orders: 1, Amount: 600}},
			Methods:    []MethodBreakdown{{Method: "kbz", Transactions: 1, ServiceRevenue: 600, CashCollected: 600}},
		}},
	}
	csvBytes, err := FormatFinanceReportCSV(report)
	if err != nil {
		t.Fatal(err)
	}
	s := string(csvBytes)
	for _, want := range []string{
		"gross_service_revenue,1000.00",
		"refunds,100.00",
		"net_service_revenue,900.00",
		"cash_collected,800.00",
		"unique_customers,2",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("csv missing %q in %s", want, s)
		}
	}
}

func TestFormatFinanceReportCSV_FullRecordsAndEscaping(t *testing.T) {
	report := FinanceReport{
		Period: "day", Timezone: "Asia/Yangon", Currency: "MMK",
		RangeStart: "2026-07-12", RangeEnd: "2026-07-12",
		Current: FinanceMetrics{GrossServiceRevenue: 10, NetServiceRevenue: 10, SuccessfulOrders: 1},
		Categories: []CategoryBreakdown{
			{Category: "new_key, \"quoted\"", Orders: 1, Amount: 10},
			{Category: "line\nbreak", Orders: 2, Amount: 20},
		},
		Methods: []MethodBreakdown{
			{Method: "kbz,pay", Transactions: 1, ServiceRevenue: 10, CashCollected: 10},
		},
		Trend: []FinanceTrendBucket{{
			PeriodStart: "2026-07-12",
			Metrics:     FinanceMetrics{GrossServiceRevenue: 10, NetServiceRevenue: 10},
		}},
	}
	csvBytes, err := FormatFinanceReportCSV(report)
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(string(csvBytes)))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v\n%s", err, csvBytes)
	}
	if len(records) < 2 {
		t.Fatalf("too few records: %d", len(records))
	}
	if got := records[0]; len(got) != 3 || got[0] != "section" || got[1] != "key" || got[2] != "value" {
		t.Fatalf("header=%v", records[0])
	}
	// Find escaped category labels via full record equality (not substrings).
	var foundQuoted, foundNewline, foundMethod bool
	for _, rec := range records {
		if len(rec) != 3 {
			t.Fatalf("record width=%d want 3: %v", len(rec), rec)
		}
		if rec[0] == "category" && rec[1] == "new_key, \"quoted\"" {
			foundQuoted = true
			if rec[2] != "1:10.00" {
				t.Fatalf("quoted category value=%q", rec[2])
			}
		}
		if rec[0] == "category" && rec[1] == "line\nbreak" {
			foundNewline = true
		}
		if rec[0] == "method" && rec[1] == "kbz,pay" {
			foundMethod = true
			if rec[2] != "1:10.00:10.00" {
				t.Fatalf("method value=%q", rec[2])
			}
		}
	}
	if !foundQuoted || !foundNewline || !foundMethod {
		t.Fatalf("missing escaped records quoted=%v newline=%v method=%v in %v", foundQuoted, foundNewline, foundMethod, records)
	}
}
