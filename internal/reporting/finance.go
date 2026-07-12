package reporting

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// ErrMixedCurrency is returned when purchase or refund rows span more than one currency.
// Callers can classify with errors.Is.
var ErrMixedCurrency = errors.New("mixed currencies in finance report input")

// ErrInvalidPeriodStart is returned when a period_start/day value cannot be parsed as YYYY-MM-DD.
var ErrInvalidPeriodStart = errors.New("invalid period start")

type MoneyDelta struct {
	Absolute   float64  `json:"absolute"`
	Percentage *float64 `json:"percentage"` // null when prior base is 0
}

type FinanceMetrics struct {
	GrossServiceRevenue float64 `json:"gross_service_revenue"`
	Refunds             float64 `json:"refunds"`             // positive magnitude
	NetServiceRevenue   float64 `json:"net_service_revenue"` // gross - refunds; UI "Net Income"
	CashCollected       float64 `json:"cash_collected"`
	WalletTopUps        float64 `json:"wallet_topups"`
	WalletSpend         float64 `json:"wallet_spend"`
	SuccessfulOrders    int     `json:"successful_orders"`   // plan purchases only
	UniqueCustomers     int     `json:"unique_customers"`    // distinct across full range
	AverageOrderValue   float64 `json:"average_order_value"` // 0 when orders == 0
	NewSubscriptions    int     `json:"new_subscriptions"`
	Extensions          int     `json:"extensions"`
}

type FinanceDelta struct {
	GrossServiceRevenue MoneyDelta `json:"gross_service_revenue"`
	Refunds             MoneyDelta `json:"refunds"`
	NetServiceRevenue   MoneyDelta `json:"net_service_revenue"`
	CashCollected       MoneyDelta `json:"cash_collected"`
}

type CategoryBreakdown struct {
	Category string  `json:"category"` // new_key | extension | wallet_topup
	Orders   int     `json:"orders"`
	Amount   float64 `json:"amount"`
}

type MethodBreakdown struct {
	Method         string  `json:"method"`
	Transactions   int     `json:"transactions"`
	ServiceRevenue float64 `json:"service_revenue"`
	CashCollected  float64 `json:"cash_collected"`
	WalletTopUps   float64 `json:"wallet_topups"`
	WalletSpend    float64 `json:"wallet_spend"`
}

type FinanceTrendBucket struct {
	PeriodStart string              `json:"period_start"` // YYYY-MM-DD Yangon
	PeriodEnd   string              `json:"period_end"`   // inclusive local date
	InProgress  bool                `json:"in_progress"`
	Metrics     FinanceMetrics      `json:"metrics"`
	Categories  []CategoryBreakdown `json:"categories"` // period-specific only
	Methods     []MethodBreakdown   `json:"methods"`    // period-specific only
}

type FinanceReport struct {
	Period      string          `json:"period"`   // day|week|month|year|custom
	Timezone    string          `json:"timezone"` // Asia/Yangon
	Currency    string          `json:"currency"`
	RangeStart  string          `json:"range_start"` // inclusive YYYY-MM-DD
	RangeEnd    string          `json:"range_end"`   // inclusive YYYY-MM-DD
	GeneratedAt time.Time       `json:"generated_at"`
	InProgress  bool            `json:"in_progress"`
	Current     FinanceMetrics  `json:"current"`
	Prior       *FinanceMetrics `json:"prior"`
	Delta       *FinanceDelta   `json:"delta"`
	// Categories/Methods are range-level totals for the selected window (summary cards / CSV).
	Categories []CategoryBreakdown `json:"categories"`
	Methods    []MethodBreakdown   `json:"methods"`
	// Trend buckets each carry their own Categories/Methods for expandable historical rows.
	Trend []FinanceTrendBucket `json:"trend"` // ascending
}

