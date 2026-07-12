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

type fakeSettlements struct {
	rows []database.SettlementPeriodRow
	err  error
}

func (f *fakeSettlements) SumSettlementsByPeriod(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod) ([]database.SettlementPeriodRow, error) {
	return f.rows, f.err
}

func TestFinanceService_GetReport_InvalidPeriod(t *testing.T) {
	s := NewFinanceService(&fakePurchases{}, &fakeRefunds{}, nil)
	_, err := s.GetReport(context.Background(), ReportQuery{Period: database.RevenueSummaryPeriod("quarter")})
	if !errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v", err)
	}
}

func TestFinanceService_GetReport_RepoErrorIsNotValidation(t *testing.T) {
	s := NewFinanceService(&fakePurchases{err: errors.New("db down")}, &fakeRefunds{}, nil)
	loc := YangonLocation()
	_, err := s.GetReport(context.Background(), ReportQuery{
		Period: database.RevenuePeriodDay,
		Now:    time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
	})
	if err == nil || errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v want non-validation repo error", err)
	}
}

func TestFinanceService_GetReport_SettlementsIncreaseCashCollected(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, loc)
	s := NewFinanceService(
		&fakePurchases{
			rows: []database.RevenueSummaryRow{{
				PeriodStart:          "2026-07-12",
				Currency:             "MMK",
				PeriodServiceRevenue: 4000,
				PeriodCashCollected:  0, // postpaid sale: revenue without cash
				PeriodServicePurchases: 1,
			}},
			n: 1,
		},
		&fakeRefunds{},
		&fakeSettlements{
			rows: []database.SettlementPeriodRow{{
				PeriodStart:     "2026-07-12",
				Currency:        "MMK",
				SettlementTotal: 1000,
				SettlementCount: 1,
			}},
		},
	)
	report, err := s.GetReport(context.Background(), ReportQuery{
		Period: database.RevenuePeriodDay,
		Now:    now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.GrossServiceRevenue != 4000 {
		t.Fatalf("gross=%v want 4000", report.Current.GrossServiceRevenue)
	}
	if report.Current.CashCollected != 1000 {
		t.Fatalf("cash=%v want 1000 (settlement only)", report.Current.CashCollected)
	}
	if report.Current.NetServiceRevenue != 4000 {
		t.Fatalf("net=%v want 4000", report.Current.NetServiceRevenue)
	}
}
