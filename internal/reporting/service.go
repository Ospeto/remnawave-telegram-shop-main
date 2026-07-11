package reporting

import (
	"context"
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

type purchaseRevenueReader interface {
	GetRevenueSummaryRange(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod) ([]database.RevenueSummaryRow, error)
	CountDistinctServiceCustomers(ctx context.Context, start, end time.Time) (int, error)
}

type refundPeriodReader interface {
	SumRefundsByPeriod(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod, adminTelegramID int64) ([]database.RefundPeriodRow, error)
}

type FinanceService struct {
	purchases purchaseRevenueReader
	refunds   refundPeriodReader
	adminID   func() int64
	now       func() time.Time
}

func NewFinanceService(purchases purchaseRevenueReader, refunds refundPeriodReader) *FinanceService {
	return &FinanceService{
		purchases: purchases,
		refunds:   refunds,
		adminID:   config.GetAdminTelegramId,
		now:       time.Now,
	}
}

type ReportQuery struct {
	Period         database.RevenueSummaryPeriod
	HistoryPeriods int
	CustomFrom     *time.Time
	CustomTo       *time.Time
	Now            time.Time
}

func (s *FinanceService) GetReport(ctx context.Context, q ReportQuery) (FinanceReport, error) {
	now := q.Now
	if now.IsZero() {
		now = s.now()
	}
	period := q.Period
	if period == "" {
		period = database.RevenuePeriodDay
	}
	// Reject unknown periods early (Normalize already used by API; still guard).
	switch period {
	case database.RevenuePeriodDay, database.RevenuePeriodWeek, database.RevenuePeriodMonth, database.RevenuePeriodYear, database.RevenuePeriodCustom:
	default:
		return FinanceReport{}, fmt.Errorf("%w: unsupported period %s", ErrInvalidReportQuery, period)
	}

	bucketPeriod := period
	if period == database.RevenuePeriodCustom {
		bucketPeriod = database.RevenuePeriodDay
	}

	windows, err := ResolveReportWindows(period, now, q.HistoryPeriods, q.CustomFrom, q.CustomTo)
	if err != nil {
		return FinanceReport{}, err
	}

	// Fetch purchase/refund rows for the full trend history window (dense trend needs all buckets).
	// Prior is a separate single-period window immediately before selected.
	purchaseRows, err := s.purchases.GetRevenueSummaryRange(ctx, windows.TrendStart, windows.TrendEnd, bucketPeriod)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load purchase revenue: %w", err)
	}
	priorPurchaseRows, err := s.purchases.GetRevenueSummaryRange(ctx, windows.PriorStart, windows.PriorEnd, bucketPeriod)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load prior purchase revenue: %w", err)
	}
	adminID := s.adminID()
	refundRows, err := s.refunds.SumRefundsByPeriod(ctx, windows.TrendStart, windows.TrendEnd, bucketPeriod, adminID)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load refunds: %w", err)
	}
	priorRefundRows, err := s.refunds.SumRefundsByPeriod(ctx, windows.PriorStart, windows.PriorEnd, bucketPeriod, adminID)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load prior refunds: %w", err)
	}
	// Unique customers for selected period only (headline cards), not full history window.
	uniq, err := s.purchases.CountDistinctServiceCustomers(ctx, windows.SelectedStart, windows.SelectedEnd)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("count customers: %w", err)
	}
	priorUniq, err := s.purchases.CountDistinctServiceCustomers(ctx, windows.PriorStart, windows.PriorEnd)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("count prior customers: %w", err)
	}

	return BuildFinanceReport(BuildFinanceReportInput{
		Period:               period,
		Now:                  now.In(YangonLocation()),
		HistoryPeriods:       windows.HistoryPeriods,
		CustomFrom:           q.CustomFrom,
		CustomTo:             q.CustomTo,
		PurchaseRows:         purchaseRows,
		RefundRows:           refundRows,
		PriorPurchaseRows:    priorPurchaseRows,
		PriorRefundRows:      priorRefundRows,
		RangeUniqueCustomers: uniq,
		PriorUniqueCustomers: priorUniq,
		// CurrentStart/End describe the selected period for cards/metadata.
		CurrentStart: windows.SelectedStart,
		CurrentEnd:   windows.SelectedEnd,
		PriorStart:   windows.PriorStart,
		PriorEnd:     windows.PriorEnd,
		// Trend window for dense bucket materialization.
		TrendStart: windows.TrendStart,
		TrendEnd:   windows.TrendEnd,
	})
}
