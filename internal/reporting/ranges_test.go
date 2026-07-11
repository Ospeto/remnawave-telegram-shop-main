package reporting

import (
	"errors"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestResolveReportWindow_DayDefaultsAndPrior(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, loc)
	cs, ce, ps, pe, err := ResolveReportWindow(database.RevenuePeriodDay, now, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// default historyPeriods=30; end = startOfDay(now)+1day; start = end-30d
	wantEnd := time.Date(2026, 7, 13, 0, 0, 0, 0, loc)
	wantStart := wantEnd.AddDate(0, 0, -30) // 2026-06-13
	if !cs.Equal(wantStart) || !ce.Equal(wantEnd) {
		t.Fatalf("current window got %v..%v want %v..%v", cs, ce, wantStart, wantEnd)
	}
	if !pe.Equal(wantStart) {
		t.Fatalf("prior end=%v want %v", pe, wantStart)
	}
	if !ps.Equal(wantStart.AddDate(0, 0, -30)) {
		t.Fatalf("prior start=%v want %v", ps, wantStart.AddDate(0, 0, -30))
	}
}

func TestResolveReportWindow_CustomMax366(t *testing.T) {
	loc := YangonLocation()
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, loc) // > 366 days inclusive
	_, _, _, _, err := ResolveReportWindow(database.RevenuePeriodCustom, time.Now().In(loc), 0, &from, &to)
	if err == nil {
		t.Fatal("expected over-bound custom range error")
	}
	if !errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v want ErrInvalidReportQuery", err)
	}
}

func TestResolveReportWindow_PeriodsOverMax(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, loc)
	_, _, _, _, err := ResolveReportWindow(database.RevenuePeriodDay, now, 9999, nil, nil)
	if !errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v", err)
	}
}
