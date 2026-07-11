package reporting

import (
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestFormatTelegramPeriodRevenueReportSeparatesWalletMovements(t *testing.T) {
	rows := []database.RevenueSummaryRow{
		{
			PeriodStart:                "2026-04-13",
			PaymentMethod:              "kpay",
			InvoiceType:                string(database.InvoiceTypeMobileBanking),
			Currency:                   "MMK",
			TotalPurchases:             1,
			TotalRevenue:               8000,
			ServiceRevenue:             8000,
			CashCollected:              8000,
			NewKeyPurchases:            1,
			PeriodTotalPurchases:       3,
			PeriodServicePurchases:     2,
			PeriodUniqueCustomers:      2,
			PeriodServiceRevenue:       15000,
			PeriodCashCollected:        18000,
			PeriodWalletTopUps:         10000,
			PeriodWalletSpend:          7000,
			PeriodNewKeyPurchases:      1,
			PeriodExtensionPurchases:   1,
			PeriodWalletTopUpPurchases: 1,
		},
		{
			PeriodStart:                "2026-04-13",
			PaymentMethod:              "wallet",
			InvoiceType:                string(database.InvoiceTypeWalletPayment),
			Currency:                   "MMK",
			TotalPurchases:             1,
			TotalRevenue:               7000,
			ServiceRevenue:             7000,
			WalletSpend:                7000,
			ExtensionPurchases:         1,
			PeriodTotalPurchases:       3,
			PeriodServicePurchases:     2,
			PeriodUniqueCustomers:      2,
			PeriodServiceRevenue:       15000,
			PeriodCashCollected:        18000,
			PeriodWalletTopUps:         10000,
			PeriodWalletSpend:          7000,
			PeriodNewKeyPurchases:      1,
			PeriodExtensionPurchases:   1,
			PeriodWalletTopUpPurchases: 1,
		},
		{
			PeriodStart:                "2026-04-13",
			PaymentMethod:              "kpay",
			InvoiceType:                string(database.InvoiceTypeWalletTopUp),
			Currency:                   "MMK",
			TotalPurchases:             1,
			CashCollected:              10000,
			WalletTopUps:               10000,
			WalletTopUpPurchases:       1,
			PeriodTotalPurchases:       3,
			PeriodServicePurchases:     2,
			PeriodUniqueCustomers:      2,
			PeriodServiceRevenue:       15000,
			PeriodCashCollected:        18000,
			PeriodWalletTopUps:         10000,
			PeriodWalletSpend:          7000,
			PeriodNewKeyPurchases:      1,
			PeriodExtensionPurchases:   1,
			PeriodWalletTopUpPurchases: 1,
		},
	}

	report := FormatTelegramPeriodRevenueReport("Weekly Revenue Report", "2026-04-13 to 2026-04-19", rows)

	for _, want := range []string{
		"Service revenue: <b>15,000 MMK</b>",
		"Cash collected: <b>18,000 MMK</b>",
		"Wallet: 10,000 MMK top-ups, 7,000 MMK wallet spend",
		"Mix: 1 new keys, 1 extensions, 1 top-ups",
		"kpay: service 8,000 MMK, cash 18,000 MMK",
		"wallet: service 7,000 MMK, cash 0 MMK",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}

	if strings.Contains(report, "25,000 MMK") {
		t.Fatalf("report appears to double count wallet top-up plus spend:\n%s", report)
	}
}

func TestPreviousWeekRangeUsesCompletedYangonWeek(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, loc)
	start, end := PreviousWeekRange(now)

	if got, want := start.Format("2006-01-02"), "2026-04-13"; got != want {
		t.Fatalf("week start = %s, want %s", got, want)
	}
	if got, want := end.Format("2006-01-02"), "2026-04-20"; got != want {
		t.Fatalf("week end = %s, want %s", got, want)
	}
	if got, want := FormatDateRange(start, end), "2026-04-13 to 2026-04-19"; got != want {
		t.Fatalf("FormatDateRange() = %s, want %s", got, want)
	}
}

func TestSummarizeRevenuePeriod_PreservesZeroServiceRevenue(t *testing.T) {
	rows := []database.RevenueSummaryRow{{
		PeriodStart:          "2026-07-01",
		Currency:             "MMK",
		PeriodServiceRevenue: 0,
		PeriodCashCollected:  0,
		PeriodTotalPurchases: 0,
		ServiceRevenue:       0,
		TotalRevenue:         0,
	}}
	totals, _ := SummarizeRevenuePeriod(rows)
	if totals.ServiceRevenue != 0 {
		t.Fatalf("service=%v want 0", totals.ServiceRevenue)
	}
}

func TestSummarizeRevenuePeriod_PeriodZeroWinsOverBreakdownFallback(t *testing.T) {
	rows := []database.RevenueSummaryRow{{
		PeriodStart:          "2026-07-01",
		Currency:             "MMK",
		PeriodServiceRevenue: 0,
		TotalRevenue:         999,
		ServiceRevenue:       999,
	}}
	totals, _ := SummarizeRevenuePeriod(rows)
	if totals.ServiceRevenue != 0 {
		t.Fatalf("got %v want 0 (period field is authoritative including zero)", totals.ServiceRevenue)
	}
}

func TestFormatTelegramFinanceReport_IncludesNetAndRefunds(t *testing.T) {
	report := FinanceReport{
		Period: "day", Currency: "MMK", RangeStart: "2026-07-11", RangeEnd: "2026-07-11",
		Current: FinanceMetrics{
			GrossServiceRevenue: 1000,
			Refunds:             100,
			NetServiceRevenue:   900,
			CashCollected:       800,
			SuccessfulOrders:    2,
			UniqueCustomers:     2,
		},
	}
	text := FormatTelegramFinanceReport("Daily Revenue Report", report)
	for _, want := range []string{"Net Income", "900", "Gross", "1,000", "Refunds", "100", "Cash"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestFormatTelegramFinanceReport_InProgressLabel(t *testing.T) {
	report := FinanceReport{
		Period: "day", Currency: "MMK", RangeStart: "2026-07-12", RangeEnd: "2026-07-12",
		InProgress: true,
		Current:    FinanceMetrics{NetServiceRevenue: 0},
	}
	text := FormatTelegramFinanceReport("Daily Revenue Report", report)
	if !strings.Contains(text, "In progress") {
		t.Fatalf("missing In progress: %s", text)
	}
}
