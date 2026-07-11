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
	// Selected period = today only; prior = yesterday; history default does not widen selected.
	wantSelectedStart := time.Date(2026, 7, 12, 0, 0, 0, 0, loc)
	wantSelectedEnd := time.Date(2026, 7, 13, 0, 0, 0, 0, loc)
	wantPriorStart := time.Date(2026, 7, 11, 0, 0, 0, 0, loc)
	wantPriorEnd := wantSelectedStart
	if !cs.Equal(wantSelectedStart) || !ce.Equal(wantSelectedEnd) {
		t.Fatalf("selected window got %v..%v want %v..%v", cs, ce, wantSelectedStart, wantSelectedEnd)
	}
	if !ps.Equal(wantPriorStart) || !pe.Equal(wantPriorEnd) {
		t.Fatalf("prior window got %v..%v want %v..%v", ps, pe, wantPriorStart, wantPriorEnd)
	}

	w, err := ResolveReportWindows(database.RevenuePeriodDay, now, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Default historyPeriods=30 for dense trend.
	wantTrendEnd := wantSelectedEnd
	wantTrendStart := wantTrendEnd.AddDate(0, 0, -30)
	if !w.TrendStart.Equal(wantTrendStart) || !w.TrendEnd.Equal(wantTrendEnd) {
		t.Fatalf("trend window got %v..%v want %v..%v", w.TrendStart, w.TrendEnd, wantTrendStart, wantTrendEnd)
	}
	if w.HistoryPeriods != 30 {
		t.Fatalf("historyPeriods=%d want 30", w.HistoryPeriods)
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

func TestResolveReportWindows_WeekSelectedIsCurrentWeek(t *testing.T) {
	loc := YangonLocation()
	// Sunday 2026-07-12 → week Mon 2026-07-06 .. Sun 2026-07-12
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, loc)
	w, err := ResolveReportWindows(database.RevenuePeriodWeek, now, 4, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantSelStart := time.Date(2026, 7, 6, 0, 0, 0, 0, loc)
	wantSelEnd := time.Date(2026, 7, 13, 0, 0, 0, 0, loc)
	if !w.SelectedStart.Equal(wantSelStart) || !w.SelectedEnd.Equal(wantSelEnd) {
		t.Fatalf("selected=%v..%v want %v..%v", w.SelectedStart, w.SelectedEnd, wantSelStart, wantSelEnd)
	}
	wantPriorStart := time.Date(2026, 6, 29, 0, 0, 0, 0, loc)
	if !w.PriorStart.Equal(wantPriorStart) || !w.PriorEnd.Equal(wantSelStart) {
		t.Fatalf("prior=%v..%v", w.PriorStart, w.PriorEnd)
	}
	// 4 weeks of trend ending at selected end.
	if !w.TrendEnd.Equal(wantSelEnd) || !w.TrendStart.Equal(wantSelEnd.AddDate(0, 0, -28)) {
		t.Fatalf("trend=%v..%v", w.TrendStart, w.TrendEnd)
	}
}

func TestDenseBucketStarts_DayIncludesGaps(t *testing.T) {
	loc := YangonLocation()
	start := time.Date(2026, 7, 10, 0, 0, 0, 0, loc)
	end := time.Date(2026, 7, 13, 0, 0, 0, 0, loc)
	got := denseBucketStarts(database.RevenuePeriodDay, start, end)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if got[0].Format("2006-01-02") != "2026-07-10" || got[2].Format("2006-01-02") != "2026-07-12" {
		t.Fatalf("got %v", got)
	}
}