type BuildFinanceReportInput struct {
	Period               database.RevenueSummaryPeriod
	Now                  time.Time
	HistoryPeriods       int
	CustomFrom           *time.Time
	CustomTo             *time.Time
	PurchaseRows         []database.RevenueSummaryRow
	RefundRows           []database.RefundPeriodRow
	SettlementRows       []database.SettlementPeriodRow
	PriorPurchaseRows    []database.RevenueSummaryRow
	PriorRefundRows      []database.RefundPeriodRow
	PriorSettlementRows  []database.SettlementPeriodRow
	RangeUniqueCustomers int
	PriorUniqueCustomers int
	// Window fields filled by FinanceService before BuildFinanceReport:
	// CurrentStart/End = selected period for headline cards (half-open).
	CurrentStart time.Time
	CurrentEnd   time.Time // half-open
	PriorStart   time.Time
	PriorEnd     time.Time // half-open
	// TrendStart/End = full history window for dense trend buckets (half-open).
	// When zero, trend falls back to CurrentStart/CurrentEnd (single selected period).
	TrendStart time.Time
	TrendEnd   time.Time // half-open
}

func moneyDelta(current, prior float64) MoneyDelta {
	d := MoneyDelta{Absolute: RoundMoney(current - prior)}
	if prior == 0 {
		d.Percentage = nil
		return d
	}
	p := RoundMoney(((current - prior) / prior) * 100)
	d.Percentage = &p
	return d
}

func finalizeMetrics(m FinanceMetrics, uniqueCustomers int) FinanceMetrics {
	m.GrossServiceRevenue = RoundMoney(m.GrossServiceRevenue)
	m.Refunds = RoundMoney(m.Refunds)
	m.NetServiceRevenue = RoundMoney(m.GrossServiceRevenue - m.Refunds)
	m.CashCollected = RoundMoney(m.CashCollected)
	m.WalletTopUps = RoundMoney(m.WalletTopUps)
	m.WalletSpend = RoundMoney(m.WalletSpend)
	m.UniqueCustomers = uniqueCustomers
	if m.SuccessfulOrders > 0 {
		m.AverageOrderValue = RoundMoney(m.GrossServiceRevenue / float64(m.SuccessfulOrders))
	} else {
		m.AverageOrderValue = 0
	}
	return m
}

type bucketKey struct {
	start    string
	currency string
}

func normalizeCurrency(currency string) string {
	return firstNonEmpty(currency, "MMK")
}

// resolveReportCurrency requires a single currency across all purchase, refund, and settlement rows.
// Empty currency values normalize to MMK. Mixed currencies return ErrMixedCurrency.
func resolveReportCurrency(
	purchaseRows []database.RevenueSummaryRow,
	refundRows []database.RefundPeriodRow,
	settlementRows []database.SettlementPeriodRow,
	priorPurchaseRows []database.RevenueSummaryRow,
	priorRefundRows []database.RefundPeriodRow,
	priorSettlementRows []database.SettlementPeriodRow,
) (string, error) {
	seen := map[string]struct{}{}
	addPurchase := func(rows []database.RevenueSummaryRow) {
		for _, row := range rows {
			seen[normalizeCurrency(row.Currency)] = struct{}{}
		}
	}
	addRefund := func(rows []database.RefundPeriodRow) {
		for _, row := range rows {
			seen[normalizeCurrency(row.Currency)] = struct{}{}
		}
	}
	addSettlement := func(rows []database.SettlementPeriodRow) {
		for _, row := range rows {
			seen[normalizeCurrency(row.Currency)] = struct{}{}
		}
	}
	addPurchase(purchaseRows)
	addPurchase(priorPurchaseRows)
	addRefund(refundRows)
	addRefund(priorRefundRows)
	addSettlement(settlementRows)
	addSettlement(priorSettlementRows)

	if len(seen) == 0 {
		return "MMK", nil
	}
	if len(seen) > 1 {
		return "", ErrMixedCurrency
	}
	for c := range seen {
		return c, nil
	}
	return "MMK", nil
}

func aggregatePurchaseBuckets(rows []database.RevenueSummaryRow) map[bucketKey]FinanceMetrics {
	seenPeriod := map[bucketKey]bool{}
	out := map[bucketKey]FinanceMetrics{}
	for _, row := range rows {
		currency := normalizeCurrency(row.Currency)
		start := firstNonEmpty(row.PeriodStart, row.Day)
		key := bucketKey{start: start, currency: currency}
		m := out[key]
		if !seenPeriod[key] {
			seenPeriod[key] = true
			m.GrossServiceRevenue += row.PeriodServiceRevenue
			m.CashCollected += row.PeriodCashCollected
			m.WalletTopUps += row.PeriodWalletTopUps
			m.WalletSpend += row.PeriodWalletSpend
			m.SuccessfulOrders += row.PeriodServicePurchases
			m.NewSubscriptions += row.PeriodNewKeyPurchases
			m.Extensions += row.PeriodExtensionPurchases
		}
		out[key] = m
	}
	return out
}

