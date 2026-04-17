package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"

	"github.com/jackc/pgconn"
)

func TestValidationResponseJSONIncludesIsAdmin(t *testing.T) {
	body, err := json.Marshal(ValidationResponse{IsAdmin: true})
	if err != nil {
		t.Fatalf("json.Marshal(ValidationResponse) error = %v", err)
	}

	if !strings.Contains(string(body), `"is_admin":true`) {
		t.Fatalf("ValidationResponse JSON = %s, want is_admin field", string(body))
	}
}

func TestRegisterHandlersProtectsAdminPromoRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "list promos", method: http.MethodGet, path: "/api/admin/promos"},
		{name: "create promo", method: http.MethodPost, path: "/api/admin/promos"},
		{name: "delete promo", method: http.MethodDelete, path: "/api/admin/promos/SALE50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://example.com"+tt.path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestListAdminPromosReturnsStatusInResponse(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.now = func() time.Time { return now }
	handler.listPromoCodes = func(context.Context) ([]database.PromoCode, error) {
		return []database.PromoCode{
			{
				Code:            "ACTIVE",
				DiscountPercent: 15,
				MaxUses:         10,
				UsedCount:       2,
				ValidUntil:      now.Add(24 * time.Hour),
			},
			{
				Code:            "EXPIRED",
				DiscountPercent: 20,
				MaxUses:         10,
				UsedCount:       1,
				ValidUntil:      now.Add(-time.Hour),
			},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/promos", nil)
	rec := httptest.NewRecorder()

	handler.ListAdminPromos(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListAdminPromos() status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("ListAdminPromos() response len = %d, want %d", len(payload), 2)
	}
	if payload[0]["status"] != "active" {
		t.Fatalf("ListAdminPromos() first status = %v, want active", payload[0]["status"])
	}
	if payload[1]["status"] != "expired" {
		t.Fatalf("ListAdminPromos() second status = %v, want expired", payload[1]["status"])
	}
}

func TestCreateAdminPromoUsesSharedValidationAndPersistsPromo(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.now = func() time.Time { return now }

	var created struct {
		code       string
		discount   int
		maxUses    int
		validUntil time.Time
	}
	handler.createPromoCode = func(_ context.Context, code string, discount, maxUses int, validUntil time.Time) error {
		created.code = code
		created.discount = discount
		created.maxUses = maxUses
		created.validUntil = validUntil
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/promos", bytes.NewBufferString(`{"code":"sale50","discount_percent":50,"duration_days":10,"max_uses":100}`))
	rec := httptest.NewRecorder()

	handler.CreateAdminPromo(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateAdminPromo() status = %d, want %d body=%q", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if created.code != "sale50" {
		t.Fatalf("CreateAdminPromo() created code = %q, want %q", created.code, "sale50")
	}
	if created.discount != 50 {
		t.Fatalf("CreateAdminPromo() created discount = %d, want %d", created.discount, 50)
	}
	if created.maxUses != 100 {
		t.Fatalf("CreateAdminPromo() created maxUses = %d, want %d", created.maxUses, 100)
	}
	if created.validUntil != now.Add(10*24*time.Hour) {
		t.Fatalf("CreateAdminPromo() created validUntil = %v, want %v", created.validUntil, now.Add(10*24*time.Hour))
	}
}

func TestCreateAdminPromoRejectsInvalidDuration(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)

	called := false
	handler.createPromoCode = func(_ context.Context, code string, discount, maxUses int, validUntil time.Time) error {
		called = true
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/promos", bytes.NewBufferString(`{"code":"sale50","discount_percent":50,"duration_days":0,"max_uses":100}`))
	rec := httptest.NewRecorder()

	handler.CreateAdminPromo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreateAdminPromo() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("CreateAdminPromo() called createPromoCode for invalid request")
	}
}

func TestCreateAdminPromoReturnsConflictForDuplicateCode(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.createPromoCode = func(_ context.Context, code string, discount, maxUses int, validUntil time.Time) error {
		return fmt.Errorf("failed to execute insert query: %w", &pgconn.PgError{Code: "23505"})
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/promos", bytes.NewBufferString(`{"code":"sale50","discount_percent":50,"duration_days":10,"max_uses":100}`))
	rec := httptest.NewRecorder()

	handler.CreateAdminPromo(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("CreateAdminPromo() status = %d, want %d body=%q", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Promo code already exists") {
		t.Fatalf("CreateAdminPromo() body = %q, want duplicate message", rec.Body.String())
	}
}

func TestDeleteAdminPromoDeletesByCode(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)

	deletedCode := ""
	handler.deletePromoCode = func(_ context.Context, code string) error {
		deletedCode = code
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/promos/SALE50", nil)
	rec := httptest.NewRecorder()

	handler.DeleteAdminPromo(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DeleteAdminPromo() status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if deletedCode != "SALE50" {
		t.Fatalf("DeleteAdminPromo() deleted code = %q, want %q", deletedCode, "SALE50")
	}
}

func TestDeleteAdminPromoRetiresReferencedPromoCode(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.now = func() time.Time {
		return time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	}
	handler.deletePromoCode = func(_ context.Context, code string) error {
		return fmt.Errorf("failed to delete promo code: %w", &pgconn.PgError{Code: "23503"})
	}

	retiredCode := ""
	var retiredAt time.Time
	handler.retirePromoCode = func(_ context.Context, code string, at time.Time) error {
		retiredCode = code
		retiredAt = at
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/promos/SALE50", nil)
	rec := httptest.NewRecorder()

	handler.DeleteAdminPromo(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DeleteAdminPromo() status = %d, want %d body=%q", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if retiredCode != "SALE50" {
		t.Fatalf("DeleteAdminPromo() retired code = %q, want %q", retiredCode, "SALE50")
	}
	wantRetiredAt := handler.currentTime().Add(-time.Second)
	if !retiredAt.Equal(wantRetiredAt) {
		t.Fatalf("DeleteAdminPromo() retiredAt = %v, want %v", retiredAt, wantRetiredAt)
	}
}

func TestDeleteAdminPromoReturnsInternalServerErrorForRepositoryFailure(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.deletePromoCode = func(_ context.Context, code string) error {
		return fmt.Errorf("database offline")
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/promos/SALE50", nil)
	rec := httptest.NewRecorder()

	handler.DeleteAdminPromo(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("DeleteAdminPromo() status = %d, want %d body=%q", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestGetMeIncludesAdminFlag(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         1,
			TelegramID: telegramID,
			Language:   "en",
		}, nil
	}
	handler.isAdminTelegramID = func(telegramID int64) bool {
		return telegramID == 42
	}
	handler.getCachedSyncKeys = func(int64) ([]syncKeyStats, bool) {
		return nil, false
	}
	handler.triggerSyncKeys = func(context.Context, int64, int64) {}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMe() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_admin":true`) {
		t.Fatalf("GetMe() body = %s, want is_admin=true", rec.Body.String())
	}
}
