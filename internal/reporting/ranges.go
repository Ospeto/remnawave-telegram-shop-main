package reporting

import (
	"errors"
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// ErrInvalidReportQuery is returned for out-of-bounds or malformed report window requests.
// Callers should classify with errors.Is.
var ErrInvalidReportQuery = errors.New("invalid report query")

const (
	defaultHistoryDay   = 30
	defaultHistoryWeek  = 12
	defaultHistoryMonth = 12
	defaultHistoryYear  = 5
	maxHistoryDay       = 366
	maxHistoryWeek      = 104
	maxHistoryMonth     = 120
	maxHistoryYear      = 20
	maxCustomInclusive  = 366
)

// ReportWindows holds half-open Yangon windows for selected cards, prior comparison, and trend history.
type ReportWindows struct {
	SelectedStart time.Time
	SelectedEnd   time.Time // half-open
	PriorStart    time.Time
	PriorEnd      time.Time // half-open
	TrendStart    time.Time
	TrendEnd      time.Time // half-open
	HistoryPeriods int
}

func normalizeHistoryPeriods(period database.RevenueSummaryPeriod, historyPeriods int) (int, error) {
	switch period {
	case database.RevenuePeriodDay:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryDay
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryDay {
			return 0, fmt.Errorf("%w: periods must be 1..%d for day", ErrInvalidReportQuery, maxHistoryDay)
		}
		return historyPeriods, nil
	case database.RevenuePeriodWeek:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryWeek
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryWeek {
			return 0, fmt.Errorf("%w: periods must be 1..%d for week", ErrInvalidReportQuery, maxHistoryWeek)
		}
		return historyPeriods, nil
	case database.RevenuePeriodMonth:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryMonth
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryMonth {
			return 0, fmt.Errorf("%w: periods must be 1..%d for month", ErrInvalidReportQuery, maxHistoryMonth)
		}
		return historyPeriods, nil
	case database.RevenuePeriodYear:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryYear
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryYear {
			return 0, fmt.Errorf("%w: periods must be 1..%d for year", ErrInvalidReportQuery, maxHistoryYear)
		}
		return historyPeriods, nil
	case database.RevenuePeriodCustom:
		return 0, nil
	default:
		return 0, fmt.Errorf("%w: unsupported period %s", ErrInvalidReportQuery, period)
	}
}

// ResolveReportWindows returns selected-period, prior-period, and dense-trend windows.
// Selected/prior are single equivalent periods (day/week/month/year) or the full custom range.
// Trend covers historyPeriods buckets ending at the selected period (custom: day buckets over the range).
func ResolveReportWindows(period database.RevenueSummaryPeriod, now time.Time, historyPeriods int, customFrom, customTo *time.Time) (ReportWindows, error) {
	loc := YangonLocation()
	now = now.In(loc)
	var w ReportWindows

	switch period {
	case database.RevenuePeriodDay:
		n, err := normalizeHistoryPeriods(period, historyPeriods)
		if err != nil {
			return ReportWindows{}, err
		}
		w.HistoryPeriods = n
		w.SelectedStart = StartOfDay(now)
		w.SelectedEnd = w.SelectedStart.AddDate(0, 0, 1)
		w.PriorEnd = w.SelectedStart
		w.PriorStart = w.PriorEnd.AddDate(0, 0, -1)
		w.TrendEnd = w.SelectedEnd
		w.TrendStart = w.TrendEnd.AddDate(0, 0, -n)
		return w, nil

	case database.RevenuePeriodWeek:
		n, err := normalizeHistoryPeriods(period, historyPeriods)
		if err != nil {
			return ReportWindows{}, err
		}
		w.HistoryPeriods = n
		w.SelectedStart = StartOfWeek(now)
		w.SelectedEnd = w.SelectedStart.AddDate(0, 0, 7)
		w.PriorEnd = w.SelectedStart
		w.PriorStart = w.PriorEnd.AddDate(0, 0, -7)
		w.TrendEnd = w.SelectedEnd
		w.TrendStart = w.TrendEnd.AddDate(0, 0, -7*n)
		return w, nil

	case database.RevenuePeriodMonth:
		n, err := normalizeHistoryPeriods(period, historyPeriods)
		if err != nil {
			return ReportWindows{}, err
		}
		w.HistoryPeriods = n
		w.SelectedStart = StartOfMonth(now)
		w.SelectedEnd = w.SelectedStart.AddDate(0, 1, 0)
		w.PriorEnd = w.SelectedStart
		w.PriorStart = w.PriorEnd.AddDate(0, -1, 0)
		w.TrendEnd = w.SelectedEnd
		w.TrendStart = w.TrendEnd.AddDate(0, -n, 0)
		return w, nil

	case database.RevenuePeriodYear:
		n, err := normalizeHistoryPeriods(period, historyPeriods)
		if err != nil {
			return ReportWindows{}, err
		}
		w.HistoryPeriods = n
		w.SelectedStart = StartOfYear(now)
		w.SelectedEnd = w.SelectedStart.AddDate(1, 0, 0)
		w.PriorEnd = w.SelectedStart
		w.PriorStart = w.PriorEnd.AddDate(-1, 0, 0)
		w.TrendEnd = w.SelectedEnd
		w.TrendStart = w.TrendEnd.AddDate(-n, 0, 0)
		return w, nil

	case database.RevenuePeriodCustom:
		if customFrom == nil || customTo == nil {
			return ReportWindows{}, fmt.Errorf("%w: custom requires from and to", ErrInvalidReportQuery)
		}
		start, end, err := database.InclusiveYangonDateRangeToHalfOpen(*customFrom, *customTo)
		if err != nil {
			return ReportWindows{}, fmt.Errorf("%w: %v", ErrInvalidReportQuery, err)
		}
		inclusiveDays := int(end.Sub(start).Hours()/24 + 0.5)
		if inclusiveDays < 1 || inclusiveDays > maxCustomInclusive {
			return ReportWindows{}, fmt.Errorf("%w: custom range must be 1..%d days", ErrInvalidReportQuery, maxCustomInclusive)
		}
		w.SelectedStart = start
		w.SelectedEnd = end
		w.PriorEnd = start
		w.PriorStart = start.AddDate(0, 0, -inclusiveDays)
		// Trend for custom is dense day buckets over the selected custom range.
		w.TrendStart = start
		w.TrendEnd = end
		w.HistoryPeriods = inclusiveDays
		return w, nil

	default:
		return ReportWindows{}, fmt.Errorf("%w: unsupported period %s", ErrInvalidReportQuery, period)
	}
}

// ResolveReportWindow returns selected current and prior half-open windows (single period each).
// Prefer ResolveReportWindows when trend bounds are also needed.
// Deprecated signature kept for existing call sites/tests that only need selected+prior.
func ResolveReportWindow(period database.RevenueSummaryPeriod, now time.Time, historyPeriods int, customFrom, customTo *time.Time) (time.Time, time.Time, time.Time, time.Time, error) {
	w, err := ResolveReportWindows(period, now, historyPeriods, customFrom, customTo)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, time.Time{}, err
	}
	return w.SelectedStart, w.SelectedEnd, w.PriorStart, w.PriorEnd, nil
}