func applyRefunds(buckets map[bucketKey]FinanceMetrics, refunds []database.RefundPeriodRow) {
	for _, rr := range refunds {
		key := bucketKey{start: rr.PeriodStart, currency: normalizeCurrency(rr.Currency)}
		m := buckets[key]
		m.Refunds += rr.RefundTotal
		buckets[key] = m
	}
}

// applySettlements adds AR settlement totals into CashCollected only.
// Does not touch GrossServiceRevenue, Refunds, WalletTopUps, or order counts.
func applySettlements(buckets map[bucketKey]FinanceMetrics, rows []database.SettlementPeriodRow) {
	for _, sr := range rows {
		key := bucketKey{start: sr.PeriodStart, currency: normalizeCurrency(sr.Currency)}
		m := buckets[key]
		m.CashCollected += sr.SettlementTotal
		buckets[key] = m
	}
}

func sumMetrics(buckets map[bucketKey]FinanceMetrics) FinanceMetrics {
	var total FinanceMetrics
	for _, m := range buckets {
		total.GrossServiceRevenue += m.GrossServiceRevenue
		total.Refunds += m.Refunds
		total.CashCollected += m.CashCollected
		total.WalletTopUps += m.WalletTopUps
		total.WalletSpend += m.WalletSpend
		total.SuccessfulOrders += m.SuccessfulOrders
		total.NewSubscriptions += m.NewSubscriptions
		total.Extensions += m.Extensions
	}
	return total
}

func bucketInclusiveEnd(period database.RevenueSummaryPeriod, start time.Time) time.Time {
	switch period {
	case database.RevenuePeriodWeek:
		return start.AddDate(0, 0, 6)
	case database.RevenuePeriodMonth:
		return start.AddDate(0, 1, -1)
	case database.RevenuePeriodYear:
		return start.AddDate(1, 0, -1)
	default: // day, custom day buckets
		return start
	}
}

func parsePeriodStart(startStr string, loc *time.Location) (time.Time, error) {
	startDay, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q: %v", ErrInvalidPeriodStart, startStr, err)
	}
	return startDay, nil
}

// validatePeriodStarts rejects malformed period_start/day values across all input row
// collections before aggregation so bad prior rows cannot affect metrics or deltas.
func validatePeriodStarts(
	loc *time.Location,
	purchaseRows []database.RevenueSummaryRow,
	refundRows []database.RefundPeriodRow,
	settlementRows []database.SettlementPeriodRow,
	priorPurchaseRows []database.RevenueSummaryRow,
	priorRefundRows []database.RefundPeriodRow,
	priorSettlementRows []database.SettlementPeriodRow,
) error {
	checkPurchase := func(rows []database.RevenueSummaryRow) error {
		for _, row := range rows {
			start := firstNonEmpty(row.PeriodStart, row.Day)
			if _, err := parsePeriodStart(start, loc); err != nil {
				return err
			}
		}
		return nil
	}
	checkRefund := func(rows []database.RefundPeriodRow) error {
		for _, row := range rows {
			if _, err := parsePeriodStart(row.PeriodStart, loc); err != nil {
				return err
			}
		}
		return nil
	}
	checkSettlement := func(rows []database.SettlementPeriodRow) error {
		for _, row := range rows {
			if _, err := parsePeriodStart(row.PeriodStart, loc); err != nil {
				return err
			}
		}
		return nil
	}
	if err := checkPurchase(purchaseRows); err != nil {
		return err
	}
	if err := checkRefund(refundRows); err != nil {
		return err
	}
	if err := checkSettlement(settlementRows); err != nil {
		return err
	}
	if err := checkPurchase(priorPurchaseRows); err != nil {
		return err
	}
	if err := checkRefund(priorRefundRows); err != nil {
		return err
	}
	return checkSettlement(priorSettlementRows)
}

func filterRowsByPeriodStart(rows []database.RevenueSummaryRow, startKey string) []database.RevenueSummaryRow {
	if startKey == "" {
		return nil
	}
	out := make([]database.RevenueSummaryRow, 0)
	for _, row := range rows {
		if firstNonEmpty(row.PeriodStart, row.Day) == startKey {
			out = append(out, row)
		}
	}
	return out
}

