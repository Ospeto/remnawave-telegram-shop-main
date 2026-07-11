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
				TotalPurchases:       firstPositive(row.PeriodTotalPurchases, row.TotalPurchases),
				ServicePurchases:     firstPositive(row.PeriodServicePurchases, row.NewKeyPurchases+row.ExtensionPurchases),
				UniqueCustomers:      firstPositive(row.PeriodUniqueCustomers, row.UniqueCustomers),
				CashCollected:        firstPositiveFloat(row.PeriodCashCollected, row.CashCollected),
				WalletTopUps:         firstPositiveFloat(row.PeriodWalletTopUps, row.WalletTopUps),
				WalletSpend:          firstPositiveFloat(row.PeriodWalletSpend, row.WalletSpend),
				ServiceRevenue:       firstPositiveFloat(row.PeriodServiceRevenue, firstPositiveFloat(row.ServiceRevenue, row.TotalRevenue)),
				NewKeyPurchases:      firstPositive(row.PeriodNewKeyPurchases, row.NewKeyPurchases),
				ExtensionPurchases:   firstPositive(row.PeriodExtensionPurchases, row.ExtensionPurchases),
				WalletTopUpPurchases: firstPositive(row.PeriodWalletTopUpPurchases, row.WalletTopUpPurchases),
			}
		}

		method := firstNonEmpty(row.PaymentMethod, "unknown")
		methodKey := method + "\x00" + currency
		total := methods[methodKey]
		total.Method = method
		total.Currency = currency
		total.Transactions += row.TotalPurchases
		total.ServiceRevenue += firstPositiveFloat(row.ServiceRevenue, row.TotalRevenue)
		total.CashCollected += row.CashCollected
		total.WalletTopUps += row.WalletTopUps
		total.WalletSpend += row.WalletSpend
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

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
