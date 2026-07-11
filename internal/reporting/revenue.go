package reporting

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

const revenueReportTimezone = "Asia/Yangon"

type RevenuePeriodTotals struct {
	PeriodStart          string
	Currency             string
	TotalPurchases       int
	ServicePurchases     int
	UniqueCustomers      int
	CashCollected        float64
	WalletTopUps         float64
	WalletSpend          float64
	ServiceRevenue       float64
	NewKeyPurchases      int
	ExtensionPurchases   int
	WalletTopUpPurchases int
}

type RevenueMethodTotals struct {
	Method         string
	Currency       string
	Transactions   int
	ServiceRevenue float64
	CashCollected  float64
	WalletTopUps   float64
	WalletSpend    float64
}

func YangonLocation() *time.Location {
	location, err := time.LoadLocation(revenueReportTimezone)
	if err != nil {
		return time.FixedZone("MMT", 6*3600+30*60)
	}
	return location
}

func StartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func StartOfWeek(t time.Time) time.Time {
	day := StartOfDay(t)
	daysSinceMonday := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -daysSinceMonday)
}

func StartOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func PreviousDayRange(now time.Time) (time.Time, time.Time) {
	end := StartOfDay(now)
	return end.AddDate(0, 0, -1), end
}

func PreviousWeekRange(now time.Time) (time.Time, time.Time) {
	end := StartOfWeek(now)
	return end.AddDate(0, 0, -7), end
}

func PreviousMonthRange(now time.Time) (time.Time, time.Time) {
	end := StartOfMonth(now)
	return end.AddDate(0, -1, 0), end
}

func StartOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}

func PreviousYearRange(now time.Time) (time.Time, time.Time) {
	end := StartOfYear(now)
	return end.AddDate(-1, 0, 0), end
}

func FormatDateRange(start, end time.Time) string {
	lastIncludedDay := end.Add(-time.Nanosecond).In(start.Location())
	if start.Format("2006-01-02") == lastIncludedDay.Format("2006-01-02") {
		return start.Format("2006-01-02")
	}
	return fmt.Sprintf("%s to %s", start.Format("2006-01-02"), lastIncludedDay.Format("2006-01-02"))
}

func SummarizeRevenuePeriod(rows []database.RevenueSummaryRow) (RevenuePeriodTotals, []RevenueMethodTotals) {
	if len(rows) == 0 {
		return RevenuePeriodTotals{Currency: "MMK"}, nil
	}

	type periodKey struct {
		start    string
		currency string
	}
	periods := make(map[periodKey]RevenuePeriodTotals)
	methods := make(map[string]RevenueMethodTotals)

	for _, row := range rows {
		currency := firstNonEmpty(row.Currency, "MMK")
		start := firstNonEmpty(row.PeriodStart, row.Day)
		key := periodKey{start: start, currency: currency}
		if _, ok := periods[key]; !ok {
			periods[key] = RevenuePeriodTotals{
				PeriodStart:          start,
				Currency:             currency,
				TotalPurchases:       row.PeriodTotalPurchases,
				ServicePurchases:     row.PeriodServicePurchases,
				UniqueCustomers:      row.PeriodUniqueCustomers, // per-bucket only; not range-level
				CashCollected:        row.PeriodCashCollected,
				WalletTopUps:         row.PeriodWalletTopUps,
				WalletSpend:          row.PeriodWalletSpend,
				ServiceRevenue:       row.PeriodServiceRevenue,
				NewKeyPurchases:      row.PeriodNewKeyPurchases,
				ExtensionPurchases:   row.PeriodExtensionPurchases,
				WalletTopUpPurchases: row.PeriodWalletTopUpPurchases,
			}
		}

		method := firstNonEmpty(row.PaymentMethod, "unknown")
		methodKey := method + "\x00" + currency
		total := methods[methodKey]
		total.Method = method
		total.Currency = currency
		total.ServiceRevenue += row.ServiceRevenue
		total.CashCollected += row.CashCollected
		total.WalletTopUps += row.WalletTopUps
		total.WalletSpend += row.WalletSpend
		total.Transactions += row.TotalPurchases
		methods[methodKey] = total
	}

	var totals RevenuePeriodTotals
	for _, period := range periods {
		totals.Currency = firstNonEmpty(totals.Currency, period.Currency)
		totals.TotalPurchases += period.TotalPurchases
		totals.ServicePurchases += period.ServicePurchases
		totals.UniqueCustomers += period.UniqueCustomers
		totals.CashCollected += period.CashCollected
		totals.WalletTopUps += period.WalletTopUps
		totals.WalletSpend += period.WalletSpend
		totals.ServiceRevenue += period.ServiceRevenue
		totals.NewKeyPurchases += period.NewKeyPurchases
		totals.ExtensionPurchases += period.ExtensionPurchases
		totals.WalletTopUpPurchases += period.WalletTopUpPurchases
	}
	if totals.Currency == "" {
		totals.Currency = "MMK"
	}

	methodList := make([]RevenueMethodTotals, 0, len(methods))
	for _, method := range methods {
		methodList = append(methodList, method)
	}
	sort.Slice(methodList, func(i, j int) bool {
		if methodList[i].ServiceRevenue == methodList[j].ServiceRevenue {
			return methodList[i].CashCollected > methodList[j].CashCollected
		}
		return methodList[i].ServiceRevenue > methodList[j].ServiceRevenue
	})

	return totals, methodList
}