func metricsForBucket(buckets map[bucketKey]FinanceMetrics, startKey, currency string, uniqueCustomers int) FinanceMetrics {
	return finalizeMetrics(buckets[bucketKey{start: startKey, currency: currency}], uniqueCustomers)
}

func BuildFinanceReport(in BuildFinanceReportInput) (FinanceReport, error) {
	if in.CurrentStart.IsZero() || in.CurrentEnd.IsZero() || !in.CurrentEnd.After(in.CurrentStart) {
		return FinanceReport{}, fmt.Errorf("current window is required")
	}
	currency, err := resolveReportCurrency(
		in.PurchaseRows, in.RefundRows, in.SettlementRows,
		in.PriorPurchaseRows, in.PriorRefundRows, in.PriorSettlementRows,
	)
	if err != nil {
		return FinanceReport{}, err
	}

	loc := YangonLocation()
	if err := validatePeriodStarts(
		loc,
		in.PurchaseRows, in.RefundRows, in.SettlementRows,
		in.PriorPurchaseRows, in.PriorRefundRows, in.PriorSettlementRows,
	); err != nil {
		return FinanceReport{}, err
	}

	now := in.Now.In(loc)
	period := in.Period
	if period == "" {
		period = database.RevenuePeriodDay
	}

	// Grain used for bucket keys / dense trend (custom uses day buckets).
	bucketPeriod := period
	if period == database.RevenuePeriodCustom {
		bucketPeriod = database.RevenuePeriodDay
	}

	allBuckets := aggregatePurchaseBuckets(in.PurchaseRows)
	applyRefunds(allBuckets, in.RefundRows)
	applySettlements(allBuckets, in.SettlementRows)
	priorBuckets := aggregatePurchaseBuckets(in.PriorPurchaseRows)
	applyRefunds(priorBuckets, in.PriorRefundRows)
	applySettlements(priorBuckets, in.PriorSettlementRows)

	// Headline Current = selected period only (not full history window sum).
	// Custom: selected range is the whole custom window → sum all buckets in PurchaseRows
	// that fall in the selected range (which is the custom fetch for trend=selected).
	selectedKey := in.CurrentStart.In(loc).Format("2006-01-02")
	var current FinanceMetrics
	var selectedPurchaseRows []database.RevenueSummaryRow
	if period == database.RevenuePeriodCustom {
		// Entire custom range is the selected period.
		current = finalizeMetrics(sumMetrics(allBuckets), in.RangeUniqueCustomers)
		selectedPurchaseRows = in.PurchaseRows
	} else {
		current = metricsForBucket(allBuckets, selectedKey, currency, in.RangeUniqueCustomers)
		selectedPurchaseRows = filterRowsByPeriodStart(in.PurchaseRows, selectedKey)
	}

	// Prior is always a single equivalent period (or equal-length custom prior range).
	prior := finalizeMetrics(sumMetrics(priorBuckets), in.PriorUniqueCustomers)

	// Categories/Methods for selected period only (summary cards / CSV).
	categories := buildCategoryBreakdown(selectedPurchaseRows)
	methods := buildMethodBreakdown(selectedPurchaseRows)

	// Per-period breakdown maps for truthful expandable trend rows.
	catsByPeriod := map[string][]database.RevenueSummaryRow{}
	for _, row := range in.PurchaseRows {
		start := firstNonEmpty(row.PeriodStart, row.Day)
		catsByPeriod[start] = append(catsByPeriod[start], row)
	}

	// Dense trend over history window (zeros for quiet buckets), chronological ascending.
	trendStart := in.TrendStart
	trendEnd := in.TrendEnd
	if trendStart.IsZero() || trendEnd.IsZero() || !trendEnd.After(trendStart) {
		// Fallback: single selected period when trend window not provided.
		trendStart = in.CurrentStart
		trendEnd = in.CurrentEnd
	}

	bucketStarts := denseBucketStarts(bucketPeriod, trendStart.In(loc), trendEnd.In(loc))
	trend := make([]FinanceTrendBucket, 0, len(bucketStarts))
	for _, startDay := range bucketStarts {
		startStr := startDay.Format("2006-01-02")
		m := allBuckets[bucketKey{start: startStr, currency: currency}]
		endDay := bucketInclusiveEnd(bucketPeriod, startDay)
		bucketEndExclusive := endDay.AddDate(0, 0, 1)
		// For week/month/year, bucketInclusiveEnd returns inclusive last day; exclusive end is next day after that.
		// For week: start + 6 days inclusive → exclusive = start+7 which equals endDay.AddDate(0,0,1) only if endDay is last day.
		// bucketInclusiveEnd for week returns start+6d; +1d = start+7d ✓
		inProgress := !now.Before(startDay) && now.Before(bucketEndExclusive)
		periodRows := catsByPeriod[startStr]
		trend = append(trend, FinanceTrendBucket{
			PeriodStart: startStr,
			PeriodEnd:   endDay.Format("2006-01-02"),
			InProgress:  inProgress,
			Metrics:     finalizeMetrics(m, 0),
			Categories:  buildCategoryBreakdown(periodRows),
			Methods:     buildMethodBreakdown(periodRows),
		})
	}

	rangeStart := in.CurrentStart.In(loc).Format("2006-01-02")
	rangeEnd := in.CurrentEnd.In(loc).Add(-time.Nanosecond).Format("2006-01-02")
	inProgress := !now.Before(in.CurrentStart) && now.Before(in.CurrentEnd)

	var priorPtr *FinanceMetrics
	var deltaPtr *FinanceDelta
	if !in.PriorStart.IsZero() && in.PriorEnd.After(in.PriorStart) {
		p := prior
		priorPtr = &p
		deltaPtr = &FinanceDelta{
			GrossServiceRevenue: moneyDelta(current.GrossServiceRevenue, prior.GrossServiceRevenue),
			Refunds:             moneyDelta(current.Refunds, prior.Refunds),
			NetServiceRevenue:   moneyDelta(current.NetServiceRevenue, prior.NetServiceRevenue),
			CashCollected:       moneyDelta(current.CashCollected, prior.CashCollected),
		}
	}

	return FinanceReport{
		Period:      string(period),
		Timezone:    "Asia/Yangon",
		Currency:    currency,
		RangeStart:  rangeStart,
		RangeEnd:    rangeEnd,
		GeneratedAt: now,
		InProgress:  inProgress,
		Current:     current,
		Prior:       priorPtr,
		Delta:       deltaPtr,
		Categories:  categories,
		Methods:     methods,
		Trend:       trend,
	}, nil
}

