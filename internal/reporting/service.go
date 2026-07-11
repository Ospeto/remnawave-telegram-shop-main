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

	cs, ce, ps, pe, err := ResolveReportWindow(period, now, q.HistoryPeriods, q.CustomFrom, q.CustomTo)
	if err != nil {
		return FinanceReport{}, err
	}

	purchaseRows, err := s.purchases.GetRevenueSummaryRange(ctx, cs, ce, bucketPeriod)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load purchase revenue: %w", err)
	}
	priorPurchaseRows, err := s.purchases.GetRevenueSummaryRange(ctx, ps, pe, bucketPeriod)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load prior purchase revenue: %w", err)
	}
	adminID := s.adminID()
	refundRows, err := s.refunds.SumRefundsByPeriod(ctx, cs, ce, bucketPeriod, adminID)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load refunds: %w", err)
	}
	priorRefundRows, err := s.refunds.SumRefundsByPeriod(ctx, ps, pe, bucketPeriod, adminID)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load prior refunds: %w", err)
	}
	uniq, err := s.purchases.CountDistinctServiceCustomers(ctx, cs, ce)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("count customers: %w", err)
	}
	priorUniq, err := s.purchases.CountDistinctServiceCustomers(ctx, ps, pe)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("count prior customers: %w", err)
	}

	return BuildFinanceReport(BuildFinanceReportInput{
		Period:               period,
		Now:                  now.In(YangonLocation()),
		HistoryPeriods:       q.HistoryPeriods,
		CustomFrom:           q.CustomFrom,
		CustomTo:             q.CustomTo,
		PurchaseRows:         purchaseRows,
		RefundRows:           refundRows,
		PriorPurchaseRows:    priorPurchaseRows,
		PriorRefundRows:      priorRefundRows,
		RangeUniqueCustomers: uniq,
		PriorUniqueCustomers: priorUniq,
		CurrentStart:         cs,
		CurrentEnd:           ce,
		PriorStart:           ps,
		PriorEnd:             pe,
	})
}
