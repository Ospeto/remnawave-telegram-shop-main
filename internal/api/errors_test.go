package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/database"
)

func TestWriteAPIErrorEmitsCodeAndMessage(t *testing.T) {
	rec := httptest.NewRecorder()

	writeAPIError(rec, http.StatusNotFound, errorCodePurchaseNotFound, "Purchase not found")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (body=%q)", err, rec.Body.String())
	}
	if body.Code != errorCodePurchaseNotFound {
		t.Fatalf("code = %q, want %q", body.Code, errorCodePurchaseNotFound)
	}
	if body.Message != "Purchase not found" {
		t.Fatalf("message = %q, want %q", body.Message, "Purchase not found")
	}
}

func TestWriteAPIErrorLogsServerFailures(t *testing.T) {
	logs := captureSlogOutput(t)
	rec := httptest.NewRecorder()

	writeAPIErrorWithCause(rec, http.StatusInternalServerError, errorCodeDatabase, "Database error", errors.New("pool exhausted"))

	if !strings.Contains(logs.String(), "code=database_error") {
		t.Fatalf("server-side failure was not logged: %s", logs.String())
	}
}

func TestWriteAPIErrorDoesNotLogClientFailures(t *testing.T) {
	logs := captureSlogOutput(t)
	rec := httptest.NewRecorder()

	writeAPIError(rec, http.StatusBadRequest, errorCodeInvalidBody, "Invalid request body")

	if strings.Contains(logs.String(), "API request failed") {
		t.Fatalf("a 4xx must not be logged as a server failure: %s", logs.String())
	}
}

// TestGetMeRejectsUnauthorizedWithMachineReadableCode pins the client contract:
// the Mini App maps these codes to localized copy instead of printing English.
func TestGetMeRejectsUnauthorizedWithMachineReadableCode(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()

	handler.GetMe(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	var body apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (body=%q)", err, rec.Body.String())
	}
	if body.Code != errorCodeUnauthorized {
		t.Fatalf("code = %q, want %q", body.Code, errorCodeUnauthorized)
	}
}

func TestGetMeUserNotFoundUsesItsOwnCode(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(context.Context, int64) (*database.Customer, error) {
		return nil, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.GetMe(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errorCodeUserNotFound) {
		t.Fatalf("body = %s, want code %q", rec.Body.String(), errorCodeUserNotFound)
	}
}
