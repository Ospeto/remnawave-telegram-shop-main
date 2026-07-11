package database

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeRevenueSummaryPeriod_YearAndCustom(t *testing.T) {
	y, err := NormalizeRevenueSummaryPeriod("year")
	if err != nil || y != RevenuePeriodYear {
		t.Fatalf("year: got %q err=%v", y, err)
	}
	c, err := NormalizeRevenueSummaryPeriod("custom")
	if err != nil || c != RevenuePeriodCustom {
		t.Fatalf("custom: got %q err=%v", c, err)
	}
	if _, err := NormalizeRevenueSummaryPeriod("quarter"); err == nil {
		t.Fatal("expected error for quarter")
	}
}

func TestBuildRevenueSummaryQuery_YearBucket(t *testing.T) {
	q, err := buildRevenueSummaryQuery(RevenuePeriodYear)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "DATE_TRUNC('year'") {
		t.Fatalf("missing year trunc: %s", q)
	}
	if !strings.Contains(q, "Asia/Yangon") {
		t.Fatalf("missing Yangon: %s", q)
	}
}

func TestInclusiveYangonDateRangeToHalfOpen(t *testing.T) {
	loc := revenueSummaryLocation()
	// Yangon is fixed UTC+06:30; expected bounds are asserted as UTC instants.
	yangonMidnightUTC := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, loc).UTC()
	}
	est := time.FixedZone("EST", -5*3600)

	tests := []struct {
		name       string
		from       time.Time
		to         time.Time
		wantStart  time.Time
		wantEnd    time.Time
		wantErr    string
	}{
		{
			name:      "yangon calendar month inclusive to exclusive end",
			from:      time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
			to:        time.Date(2026, 1, 31, 0, 0, 0, 0, loc),
			wantStart: yangonMidnightUTC(2026, 1, 1),
			wantEnd:   yangonMidnightUTC(2026, 2, 1),
		},
		{
			name: "interprets instants by Yangon calendar day across input locations",
			// 2026-01-15 22:00 UTC = 2026-01-16 04:30 Yangon → Jan 16
			from: time.Date(2026, 1, 15, 22, 0, 0, 0, time.UTC),
			// 2026-01-20 10:00 EST = 2026-01-20 15:00 UTC = 2026-01-20 21:30 Yangon → Jan 20
			to:        time.Date(2026, 1, 20, 10, 0, 0, 0, est),
			wantStart: yangonMidnightUTC(2026, 1, 16),
			wantEnd:   yangonMidnightUTC(2026, 1, 21),
		},
		{
			name:      "leap day single day has exclusive end on March 1",
			from:      time.Date(2024, 2, 29, 15, 30, 0, 0, time.UTC),
			to:        time.Date(2024, 2, 29, 8, 0, 0, 0, est),
			wantStart: yangonMidnightUTC(2024, 2, 29),
			wantEnd:   yangonMidnightUTC(2024, 3, 1),
		},
		{
			name:      "year rollover inclusive Dec 31 through Jan 1",
			from:      time.Date(2025, 12, 31, 0, 0, 0, 0, loc),
			to:        time.Date(2026, 1, 1, 23, 59, 59, 0, loc),
			wantStart: yangonMidnightUTC(2025, 12, 31),
			wantEnd:   yangonMidnightUTC(2026, 1, 2),
		},
		{
			name:    "reversed range returns stable error",
			from:    time.Date(2026, 1, 31, 0, 0, 0, 0, loc),
			to:      time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
			wantErr: "to must be on or after from",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := InclusiveYangonDateRangeToHalfOpen(tt.from, tt.to)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !start.UTC().Equal(tt.wantStart) {
				t.Fatalf("start UTC = %v, want %v", start.UTC(), tt.wantStart)
			}
			if !end.UTC().Equal(tt.wantEnd) {
				t.Fatalf("end UTC = %v, want %v", end.UTC(), tt.wantEnd)
			}
			if start.Location().String() != loc.String() {
				t.Fatalf("start location = %s, want %s", start.Location(), loc)
			}
			if end.Location().String() != loc.String() {
				t.Fatalf("end location = %s, want %s", end.Location(), loc)
			}
		})
	}
}

func TestStartOfRevenueYear(t *testing.T) {
	loc := revenueSummaryLocation()
	in := time.Date(2026, 7, 12, 15, 45, 30, 0, loc)
	got := startOfRevenueYear(in)
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("startOfRevenueYear() = %v, want %v", got, want)
	}
	if got.Location().String() != loc.String() {
		t.Fatalf("location = %s, want %s", got.Location(), loc)
	}
}

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