func FormatTelegramPeriodRevenueReport(title, periodLabel string, rows []database.RevenueSummaryRow) string {
	if len(rows) == 0 {
		return fmt.Sprintf("📊 <b>%s</b>\n\n<b>%s</b>\nNo paid activity.", html.EscapeString(title), html.EscapeString(periodLabel))
	}

	totals, methods := SummarizeRevenuePeriod(rows)
	rawCurrency := firstNonEmpty(totals.Currency, "MMK")
	currency := html.EscapeString(rawCurrency)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 <b>%s</b>\n\n", html.EscapeString(title)))
	sb.WriteString(fmt.Sprintf("<b>%s</b>\n", html.EscapeString(periodLabel)))
	sb.WriteString(fmt.Sprintf("Service revenue: <b>%s %s</b> (%d plan txns, %d users)\n", FormatNumber(totals.ServiceRevenue), currency, totals.ServicePurchases, totals.UniqueCustomers))
	sb.WriteString(fmt.Sprintf("Cash collected: <b>%s %s</b>\n", FormatNumber(totals.CashCollected), currency))
	if totals.WalletTopUps > 0 || totals.WalletSpend > 0 {
		sb.WriteString(fmt.Sprintf("Wallet: %s %s top-ups, %s %s wallet spend\n", FormatNumber(totals.WalletTopUps), currency, FormatNumber(totals.WalletSpend), currency))
	}
	sb.WriteString(fmt.Sprintf("Mix: %d new keys, %d extensions, %d top-ups\n", totals.NewKeyPurchases, totals.ExtensionPurchases, totals.WalletTopUpPurchases))

	if len(methods) > 0 {
		sb.WriteString("\n<b>By method</b>\n")
		for _, method := range methods {
			methodCurrency := html.EscapeString(firstNonEmpty(method.Currency, rawCurrency))
			sb.WriteString(fmt.Sprintf("  %s: service %s %s, cash %s %s (%d txns)\n",
				html.EscapeString(method.Method),
				FormatNumber(method.ServiceRevenue),
				methodCurrency,
				FormatNumber(method.CashCollected),
				methodCurrency,
				method.Transactions,
			))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func FormatTelegramFinanceReport(title string, report FinanceReport) string {
	currency := html.EscapeString(firstNonEmpty(report.Currency, "MMK"))
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 <b>%s</b>\n\n", html.EscapeString(title)))
	rangeLabel := report.RangeStart
	if report.RangeStart != report.RangeEnd {
		rangeLabel = fmt.Sprintf("%s to %s", report.RangeStart, report.RangeEnd)
	}
	if report.InProgress {
		rangeLabel += " (In progress)"
	}
	sb.WriteString(fmt.Sprintf("<b>%s</b>\n", html.EscapeString(rangeLabel)))
	sb.WriteString(fmt.Sprintf("Net Income: <b>%s %s</b>\n", FormatNumber(report.Current.NetServiceRevenue), currency))
	sb.WriteString(fmt.Sprintf("Gross: <b>%s %s</b>\n", FormatNumber(report.Current.GrossServiceRevenue), currency))
	sb.WriteString(fmt.Sprintf("Refunds: <b>%s %s</b>\n", FormatNumber(report.Current.Refunds), currency))
	sb.WriteString(fmt.Sprintf("Cash collected: <b>%s %s</b>\n", FormatNumber(report.Current.CashCollected), currency))
	if report.Current.WalletTopUps > 0 || report.Current.WalletSpend > 0 {
		sb.WriteString(fmt.Sprintf("Wallet: %s %s top-ups, %s %s spend\n",
			FormatNumber(report.Current.WalletTopUps), currency,
			FormatNumber(report.Current.WalletSpend), currency))
	}
	sb.WriteString(fmt.Sprintf("Orders: %d plan txns, %d users\n", report.Current.SuccessfulOrders, report.Current.UniqueCustomers))
	sb.WriteString(fmt.Sprintf("Mix: %d new keys, %d extensions\n", report.Current.NewSubscriptions, report.Current.Extensions))
	if len(report.Methods) > 0 {
		sb.WriteString("\n<b>By method</b>\n")
		for _, method := range report.Methods {
			sb.WriteString(fmt.Sprintf("  %s: service %s %s, cash %s %s (%d txns)\n",
				html.EscapeString(method.Method),
				FormatNumber(method.ServiceRevenue), currency,
				FormatNumber(method.CashCollected), currency,
				method.Transactions,
			))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func FormatRevenueCommandFromReport(report FinanceReport, today string) string {
	var sb strings.Builder
	sb.WriteString("📊 <b>Revenue Summary</b>\n\n")
	sb.WriteString(fmt.Sprintf("<b>Selected range</b> %s", report.RangeStart))
	if report.RangeStart != report.RangeEnd {
		sb.WriteString(" to " + report.RangeEnd)
	}
	if report.InProgress {
		sb.WriteString(" (In progress)")
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Net Income: <b>%s %s</b>\n", FormatNumber(report.Current.NetServiceRevenue), html.EscapeString(report.Currency)))
	sb.WriteString(fmt.Sprintf("Gross: %s, Refunds: %s, Cash: %s\n",
		FormatNumber(report.Current.GrossServiceRevenue),
		FormatNumber(report.Current.Refunds),
		FormatNumber(report.Current.CashCollected),
	))
	sb.WriteString(fmt.Sprintf("Orders: %d, Users: %d\n", report.Current.SuccessfulOrders, report.Current.UniqueCustomers))
	if len(report.Trend) > 0 {
		sb.WriteString("\n<b>Trend</b>\n")
		// show last up to 7 buckets newest last in list already ascending — print reverse for recency
		start := 0
		if len(report.Trend) > 7 {
			start = len(report.Trend) - 7
		}
		for _, b := range report.Trend[start:] {
			label := b.PeriodStart
			if b.PeriodStart == today {
				label += " (today)"
			}
			if b.InProgress {
				label += " *"
			}
			sb.WriteString(fmt.Sprintf("  %s: net %s, gross %s, refunds %s\n",
				html.EscapeString(label),
				FormatNumber(b.Metrics.NetServiceRevenue),
				FormatNumber(b.Metrics.GrossServiceRevenue),
				FormatNumber(b.Metrics.Refunds),
			))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func FormatRevenueCommand(rows []database.RevenueSummaryRow, today string) string {
	if len(rows) == 0 {
		return "📊 No revenue data for the selected period."
	}

	byDay := make(map[string][]database.RevenueSummaryRow)
	var days []string
	for _, row := range rows {
		day := firstNonEmpty(row.PeriodStart, row.Day)
		if day == "" {
			continue
		}
		if _, ok := byDay[day]; !ok {
			days = append(days, day)
		}
		byDay[day] = append(byDay[day], row)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))

	var sb strings.Builder
	sb.WriteString("📊 <b>Revenue Summary</b>\n\n")

	sb.WriteString("<b>Today</b>\n")
	if todayRows := byDay[today]; len(todayRows) > 0 {
		writeCompactPeriodSummary(&sb, todayRows)
	} else {
		sb.WriteString("  No paid activity yet today\n")
	}

	sb.WriteString("\n<b>Last 7 Days</b>\n")
	for _, day := range days {
		totals, _ := SummarizeRevenuePeriod(byDay[day])
		label := day
		if day == today {
			label += " (today)"
		}
		sb.WriteString(fmt.Sprintf("  %s: %s %s service, %s %s cash (%d plan txns, %d users)\n",
			html.EscapeString(label),
			FormatNumber(totals.ServiceRevenue),
			html.EscapeString(totals.Currency),
			FormatNumber(totals.CashCollected),
			html.EscapeString(totals.Currency),
			totals.ServicePurchases,
			totals.UniqueCustomers,
		))
	}

	return strings.TrimRight(sb.String(), "\n")
}

func writeCompactPeriodSummary(sb *strings.Builder, rows []database.RevenueSummaryRow) {
	totals, methods := SummarizeRevenuePeriod(rows)
	currency := html.EscapeString(totals.Currency)
	sb.WriteString(fmt.Sprintf("  Service revenue: <b>%s %s</b> (%d plan txns, %d users)\n", FormatNumber(totals.ServiceRevenue), currency, totals.ServicePurchases, totals.UniqueCustomers))
	sb.WriteString(fmt.Sprintf("  Cash collected: <b>%s %s</b>\n", FormatNumber(totals.CashCollected), currency))
	if totals.WalletTopUps > 0 || totals.WalletSpend > 0 {
		sb.WriteString(fmt.Sprintf("  Wallet: %s %s top-ups, %s %s spend\n", FormatNumber(totals.WalletTopUps), currency, FormatNumber(totals.WalletSpend), currency))
	}
	for _, method := range methods {
		if method.ServiceRevenue == 0 && method.CashCollected == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s: service %s, cash %s (%d txns)\n",
			html.EscapeString(method.Method),
			FormatNumber(method.ServiceRevenue),
			FormatNumber(method.CashCollected),
			method.Transactions,
		))
	}
}

func FormatNumber(n float64) string {
	if n == float64(int64(n)) {
		return addCommas(fmt.Sprintf("%d", int64(n)))
	}
	parts := strings.SplitN(fmt.Sprintf("%.2f", n), ".", 2)
	return addCommas(parts[0]) + "." + parts[1]
}

func addCommas(s string) string {
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign = "-"
		s = strings.TrimPrefix(s, "-")
	}
	n := len(s)
	if n <= 3 {
		return sign + s
	}
	var b strings.Builder
	rem := n % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if n > rem {
			b.WriteString(",")
		}
	}
	for i := rem; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteString(",")
		}
	}
	return sign + b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
