package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func TestGetMeIncludesSupportURL(t *testing.T) {
	restore := config.SetSupportURLForTesting("https://t.me/my-support")
	defer restore()

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         1,
			TelegramID: telegramID,
			Language:   "en",
		}, nil
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
	if !strings.Contains(rec.Body.String(), `"support_url":"https://t.me/my-support"`) {
		t.Fatalf("GetMe() body = %s, want support_url field", rec.Body.String())
	}
}