func buildCategoryBreakdown(rows []database.RevenueSummaryRow) []CategoryBreakdown {
	catMap := map[string]CategoryBreakdown{}
	for _, row := range rows {
		cat := firstNonEmpty(row.RevenueCategory, "unknown")
		c := catMap[cat]
		c.Category = cat
		c.Orders += row.TotalPurchases
		if row.InvoiceType == string(database.InvoiceTypeWalletTopUp) || cat == "wallet_topup" {
			c.Amount += row.WalletTopUps
		} else {
			c.Amount += row.ServiceRevenue
		}
		catMap[cat] = c
	}
	out := make([]CategoryBreakdown, 0, len(catMap))
	for _, c := range catMap {
		c.Amount = RoundMoney(c.Amount)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

func buildMethodBreakdown(rows []database.RevenueSummaryRow) []MethodBreakdown {
	methodMap := map[string]MethodBreakdown{}
	for _, row := range rows {
		method := firstNonEmpty(row.PaymentMethod, "unknown")
		m := methodMap[method]
		m.Method = method
		m.Transactions += row.TotalPurchases
		m.ServiceRevenue += row.ServiceRevenue
		m.CashCollected += row.CashCollected
		m.WalletTopUps += row.WalletTopUps
		m.WalletSpend += row.WalletSpend
		methodMap[method] = m
	}
	out := make([]MethodBreakdown, 0, len(methodMap))
	for _, m := range methodMap {
		m.ServiceRevenue = RoundMoney(m.ServiceRevenue)
		m.CashCollected = RoundMoney(m.CashCollected)
		m.WalletTopUps = RoundMoney(m.WalletTopUps)
		m.WalletSpend = RoundMoney(m.WalletSpend)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ServiceRevenue == out[j].ServiceRevenue {
			return out[i].Method < out[j].Method
		}
		return out[i].ServiceRevenue > out[j].ServiceRevenue
	})
	return out
}
