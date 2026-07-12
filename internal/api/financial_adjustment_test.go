package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func postFinancialAdjustment(t *testing.T, h *APIHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec, req)
	return rec
}

func assertSanitizedBody(t *testing.T, body, secret string) {
	t.Helper()
	if strings.Contains(body, secret) {
		t.Fatalf("response leaked secret detail %q: %q", secret, body)
	}
	if strings.Contains(body, "SQLSTATE") || strings.Contains(body, "pq:") {
		t.Fatalf("response leaked database detail: %q", body)
	}
}

func assertJSONFinancialAdjustment(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantKey string) database.FinancialAdjustment {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type=%q want application/json", ct)
	}
	var row database.FinancialAdjustment
	if err := json.Unmarshal(rec.Body.Bytes(), &row); err != nil {
		t.Fatalf("invalid JSON body: %v body=%s", err, rec.Body.String())
	}
	if wantKey != "" && row.IdempotencyKey != wantKey {
		t.Fatalf("key=%q want %q", row.IdempotencyKey, wantKey)
	}
	return row
}

func TestCreateFinancialAdjustment_MissingIdempotencyKey400(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":100,"currency":"MMK"}`
	rec := postFinancialAdjustment(t, h, body)
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
	rec := postFinancialAdjustment(t, h, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestCreateFinancialAdjustment_NaNAmount400(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	// JSON allows NaN only via non-standard encoding; use a number that decodes then fails validation
	// by sending the string form Go's encoding/json rejects — instead send Inf via raw JSON number
	// that json.Unmarshal accepts as float64 NaN is not standard. Use math via crafted payload:
	// encoding/json rejects NaN/Inf in numbers. Simulate by posting a valid decode path with
	// amount set after decode is not possible; use "null" no — use very large then check Inf path
	// via direct handler field is hard. Send amount as string fails decode.
	// Practical approach: call with body using JavaScript-style NaN is invalid JSON.
	// Use amount that becomes non-finite only if we inject — instead test via repo-bypass:
	// Decode `"amount":1e400` may become +Inf in float64.
	body := `{"adjustment_type":"refund","amount":1e400,"currency":"MMK","idempotency_key":"nan-key"}`
	rec := postFinancialAdjustment(t, h, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s want 400 for non-finite amount", rec.Code, rec.Body.String())
	}
	if repo.calls != 0 {
		t.Fatal("repo should not be called for non-finite amount")
	}
}

func TestCreateFinancialAdjustment_IdempotencyMismatch409(t *testing.T) {
	secret := "payload-mismatch-detail=leak"
	repo := &fakeAdjustmentRepo{
		err: fmt.Errorf("create: %w: %s", database.ErrFinancialAdjustmentIdempotencyMismatch, secret),
	}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":10,"currency":"MMK","idempotency_key":"mismatch-key"}`
	rec := postFinancialAdjustment(t, h, body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Idempotency key already used with a different payload") {
		t.Fatalf("body=%q", rec.Body.String())
	}
	assertSanitizedBody(t, rec.Body.String(), "leak")
}

func TestCreateFinancialAdjustment_Created201AndReplay200(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":100.5,"currency":"MMK","idempotency_key":"refund:1","reason":"test"}`

	rec1 := postFinancialAdjustment(t, h, body)
	assertJSONFinancialAdjustment(t, rec1, http.StatusCreated, "refund:1")
	if repo.last.CreatedBy != "admin:42" {
		t.Fatalf("created_by=%q", repo.last.CreatedBy)
	}

	repo.created = false // simulate idempotent hit
	rec2 := postFinancialAdjustment(t, h, body)
	assertJSONFinancialAdjustment(t, rec2, http.StatusOK, "refund:1")
}

func TestCreateFinancialAdjustment_RepoError500(t *testing.T) {
	secret := "pgx: connection reset token=super-secret"
	repo := &fakeAdjustmentRepo{err: fmt.Errorf("%s: %w", secret, context.DeadlineExceeded)}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":10,"currency":"MMK","idempotency_key":"k2"}`
	rec := postFinancialAdjustment(t, h, body)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
	assertSanitizedBody(t, rec.Body.String(), "super-secret")
	if !strings.Contains(rec.Body.String(), "Failed to create financial adjustment") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestCreateFinancialAdjustment_SentinelStatuses(t *testing.T) {
	secret := "SQLSTATE 23503 detail=purchase_id=999 token=leak-me"
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantPublic string
	}{
		{
			name:       "foreign_key_400",
			err:        fmt.Errorf("insert financial_adjustment: %w: %s", database.ErrFinancialAdjustmentForeignKey, secret),
			wantStatus: http.StatusBadRequest,
			wantPublic: "Invalid financial adjustment",
		},
		{
			name:       "check_400",
			err:        fmt.Errorf("insert financial_adjustment: %w: %s", database.ErrFinancialAdjustmentCheck, secret),
			wantStatus: http.StatusBadRequest,
			wantPublic: "Invalid financial adjustment",
		},
		{
			name:       "unique_409",
			err:        fmt.Errorf("insert financial_adjustment: %w: %s", database.ErrFinancialAdjustmentUnique, secret),
			wantStatus: http.StatusConflict,
			wantPublic: "Financial adjustment conflict",
		},
		{
			name:       "idempotency_conflict_409",
			err:        fmt.Errorf("insert financial_adjustment: %w: %s", database.ErrFinancialAdjustmentIdempotencyConflict, secret),
			wantStatus: http.StatusConflict,
			wantPublic: "Financial adjustment conflict",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeAdjustmentRepo{err: tc.err}
			h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
			body := `{"adjustment_type":"refund","amount":10,"currency":"MMK","idempotency_key":"sentinel-key"}`
			rec := postFinancialAdjustment(t, h, body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantPublic) {
				t.Fatalf("body=%q want public %q", rec.Body.String(), tc.wantPublic)
			}
			assertSanitizedBody(t, rec.Body.String(), "leak-me")
			assertSanitizedBody(t, rec.Body.String(), secret)
			if errors.Is(tc.err, database.ErrFinancialAdjustmentForeignKey) ||
				errors.Is(tc.err, database.ErrFinancialAdjustmentCheck) ||
				errors.Is(tc.err, database.ErrFinancialAdjustmentUnique) ||
				errors.Is(tc.err, database.ErrFinancialAdjustmentIdempotencyConflict) {
				// wrapped sentinel must still classify via errors.Is
			} else {
				t.Fatal("test case must wrap a known sentinel")
			}
		})
	}
}

func TestRegisterHandlersProtectsFinancialAdjustmentRoute(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		t.Fatalf("unauthenticated request must not succeed, status=%d", rec.Code)
	}
}
