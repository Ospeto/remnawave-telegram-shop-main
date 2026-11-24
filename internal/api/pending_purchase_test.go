package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// pendingPurchaseGetMeHandler builds an APIHandler whose /api/me path is fully
// stubbed, so a test can assert only what it cares about.
func pendingPurchaseGetMeHandler(t *testing.T, pending *database.Purchase, lookupErr error) *APIHandler {
	t.Helper()

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, Language: "en"}, nil
	}
	// A warm cache keeps these tests off the bounded cold-cache wait (U4).
	handler.getCachedSyncKeys = func(int64) ([]syncKeyStats, bool) { return nil, true }
	handler.triggerSyncKeys = func(context.Context, int64, int64) {}
	handler.findPendingPurchase = func(context.Context, int64) (*database.Purchase, error) {
		return pending, lookupErr
	}
	return handler
}

func decodeMeResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GetMe() body is not JSON: %v (body=%q)", err, rec.Body.String())
	}
	return body
}

func TestGetMeExposesPendingPurchaseSoTheAppCanShowIt(t *testing.T) {
	created := time.Date(2026, 4, 1, 9, 30, 0, 0, time.UTC)
	pending := &database.Purchase{
		ID:            4242,
		CustomerID:    7,
		Status:        database.PurchaseStatusPending,
		InvoiceType:   database.InvoiceTypeMobileBanking,
		Amount:        15000,
		Currency:      "",
		PlanLabel:     "1 month",
		Days:          30,
		TrafficLimitGB: 100,
		CreatedAt:     created,
	}
	handler := pendingPurchaseGetMeHandler(t, pending, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(555)))
	rec := httptest.NewRecorder()
	handler.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMe() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := decodeMeResponse(t, rec)
	raw, ok := body["pending_purchase"]
	if !ok {
		t.Fatalf("GetMe() body has no pending_purchase: %s", rec.Body.String())
	}
	info, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("pending_purchase is %T, want an object", raw)
	}
	if got := info["id"]; got != float64(4242) {
		t.Fatalf("pending_purchase.id = %v, want 4242", got)
	}
	if got := info["status"]; got != string(database.PurchaseStatusPending) {
		t.Fatalf("pending_purchase.status = %v, want %q", got, database.PurchaseStatusPending)
	}
	if got := info["invoice_type"]; got != string(database.InvoiceTypeMobileBanking) {
		t.Fatalf("pending_purchase.invoice_type = %v, want %q", got, database.InvoiceTypeMobileBanking)
	}
	if got := info["awaiting_screenshot"]; got != true {
		t.Fatalf("pending_purchase.awaiting_screenshot = %v, want true", got)
	}
	if got := info["created_at"]; got != created.Format(time.RFC3339) {
		t.Fatalf("pending_purchase.created_at = %v, want %q", got, created.Format(time.RFC3339))
	}
	if got := info["amount"]; got != float64(15000) {
		t.Fatalf("pending_purchase.amount = %v, want 15000", got)
	}
}

func TestGetMeMarksProcessingPurchaseAsNotAwaitingScreenshot(t *testing.T) {
	handler := pendingPurchaseGetMeHandler(t, &database.Purchase{
		ID:          77,
		CustomerID:  7,
		Status:      database.PurchaseStatusProcessing,
		InvoiceType: database.InvoiceTypeMobileBanking,
		Amount:      9000,
		CreatedAt:   time.Date(2026, 4, 1, 9, 30, 0, 0, time.UTC),
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(555)))
	rec := httptest.NewRecorder()
	handler.GetMe(rec, req)

	body := decodeMeResponse(t, rec)
	info, ok := body["pending_purchase"].(map[string]any)
	if !ok {
		t.Fatalf("pending_purchase missing for a processing purchase: %s", rec.Body.String())
	}
	if got := info["awaiting_screenshot"]; got != false {
		t.Fatalf("waiting_screenshot = %v, want false for a processing purchase", got)
	}
}

func TestGetMeOmitsPendingPurchaseWhenNothingIsInFlight(t *testing.T) {
	handler := pendingPurchaseGetMeHandler(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(555)))
	rec := httptest.NewRecorder()
	handler.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMe() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "pending_purchase") {
		t.Fatalf("GetMe() body must omit pending_purchase when nothing is in flight: %s", rec.Body.String())
	}
}

func TestGetMeSurvivesAPendingPurchaseLookupFailure(t *testing.T) {
	handler := pendingPurchaseGetMeHandler(t, nil, context.DeadlineExceeded)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(555)))
	rec := httptest.NewRecorder()
	handler.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMe() must not fail because of the pending lookup: status = %d body=%q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "pending_purchase") {
		t.Fatalf("GetMe() body must omit pending_purchase after a lookup failure: %s", rec.Body.String())
	}
}
