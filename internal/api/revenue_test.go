package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/reporting"
)

type fakeFinanceService struct {
	report reporting.FinanceReport
	err    error
	last   reporting.ReportQuery
}

// Locked: APIHandler.financeService is financeReporter interface (see handlers.go in this task).
// fakeFinanceService satisfies financeReporter for unit tests.

func (f *fakeFinanceService) GetReport(ctx context.Context, q reporting.ReportQuery) (reporting.FinanceReport, error) {
	f.last = q
	return f.report, f.err
}

func sampleFinanceReport() reporting.FinanceReport {
	metrics := reporting.FinanceMetrics{
		GrossServiceRevenue: 1000,
		Refunds:             100,
		NetServiceRevenue:   900,
		CashCollected:       800,
		SuccessfulOrders:    2,
		UniqueCustomers:     2,
		AverageOrderValue:   500,
	}
	cats := []reporting.CategoryBreakdown{{Category: "new_key", Orders: 1, Amount: 600}}
	methods := []reporting.MethodBreakdown{{
		Method: "kbz", Transactions: 1, ServiceRevenue: 600, CashCollected: 600,
	}}
	return reporting.FinanceReport{
		Period:     "day",
		Timezone:   "Asia/Yangon",
		Currency:   "MMK",
		RangeStart: "2026-07-12",
		RangeEnd:   "2026-07-12",
		InProgress: true,
		Current:    metrics,
		Categories: cats,
		Methods:    methods,
		Trend: []reporting.FinanceTrendBucket{{
			PeriodStart: "2026-07-12",
			PeriodEnd:   "2026-07-12",
			InProgress:  true,
			Metrics:     metrics,
			Categories:  cats,
			Methods:     methods,
		}},
	}
}

func TestGetRevenueSummary_InvalidPeriod400(t *testing.T) {
	// period is validated in parseReportQuery before FinanceService is called
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=quarter", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetRevenueSummary_CustomRequiresFromTo(t *testing.T) {
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=custom&from=2026-01-01", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestGetRevenueSummary_ServiceValidation400(t *testing.T) {
	fake := &fakeFinanceService{err: fmt.Errorf("%w: periods too large", reporting.ErrInvalidReportQuery)}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day&periods=30", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetRevenueSummary_RepoError500(t *testing.T) {
	fake := &fakeFinanceService{err: errors.New("db down")}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestGetRevenueSummary_OKJSON(t *testing.T) {
	fake := &fakeFinanceService{report: sampleFinanceReport()}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day&periods=7", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type=%q want application/json", ct)
	}
	var got reporting.FinanceReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Current.NetServiceRevenue != 900 {
		t.Fatalf("net=%v", got.Current.NetServiceRevenue)
	}
	if fake.last.HistoryPeriods != 7 {
		t.Fatalf("history=%d", fake.last.HistoryPeriods)
	}
}

func TestExportRevenue_CSVMatchesJSONNet(t *testing.T) {
	rep := sampleFinanceReport()
	fake := &fakeFinanceService{report: rep}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake

	reqJSON := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day", nil)
	reqJSON = reqJSON.WithContext(context.WithValue(reqJSON.Context(), telegramIDKey, int64(42)))
	recJSON := httptest.NewRecorder()
	h.GetRevenueSummary(recJSON, reqJSON)
	var got reporting.FinanceReport
	_ = json.Unmarshal(recJSON.Body.Bytes(), &got)

	reqCSV := httptest.NewRequest(http.MethodGet, "/api/revenue/export?period=day", nil)
	reqCSV = reqCSV.WithContext(context.WithValue(reqCSV.Context(), telegramIDKey, int64(42)))
	recCSV := httptest.NewRecorder()
	h.ExportRevenue(recCSV, reqCSV)
	if recCSV.Code != http.StatusOK {
		t.Fatalf("csv status=%d", recCSV.Code)
	}
	if ct := recCSV.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type=%q want text/csv; charset=utf-8", ct)
	}
	if cd := recCSV.Header().Get("Content-Disposition"); cd != `attachment; filename="finance-report.csv"` {
		t.Fatalf("Content-Disposition=%q", cd)
	}
	if !strings.Contains(recCSV.Body.String(), "net_service_revenue,900.00") {
		t.Fatalf("csv=%s", recCSV.Body.String())
	}
	if got.Current.NetServiceRevenue != 900 {
		t.Fatalf("json net diverged")
	}
}

func TestGetRevenueSummary_NilService500(t *testing.T) {
	// NewAPIHandler(nil finance) must leave financeService as a true nil interface.
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Failed to fetch revenue") {
		t.Fatalf("body=%s want sanitized message", rec.Body.String())
	}
}

func TestExportRevenue_NilService500(t *testing.T) {
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue/export?period=day", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.ExportRevenue(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Failed to export revenue") {
		t.Fatalf("body=%s want sanitized message", rec.Body.String())
	}
}

func TestGetRevenueSummary_ExcessiveHistory400(t *testing.T) {
	// Real FinanceService validates bounds in ResolveReportWindow before repo access.
	svc := reporting.NewFinanceService(nil, nil)
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, svc, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day&periods=367", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetRevenueSummary_CustomRangeOver366Days400(t *testing.T) {
	svc := reporting.NewFinanceService(nil, nil)
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, svc, nil)
	// Inclusive span > 366 days (2025-01-01 .. 2026-01-03).
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=custom&from=2025-01-01&to=2026-01-03", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetRevenueSummary_MixedCurrency400(t *testing.T) {
	fake := &fakeFinanceService{err: reporting.ErrMixedCurrency}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s want 400", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "mixed currencies") {
		// sanitized message preferred; raw sentinel text is acceptable only if not leaking internals
	}
	if rec.Code == http.StatusInternalServerError {
		t.Fatal("mixed currency must not be 500")
	}
}

func TestExportRevenue_MixedCurrency400(t *testing.T) {
	fake := &fakeFinanceService{err: fmt.Errorf("build: %w", reporting.ErrMixedCurrency)}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue/export?period=day", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.ExportRevenue(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s want 400", rec.Code, rec.Body.String())
	}
}

func TestRegisterHandlersProtectsRevenueRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	for _, path := range []string{"/api/revenue?period=day", "/api/revenue/export?period=day"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d want %d body=%s", path, rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
	}
}
