package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type fakeAdjustmentRepo struct {
	last    database.CreateFinancialAdjustmentInput
	row     *database.FinancialAdjustment
	created bool
	err     error
	calls   int
}

func (f *fakeAdjustmentRepo) Create(ctx context.Context, in database.CreateFinancialAdjustmentInput) (*database.FinancialAdjustment, bool, error) {
	f.calls++
	f.last = in
	if f.err != nil {
		return nil, false, f.err
	}
	if f.row == nil {
		id := int64(1)
		f.row = &database.FinancialAdjustment{
			ID:             id,
			AdjustmentType: in.AdjustmentType,
			Amount:         in.Amount,
			Currency:       in.Currency,
			EffectiveAt:    in.EffectiveAt,
			Reason:         in.Reason,
			ExternalRef:    in.ExternalRef,
			CreatedBy:      in.CreatedBy,
			IdempotencyKey: in.IdempotencyKey,
			CreatedAt:      time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
		}
		f.created = true
	}
	return f.row, f.created, nil
}

func TestCreateFinancialAdjustment_MissingIdempotencyKey400(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":100,"currency":"MMK"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.calls != 0 {
		t.Fatal("repo should not be called")
	}
}

func TestCreateFinancialAdjustment_NonPositiveAmount400(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":0,"currency":"MMK","idempotency_key":"k1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestCreateFinancialAdjustment_Created201AndReplay200(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":100.5,"currency":"MMK","idempotency_key":"refund:1","reason":"test"}`

	req1 := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req1 = req1.WithContext(context.WithValue(req1.Context(), telegramIDKey, int64(42)))
	rec1 := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	if repo.last.CreatedBy != "admin:42" {
		t.Fatalf("created_by=%q", repo.last.CreatedBy)
	}

	repo.created = false // simulate idempotent hit
	req2 := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req2 = req2.WithContext(context.WithValue(req2.Context(), telegramIDKey, int64(42)))
	rec2 := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("replay status=%d", rec2.Code)
	}
	var row database.FinancialAdjustment
	if err := json.Unmarshal(rec2.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row.IdempotencyKey != "refund:1" {
		t.Fatalf("key=%q", row.IdempotencyKey)
	}
}

func TestCreateFinancialAdjustment_RepoError500(t *testing.T) {
	repo := &fakeAdjustmentRepo{err: context.DeadlineExceeded}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":10,"currency":"MMK","idempotency_key":"k2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestRegisterHandlersProtectsFinancialAdjustmentRoute(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		t.Fatalf("unauthenticated request must not succeed, status=%d", rec.Code)
	}
}
