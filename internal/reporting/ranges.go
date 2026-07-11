package reporting

import (
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

var ErrInvalidReportQuery = fmt.Errorf("invalid report query")

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

func ResolveReportWindow(period database.RevenueSummaryPeriod, now time.Time, historyPeriods int, customFrom, customTo *time.Time) (time.Time, time.Time, time.Time, time.Time, error) {
	loc := YangonLocation()
	now = now.In(loc)
	switch period {
	case database.RevenuePeriodDay:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryDay
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryDay {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for day", ErrInvalidReportQuery, maxHistoryDay)
		}
		end := StartOfDay(now).AddDate(0, 0, 1)
		start := end.AddDate(0, 0, -historyPeriods)
		priorEnd := start
		priorStart := priorEnd.AddDate(0, 0, -historyPeriods)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodWeek:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryWeek
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryWeek {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for week", ErrInvalidReportQuery, maxHistoryWeek)
		}
		end := StartOfWeek(now).AddDate(0, 0, 7)
		start := end.AddDate(0, 0, -7*historyPeriods)
		priorEnd := start
		priorStart := priorEnd.AddDate(0, 0, -7*historyPeriods)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodMonth:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryMonth
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryMonth {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for month", ErrInvalidReportQuery, maxHistoryMonth)
		}
		end := StartOfMonth(now).AddDate(0, 1, 0)
		start := end.AddDate(0, -historyPeriods, 0)
		priorEnd := start
		priorStart := priorEnd.AddDate(0, -historyPeriods, 0)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodYear:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryYear
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryYear {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for year", ErrInvalidReportQuery, maxHistoryYear)
		}
		end := StartOfYear(now).AddDate(1, 0, 0)
		start := end.AddDate(-historyPeriods, 0, 0)
		priorEnd := start
		priorStart := priorEnd.AddDate(-historyPeriods, 0, 0)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodCustom:
		if customFrom == nil || customTo == nil {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: custom requires from and to", ErrInvalidReportQuery)
		}
		start, end, err := database.InclusiveYangonDateRangeToHalfOpen(*customFrom, *customTo)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: %v", ErrInvalidReportQuery, err)
		}
		inclusiveDays := int(end.Sub(start).Hours()/24 + 0.5)
		if inclusiveDays < 1 || inclusiveDays > maxCustomInclusive {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: custom range must be 1..%d days", ErrInvalidReportQuery, maxCustomInclusive)
		}
		priorEnd := start
		priorStart := priorEnd.AddDate(0, 0, -inclusiveDays)
		return start, end, priorStart, priorEnd, nil
	default:
		return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: unsupported period %s", ErrInvalidReportQuery, period)
	}
}
