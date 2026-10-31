package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"remnawave-tg-shop-bot/internal/database"
)

func TestRegisterHandlersProtectsHideKeyRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "hide by path param", method: http.MethodPost, path: "/api/keys/123/hide"},
		{name: "archive by path param", method: http.MethodPost, path: "/api/keys/123/archive"},
		{name: "hide by body", method: http.MethodPost, path: "/api/keys/hide"},
		{name: "archive by body", method: http.MethodPost, path: "/api/keys/archive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://example.com"+tt.path, strings.NewReader(`{"key_id":123}`))
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHideKeyRejectsNonPostMethods(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/keys/123/hide", nil)
			req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(12345)))
			rec := httptest.NewRecorder()

			handler.KeyRoute(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected status 405 MethodNotAllowed, got %d", rec.Code)
			}
		})
	}
}

func TestHideKeyRejectsInvalidKeyIDs(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(ctx context.Context, tid int64) (*database.Customer, error) {
		return &database.Customer{ID: 10, TelegramID: tid}, nil
	}

	invalidPaths := []string{
		"/api/keys/0/hide",
		"/api/keys/-5/hide",
		"/api/keys/abc/hide",
	}

	for _, path := range invalidPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(12345)))
			rec := httptest.NewRecorder()

			handler.KeyRoute(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("path %q status = %d, want %d", path, rec.Code, http.StatusBadRequest)
			}
		})
	}

	// Also test body endpoint with invalid key_id
	bodyReqs := []string{
		`{"key_id": 0}`,
		`{"key_id": -10}`,
		`{"key_id": "not-an-int"}`,
	}
	for _, body := range bodyReqs {
		t.Run("body: "+body, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/keys/hide", strings.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(12345)))
			rec := httptest.NewRecorder()

			handler.HideKey(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("body %q status = %d, want %d", body, rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHideKeyErrorMatrix(t *testing.T) {
	tests := []struct {
		name       string
		hideErr    error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "key not found",
			hideErr:    database.ErrKeyNotFound,
			wantStatus: http.StatusNotFound,
			wantBody:   "Subscription key not found",
		},
		{
			name:       "key not owned by customer",
			hideErr:    database.ErrKeyNotOwned,
			wantStatus: http.StatusNotFound,
			wantBody:   "Subscription key not found",
		},
		{
			name:       "key is active",
			hideErr:    database.ErrKeyActive,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Active keys cannot be hidden",
		},
		{
			name:       "key is deleted",
			hideErr:    database.ErrKeyDeleted,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Deleted keys cannot be hidden",
		},
		{
			name:       "key has indeterminate expiry",
			hideErr:    database.ErrKeyIndeterminate,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Keys with indeterminate expiry cannot be hidden",
		},
		{
			name:       "renewal in flight conflict",
			hideErr:    database.ErrKeyRenewalInFlight,
			wantStatus: http.StatusConflict,
			wantBody:   "Renewal in progress, please try again shortly",
		},
		{
			name:       "internal database error",
			hideErr:    errors.New("db connection lost"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to hide key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			handler.findCustomerByTelegramID = func(ctx context.Context, tid int64) (*database.Customer, error) {
				return &database.Customer{ID: 10, TelegramID: tid}, nil
			}
			handler.hideSubscriptionKey = func(ctx context.Context, keyID, customerID int64) error {
				if customerID != 10 || keyID != 42 {
					t.Fatalf("hideSubscriptionKey called with customerID=%d, keyID=%d", customerID, keyID)
				}
				return tt.hideErr
			}

			req := httptest.NewRequest(http.MethodPost, "/api/keys/42/hide", nil)
			req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(12345)))
			rec := httptest.NewRecorder()

			handler.KeyRoute(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want containing %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHideKeySuccessBothPathAndBodyEndpoints(t *testing.T) {
	called := false
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(ctx context.Context, tid int64) (*database.Customer, error) {
		return &database.Customer{ID: 10, TelegramID: tid}, nil
	}
	handler.hideSubscriptionKey = func(ctx context.Context, keyID, customerID int64) error {
		if keyID == 42 && customerID == 10 {
			called = true
			return nil
		}
		return errors.New("unexpected params")
	}

	// 1. Path endpoint: POST /api/keys/42/hide
	req1 := httptest.NewRequest(http.MethodPost, "/api/keys/42/hide", nil)
	req1 = req1.WithContext(context.WithValue(req1.Context(), telegramIDKey, int64(12345)))
	rec1 := httptest.NewRecorder()

	handler.KeyRoute(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("KeyRoute status = %d, want 200", rec1.Code)
	}
	if !called {
		t.Fatal("hideSubscriptionKey was not called")
	}
	var resp1 map[string]interface{}
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("unmarshal json error: %v", err)
	}
	if resp1["success"] != true || resp1["key_id"] != float64(42) {
		t.Fatalf("unexpected response: %+v", resp1)
	}

	// 2. Body endpoint: POST /api/keys/hide
	called = false
	req2 := httptest.NewRequest(http.MethodPost, "/api/keys/hide", strings.NewReader(`{"key_id":42}`))
	req2 = req2.WithContext(context.WithValue(req2.Context(), telegramIDKey, int64(12345)))
	rec2 := httptest.NewRecorder()

	handler.HideKey(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("HideKey status = %d, want 200", rec2.Code)
	}
	if !called {
		t.Fatal("hideSubscriptionKey was not called for body endpoint")
	}
}

func TestGetMeExcludesHiddenKeysPreservesLegacyAndTrialInvariants(t *testing.T) {
	now := time.Now()
	expiredPast := now.Add(-48 * time.Hour)
	legacySubURL := "https://example.com/legacy-fallback"

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(ctx context.Context, tid int64) (*database.Customer, error) {
		return &database.Customer{
			ID:               10,
			TelegramID:       tid,
			SubscriptionLink: &legacySubURL,
			ExpireAt:         &expiredPast,
		}, nil
	}
	handler.getCachedSyncKeys = func(int64) ([]syncKeyStats, bool) { return nil, true }

	// Customer has one expired key which is marked IsHidden=true
	handler.findSubscriptionKeys = func(ctx context.Context, custID int64) ([]database.SubscriptionKey, error) {
		return []database.SubscriptionKey{
			{
				ID:              99,
				CustomerID:      custID,
				RemnawaveUUID:   uuid.New(),
				SubscriptionURL: "https://example.com/hidden-sub",
				Status:          "expired",
				ExpireAt:        &expiredPast,
				IsHidden:        true,
				CreatedAt:       now.Add(-72 * time.Hour),
			},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(12345)))
	rec := httptest.NewRecorder()

	handler.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMe status = %d, want 200", rec.Code)
	}

	var resp ValidationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Invariant 1: Hidden key is not shown in keys array
	if len(resp.Keys) != 0 {
		t.Fatalf("GetMe returned %d keys, want 0 (hidden key must be excluded)", len(resp.Keys))
	}

	// Invariant 2: Legacy fallback is NOT activated (hasMigratedKeys was true)
	// If legacy fallback activated, it would have created a Key 1 with ID 0.
	for _, k := range resp.Keys {
		if k.ID == 0 {
			t.Fatal("legacy fallback resurrected key with ID 0")
		}
	}

	// Invariant 3: Customer is not marked active
	if resp.IsActive {
		t.Fatal("resp.IsActive = true, want false")
	}

	// Invariant 4: Trial eligibility is NOT restored (user has key history)
	if resp.TrialEligible {
		t.Fatal("resp.TrialEligible = true, want false (hiding key must not reset trial)")
	}
}

func TestCompactSubscriptionKeysForDisplayPreservesHiddenState(t *testing.T) {
	now := time.Now()
	expired := now.Add(-10 * time.Hour)
	url := "https://example.com/sub/duplicate"

	// Two duplicate rows for the same subscription URL (e.g. from past reconciliations)
	// One is marked IsHidden=true, the older one is not
	keys := []database.SubscriptionKey{
		{
			ID:              1,
			SubscriptionURL: url,
			Status:          "expired",
			ExpireAt:        &expired,
			IsHidden:        false,
			CreatedAt:       now.Add(-5 * time.Hour),
		},
		{
			ID:              2,
			SubscriptionURL: url,
			Status:          "expired",
			ExpireAt:        &expired,
			IsHidden:        true,
			CreatedAt:       now.Add(-2 * time.Hour),
		},
	}

	compacted := compactSubscriptionKeysForDisplay(keys)
	if len(compacted) != 1 {
		t.Fatalf("compacted len = %d, want 1", len(compacted))
	}
	if !compacted[0].IsHidden {
		t.Fatal("compacted key IsHidden = false, want true (hidden state must be preserved)")
	}
}
