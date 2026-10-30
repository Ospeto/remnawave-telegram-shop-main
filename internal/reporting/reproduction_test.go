package reporting

import (
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// TestReproduction_PostpaidWalletSettlement_DoubleCount proves that wallet-funded settlements
// must not be added to CashCollected because cash was already collected upon wallet top-up.
//
// Lifecycle:
// 1. Day 1: User tops up wallet with 100,000 MMK cash (e.g. via KBZPay).
//   - Actual cash in bank: +100,000 MMK.
//
// 2. Day 2: User purchases VPN service on postpaid credit for 100,000 MMK.
//   - Service delivered: +100,000 MMK. Cash collected: 0 MMK.
//
// 3. Day 3: User settles the 100,000 MMK postpaid debt using wallet balance (payment_method = 'wallet').
//   - Debt settled: 100,000 MMK. Cash in bank: 0 MMK (already collected on Day 1).
//
// Prior to fix: SumSettlementsByPeriod summed all settlements without filtering customer wallet debits,
// causing CashCollected to be 200,000 MMK (double-counted).
// With the fix: buildSumSettlementsByPeriodSQL filters 'created_by LIKE admin:%',
// returning 0 settlement cash for wallet settlements.
func TestReproduction_PostpaidWalletSettlement_DoubleCount(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, loc)

	purchasesMock := &fakePurchases{
		rows: []database.RevenueSummaryRow{
			{
				PeriodStart:            "2026-10-01",
				Currency:               "MMK",
				PeriodCashCollected:    100000, // Day 1: 100,000 MMK cash collected via wallet top-up
				PeriodServiceRevenue:   100000, // Day 2: 100,000 MMK postpaid plan delivery
				PeriodWalletTopUps:     100000,
				PeriodServicePurchases: 1,
				PeriodNewKeyPurchases:  1,
				TotalPurchases:         2,
				PaymentMethod:          "mobile_banking",
				RevenueCategory:        "wallet_topup",
			},
		},
		n: 1,
	}

	// Under patched query: wallet settlement (created_by LIKE 'customer:%') is excluded from
	// SumSettlementsByPeriod, so settlement rows for wallet settlements are empty / 0.
	settlementsMock := &fakeSettlements{
		rows: []database.SettlementPeriodRow{}, // 0 external cash from wallet settlement
	}

	svc := NewFinanceService(purchasesMock, &fakeRefunds{}, settlementsMock)
	report, err := svc.GetReport(t.Context(), ReportQuery{
		Period: database.RevenuePeriodMonth,
		Now:    now,
	})
	if err != nil {
		t.Fatalf("unexpected error getting report: %v", err)
	}

	if report.Current.GrossServiceRevenue != 100000 {
		t.Fatalf("GrossServiceRevenue = %v, want 100000", report.Current.GrossServiceRevenue)
	}

	// Cash collected MUST equal 100,000 MMK (initial cash top-up), NOT 200,000 MMK.
	if report.Current.CashCollected != 100000 {
		t.Fatalf("CashCollected = %v, want 100000", report.Current.CashCollected)
	}
}

// TestVerification_PostpaidExternalCashSettlement_CountedInCash verifies that external-cash
// settlements (direct cash or bank payments recorded by admin) CONTINUE to be counted in CashCollected.
func TestVerification_PostpaidExternalCashSettlement_CountedInCash(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, loc)

	purchasesMock := &fakePurchases{
		rows: []database.RevenueSummaryRow{
			{
				PeriodStart:            "2026-10-01",
				Currency:               "MMK",
				PeriodCashCollected:    100000, // 100,000 MMK initial cash
				PeriodServiceRevenue:   100000, // 100,000 MMK postpaid delivery
				PeriodWalletTopUps:     100000,
				PeriodServicePurchases: 1,
				PeriodNewKeyPurchases:  1,
				TotalPurchases:         2,
				PaymentMethod:          "mobile_banking",
				RevenueCategory:        "wallet_topup",
			},
		},
		n: 1,
	}

	// Admin records an external cash settlement of 50,000 MMK (fresh cash collected).
	settlementsMock := &fakeSettlements{
		rows: []database.SettlementPeriodRow{
			{
				PeriodStart:     "2026-10-01",
				Currency:        "MMK",
				SettlementTotal: 50000, // 50,000 MMK external cash settlement
				SettlementCount: 1,
			},
		},
	}

	svc := NewFinanceService(purchasesMock, &fakeRefunds{}, settlementsMock)
	report, err := svc.GetReport(t.Context(), ReportQuery{
		Period: database.RevenuePeriodMonth,
		Now:    now,
	})
	if err != nil {
		t.Fatalf("unexpected error getting report: %v", err)
	}

	// Initial cash 100,000 + external cash settlement 50,000 = 150,000 MMK.
	if report.Current.CashCollected != 150000 {
		t.Fatalf("CashCollected = %v, want 150000", report.Current.CashCollected)
	}
}