// denseBucketStarts materializes every bucket start in [trendStart, trendEnd) for the period grain.
func denseBucketStarts(period database.RevenueSummaryPeriod, trendStart, trendEnd time.Time) []time.Time {
	if !trendEnd.After(trendStart) {
		return nil
	}
	loc := trendStart.Location()
	var out []time.Time
	switch period {
	case database.RevenuePeriodWeek:
		cur := StartOfWeek(trendStart.In(loc))
		if cur.Before(trendStart) {
			// trendStart should already be week-aligned from ResolveReportWindows.
			cur = trendStart.In(loc)
		}
		for cur.Before(trendEnd) {
			out = append(out, cur)
			cur = cur.AddDate(0, 0, 7)
		}
	case database.RevenuePeriodMonth:
		cur := StartOfMonth(trendStart.In(loc))
		for cur.Before(trendEnd) {
			out = append(out, cur)
			cur = cur.AddDate(0, 1, 0)
		}
	case database.RevenuePeriodYear:
		cur := StartOfYear(trendStart.In(loc))
		for cur.Before(trendEnd) {
			out = append(out, cur)
			cur = cur.AddDate(1, 0, 0)
		}
	default: // day and custom (day buckets)
		cur := StartOfDay(trendStart.In(loc))
		for cur.Before(trendEnd) {
			out = append(out, cur)
			cur = cur.AddDate(0, 0, 1)
		}
	}
	return out
}
