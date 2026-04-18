package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

func TestGetMeMarksUsedTrialsIneligible(t *testing.T) {
	restore := config.SetTrialConfigForTesting(7, 10)
	defer restore()

	usedAt := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:          1,
			TelegramID:  telegramID,
			Language:    "en",
			TrialUsedAt: &usedAt,
		}, nil
	}
	handler.getCachedSyncKeys = func(int64) ([]syncKeyStats, bool) { return nil, false }
	handler.triggerSyncKeys = func(context.Context, int64, int64) {}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMe() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"trial_eligible":false`) {
		t.Fatalf("GetMe() body = %s, want trial_eligible=false", rec.Body.String())
	}
}

func TestActivateTrialMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{name: "trial unavailable", err: payment.ErrTrialUnavailable, status: http.StatusBadRequest, body: "Trial is not available"},
		{name: "customer missing", err: payment.ErrCustomerNotFound, status: http.StatusNotFound, body: "User not found"},
		{name: "trial already used", err: payment.ErrTrialAlreadyUsed, status: http.StatusConflict, body: "Trial already used"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
			handler.activateCustomerTrial = func(context.Context, int64) (string, error) {
				return "", tt.err
			}

			ctx := context.WithValue(context.Background(), telegramIDKey, int64(42))
			ctx = context.WithValue(ctx, payment.UsernameCtxKey, "alice")
			req := httptest.NewRequest(http.MethodPost, "/api/trial", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			handler.ActivateTrial(rec, req)

			if rec.Code != tt.status {
				t.Fatalf("ActivateTrial() status = %d, want %d body=%q", rec.Code, tt.status, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.body) {
				t.Fatalf("ActivateTrial() body = %q, want %q", rec.Body.String(), tt.body)
			}
		})
	}
}

func TestActivateTrialSuccessReturnsSubscriptionURL(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.activateCustomerTrial = func(context.Context, int64) (string, error) {
		return "https://sub.example.com/trial", nil
	}

	ctx := context.WithValue(context.Background(), telegramIDKey, int64(42))
	ctx = context.WithValue(ctx, payment.UsernameCtxKey, "alice")
	req := httptest.NewRequest(http.MethodPost, "/api/trial", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ActivateTrial(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ActivateTrial() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"subscription_url":"https://sub.example.com/trial"`) {
		t.Fatalf("ActivateTrial() body = %s, want subscription URL", rec.Body.String())
	}
}
