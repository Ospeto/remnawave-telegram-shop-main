package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"remnawave-tg-shop-bot/internal/database"
)

func TestPatchAdminCustomerResellerTrue(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	var updatedID int64
	var updatedFields map[string]interface{}
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         7,
			TelegramID: telegramID,
			IsReseller: false,
		}, nil
	}
	handler.updateCustomerFields = func(_ context.Context, id int64, updates map[string]interface{}) error {
		updatedID = id
		updatedFields = updates
		return nil
	}

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/admin/customers/12345/reseller",
		bytes.NewBufferString(`{"is_reseller":true}`),
	)
	rec := httptest.NewRecorder()

	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("AdminCustomerByTelegramID() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload AdminResellerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body=%q", err, rec.Body.String())
	}
	if payload.TelegramID != 12345 {
		t.Fatalf("telegram_id = %d, want 12345", payload.TelegramID)
	}
	if !payload.IsReseller {
		t.Fatal("is_reseller = false, want true")
	}
	if updatedID != 7 {
		t.Fatalf("UpdateFields id = %d, want 7", updatedID)
	}
	if updatedFields == nil {
		t.Fatal("UpdateFields was not called")
	}
	got, ok := updatedFields["is_reseller"].(bool)
	if !ok || !got {
		t.Fatalf("UpdateFields is_reseller = %#v, want true", updatedFields["is_reseller"])
	}
}

func TestRegisterHandlersProtectsAdminResellerRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "list resellers", method: http.MethodGet, path: "/api/admin/resellers"},
		{name: "toggle reseller", method: http.MethodPatch, path: "/api/admin/customers/12345/reseller"},
		{name: "set credit", method: http.MethodPatch, path: "/api/admin/customers/12345/credit"},
		{name: "admin settlement", method: http.MethodPost, path: "/api/admin/customers/12345/settlements"},
		{name: "admin ledger", method: http.MethodGet, path: "/api/admin/customers/12345/ledger"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://example.com"+tt.path, bytes.NewBufferString(`{"is_reseller":true}`))
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestListResellersReturnsResellers(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.listResellersFn = func(_ context.Context) ([]database.Customer, error) {
		return []database.Customer{
			{ID: 1, TelegramID: 111, IsReseller: true},
			{ID: 2, TelegramID: 222, IsReseller: true},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/resellers", nil)
	rec := httptest.NewRecorder()

	handler.ListResellers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListResellers() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload []AdminResellerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("ListResellers() len = %d, want 2", len(payload))
	}
	if payload[0].TelegramID != 111 || !payload[0].IsReseller {
		t.Fatalf("payload[0] = %+v", payload[0])
	}
	if payload[1].TelegramID != 222 || !payload[1].IsReseller {
		t.Fatalf("payload[1] = %+v", payload[1])
	}
}
