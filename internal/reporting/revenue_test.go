package reporting

import (
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

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
