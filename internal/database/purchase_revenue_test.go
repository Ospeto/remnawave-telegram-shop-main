package database

import (
	"strings"
	"testing"
)

func TestBuildRevenueSummaryQueryExcludesAdminTelegramID(t *testing.T) {
	query, err := buildRevenueSummaryQuery(RevenuePeriodDay)
	if err != nil {
		t.Fatalf("buildRevenueSummaryQuery() error = %v", err)
	}

	if !strings.Contains(query, "JOIN customer c ON c.id = p.customer_id") {
		t.Fatalf("buildRevenueSummaryQuery() missing customer join: %s", query)
	}
	if !strings.Contains(query, "($3::bigint = 0 OR c.telegram_id <> $3)") {
		t.Fatalf("buildRevenueSummaryQuery() missing admin exclusion clause: %s", query)
	}
	if !strings.Contains(query, "AT TIME ZONE 'Asia/Yangon'") {
		t.Fatalf("buildRevenueSummaryQuery() missing Yangon timezone bucketing: %s", query)
	}
	if !strings.Contains(query, "p.paid_at >= $1") || !strings.Contains(query, "p.paid_at < $2") {
		t.Fatalf("buildRevenueSummaryQuery() missing explicit range predicates: %s", query)
	}
	if strings.Contains(query, "AT TIME ZONE 'UTC'") {
		t.Fatalf("buildRevenueSummaryQuery() should not regress to UTC bucketing: %s", query)
	}
}

func TestBuildRevenueSummaryQuerySeparatesWalletCashFromServiceRevenue(t *testing.T) {
	query, err := buildRevenueSummaryQuery(RevenuePeriodDay)
	if err != nil {
		t.Fatalf("buildRevenueSummaryQuery() error = %v", err)
	}

	for _, want := range []string{
		"invoice_type <> 'wallet_topup'",
		"invoice_type IN ('crypto', 'mobile_banking', 'wallet_topup')",
		"invoice_type = 'wallet_payment'",
		"period_service_revenue",
		"period_cash_collected",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("buildRevenueSummaryQuery() missing %q in query: %s", want, query)
		}
	}
}

func TestBuildRevenueSummaryQuerySupportsWeeklyAndMonthlyPeriods(t *testing.T) {
	weekly, err := buildRevenueSummaryQuery(RevenuePeriodWeek)
	if err != nil {
		t.Fatalf("weekly buildRevenueSummaryQuery() error = %v", err)
	}
	if !strings.Contains(weekly, "DATE_TRUNC('week'") {
		t.Fatalf("weekly query missing week truncation: %s", weekly)
	}

	monthly, err := buildRevenueSummaryQuery(RevenuePeriodMonth)
	if err != nil {
		t.Fatalf("monthly buildRevenueSummaryQuery() error = %v", err)
	}
	if !strings.Contains(monthly, "DATE_TRUNC('month'") {
		t.Fatalf("monthly query missing month truncation: %s", monthly)
	}
}

func TestBuildRevenueSummaryQueryRejectsUnknownPeriod(t *testing.T) {
	if _, err := buildRevenueSummaryQuery(RevenueSummaryPeriod("quarter")); err == nil {
		t.Fatal("buildRevenueSummaryQuery() error = nil, want unsupported period error")
	}
}
