package reporting

import (
	"context"
	"errors"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type fakePurchases struct {
	rows []database.RevenueSummaryRow
	n    int
	err  error
}

func (f *fakePurchases) GetRevenueSummaryRange(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod) ([]database.RevenueSummaryRow, error) {
	return f.rows, f.err
}
func (f *fakePurchases) CountDistinctServiceCustomers(ctx context.Context, start, end time.Time) (int, error) {
	return f.n, f.err
}

type fakeRefunds struct {
	rows []database.RefundPeriodRow
	err  error
}

func (f *fakeRefunds) SumRefundsByPeriod(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod, adminTelegramID int64) ([]database.RefundPeriodRow, error) {
	return f.rows, f.err
}

func TestFinanceService_GetReport_InvalidPeriod(t *testing.T) {
	s := NewFinanceService(&fakePurchases{}, &fakeRefunds{})
	_, err := s.GetReport(context.Background(), ReportQuery{Period: database.RevenueSummaryPeriod("quarter")})
	if !errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v", err)
	}
}

func TestFinanceService_GetReport_RepoErrorIsNotValidation(t *testing.T) {
	s := NewFinanceService(&fakePurchases{err: errors.New("db down")}, &fakeRefunds{})
	loc := YangonLocation()
	_, err := s.GetReport(context.Background(), ReportQuery{
		Period: database.RevenuePeriodDay,
		Now:    time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
	})
	if err == nil || errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v want non-validation repo error", err)
	}
}