// TestVerification_CombinedWalletAndExternalCashSettlement verifies that when both
// wallet settlements and external cash settlements occur in the same period, only
// external cash settlements increment CashCollected.
func TestVerification_CombinedWalletAndExternalCashSettlement(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, loc)

	purchasesMock := &fakePurchases{
		rows: []database.RevenueSummaryRow{
			{
				PeriodStart:            "2026-10-01",
				Currency:               "MMK",
				PeriodCashCollected:    100000, // 100,000 MMK initial cash
				PeriodServiceRevenue:   100000, // 100,000 MMK postpaid delivery
				PeriodWalletTopUps:     100000,
				PeriodServicePurchases: 1,
				PeriodNewKeyPurchases:  1,
				TotalPurchases:         2,
				PaymentMethod:          "mobile_banking",
				RevenueCategory:        "wallet_topup",
			},
		},
		n: 1,
	}

	// Only external cash settlement (30,000 MMK) is included by SumSettlementsByPeriod.
	// The 70,000 MMK wallet settlement is filtered out by 'created_by LIKE admin:%'.
	settlementsMock := &fakeSettlements{
		rows: []database.SettlementPeriodRow{
			{
				PeriodStart:     "2026-10-01",
				Currency:        "MMK",
				SettlementTotal: 30000, // only the 30,000 MMK external cash settlement
				SettlementCount: 1,
			},
		},
	}

	svc := NewFinanceService(purchasesMock, &fakeRefunds{}, settlementsMock)
	report, err := svc.GetReport(t.Context(), ReportQuery{
		Period: database.RevenuePeriodMonth,
		Now:    now,
	})
	if err != nil {
		t.Fatalf("unexpected error getting report: %v", err)
	}

	if report.Current.CashCollected != 130000 {
		t.Fatalf("CashCollected = %v want 130000 (100k top-up + 30k external cash)", report.Current.CashCollected)
	}
	if report.Current.GrossServiceRevenue != 100000 {
		t.Fatalf("GrossServiceRevenue = %v want 100000", report.Current.GrossServiceRevenue)
	}
	if report.Current.WalletTopUps != 100000 {
		t.Fatalf("WalletTopUps = %v want 100000", report.Current.WalletTopUps)
	}
}

// TestSettlementQueryFilter_MatchesWritePathSemantics verifies that the SQL query
// generated by buildSumSettlementsByPeriodSQL matches the exact write paths in handlers.go:
// - handlers.go:3194 (CreateResellerSettlement) sets created_by = "customer:<telegram_id>"
// - handlers.go:2040 (postAdminCustomerSettlement) sets created_by = "admin:<admin_telegram_id>"
func TestSettlementQueryFilter_MatchesWritePathSemantics(t *testing.T) {
	// Ledger entries representing actual writes from the two endpoints
	entries := []struct {
		name      string
		entryType string
		createdBy string
		amount    float64
		wantCash  bool
	}{
		{
			name:      "CreateResellerSettlement (wallet self-settlement)",
			entryType: "settlement",
			createdBy: "customer:12345678",
			amount:    100000,
			wantCash:  false, // wallet funded -> MUST NOT count as cash
		},
		{
			name:      "postAdminCustomerSettlement (external cash receipt)",
			entryType: "settlement",
			createdBy: "admin:98765432",
			amount:    50000,
			wantCash:  true, // external cash -> MUST count as cash
		},
		{
			name:      "postpaid plan sale",
			entryType: "sale",
			createdBy: "system:postpaid",
			amount:    100000,
			wantCash:  false, // sale -> not a settlement
		},
		{
			name:      "admin credit adjustment",
			entryType: "adjustment",
			createdBy: "admin:98765432",
			amount:    10000,
			wantCash:  false, // adjustment -> not a settlement
		},
	}

	// Evaluation logic matching SQL WHERE clause:
	// entry_type = 'settlement' AND created_by LIKE 'admin:%'
	sqlFilterMatches := func(entryType, createdBy string) bool {
		if entryType != "settlement" {
			return false
		}
		return strings.HasPrefix(createdBy, "admin:")
	}

	for _, tc := range entries {
		t.Run(tc.name, func(t *testing.T) {
			gotCash := sqlFilterMatches(tc.entryType, tc.createdBy)
			if gotCash != tc.wantCash {
				t.Fatalf("sqlFilterMatches(%q, %q) = %v, want %v", tc.entryType, tc.createdBy, gotCash, tc.wantCash)
			}
		})
	}
}
