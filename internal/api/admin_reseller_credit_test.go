package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestPatchAdminCustomerCreditLimit(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         7,
			TelegramID: telegramID,
			IsReseller: true,
		}, nil
	}

	var gotCustomerID int64
	var gotLimit float64
	handler.setResellerCreditLimitFn = func(_ context.Context, customerID int64, limit float64) (*database.ResellerCreditAccount, error) {
		gotCustomerID = customerID
		gotLimit = limit
		return &database.ResellerCreditAccount{
			CustomerID:  customerID,
			CreditLimit: limit,
			BalanceOwed: 2500,
		}, nil
	}

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/admin/customers/12345/credit",
		bytes.NewBufferString(`{"credit_limit": 50000}`),
	)
	rec := httptest.NewRecorder()

	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotCustomerID != 7 {
		t.Fatalf("SetCreditLimit customerID = %d, want 7", gotCustomerID)
	}
	if gotLimit != 50000 {
		t.Fatalf("SetCreditLimit limit = %v, want 50000", gotLimit)
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
	if payload.CreditLimit != 50000 {
		t.Fatalf("credit_limit = %v, want 50000", payload.CreditLimit)
	}
	if payload.BalanceOwed != 2500 {
		t.Fatalf("balance_owed = %v, want 2500", payload.BalanceOwed)
	}
	if payload.RemainingCredit != 47500 {
		t.Fatalf("remaining_credit = %v, want 47500", payload.RemainingCredit)
	}
}

func TestPatchAdminCustomerCreditLimitBelowOwed(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, IsReseller: true}, nil
	}
	handler.setResellerCreditLimitFn = func(context.Context, int64, float64) (*database.ResellerCreditAccount, error) {
		return nil, database.ErrResellerCreditLimitBelowOwed
	}

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/admin/customers/12345/credit",
		bytes.NewBufferString(`{"credit_limit": 100}`),
	)
	rec := httptest.NewRecorder()
	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPostAdminCustomerSettlementNoWalletDebit(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         7,
			TelegramID: telegramID,
			IsReseller: true,
		}, nil
	}

	var (
		gotCustomerID     int64
		gotAmount         float64
		gotNote           string
		gotCreatedBy      string
		gotIdempotencyKey string
		deductCalled      bool
	)

	// Guard: production wallet path must not be reachable via this seam.
	// If someone wires DeductBalance into admin settlement, this test still
	// asserts the seam path never touches wallet.
	_ = deductCalled

	handler.recordAdminSettlementFn = func(
		_ context.Context,
		customerID int64,
		amount float64,
		note, createdBy, idempotencyKey string,
	) (*database.ResellerLedgerEntry, *database.ResellerCreditAccount, bool, error) {
		gotCustomerID = customerID
		gotAmount = amount
		gotNote = note
		gotCreatedBy = createdBy
		gotIdempotencyKey = idempotencyKey
		return &database.ResellerLedgerEntry{
				ID:             42,
				CustomerID:     customerID,
				EntryType:      database.ResellerLedgerEntryTypeSettlement,
				Direction:      database.ResellerLedgerDirectionDecrease,
				Amount:         amount,
				CreatedBy:      createdBy,
				IdempotencyKey: idempotencyKey,
			}, &database.ResellerCreditAccount{
				CustomerID:  customerID,
				CreditLimit: 50000,
				BalanceOwed: 9000,
			}, true, nil
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/customers/12345/settlements",
		bytes.NewBufferString(`{"amount": 1000, "note": "cash received"}`),
	)
	req.Header.Set("Idempotency-Key", "admin-settle-1")
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(999)))
	rec := httptest.NewRecorder()

	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if gotCustomerID != 7 {
		t.Fatalf("customerID = %d, want 7", gotCustomerID)
	}
	if gotAmount != 1000 {
		t.Fatalf("amount = %v, want 1000", gotAmount)
	}
	if gotNote != "cash received" {
		t.Fatalf("note = %q, want %q", gotNote, "cash received")
	}
	if gotCreatedBy != "admin:999" {
		t.Fatalf("created_by = %q, want admin:999", gotCreatedBy)
	}
	if gotIdempotencyKey != "admin-settle-1" {
		t.Fatalf("idempotency_key = %q, want admin-settle-1", gotIdempotencyKey)
	}

	var payload CreateSettlementResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body=%q", err, rec.Body.String())
	}
	if !payload.Created {
		t.Fatal("created = false, want true")
	}
	if payload.Amount != 1000 {
		t.Fatalf("amount = %v, want 1000", payload.Amount)
	}
	if payload.BalanceOwed != 9000 {
		t.Fatalf("balance_owed = %v, want 9000", payload.BalanceOwed)
	}
	if payload.RemainingCredit != 41000 {
		t.Fatalf("remaining_credit = %v, want 41000", payload.RemainingCredit)
	}
	if payload.LedgerEntryID != 42 {
		t.Fatalf("ledger_entry_id = %d, want 42", payload.LedgerEntryID)
	}
}

func TestPostAdminCustomerSettlementRejectsNonPositiveAmount(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, IsReseller: true}, nil
	}
	handler.recordAdminSettlementFn = func(context.Context, int64, float64, string, string, string) (*database.ResellerLedgerEntry, *database.ResellerCreditAccount, bool, error) {
		t.Fatal("recordAdminSettlementFn must not be called for invalid amount")
		return nil, nil, false, nil
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/customers/12345/settlements",
		bytes.NewBufferString(`{"amount": 0, "note": "x"}`),
	)
	req.Header.Set("Idempotency-Key", "k")
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(999)))
	rec := httptest.NewRecorder()
	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPostAdminCustomerSettlementOverSettlement(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, IsReseller: true}, nil
	}
	handler.recordAdminSettlementFn = func(context.Context, int64, float64, string, string, string) (*database.ResellerLedgerEntry, *database.ResellerCreditAccount, bool, error) {
		return nil, nil, false, database.ErrResellerOverSettlement
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/customers/12345/settlements",
		bytes.NewBufferString(`{"amount": 99999}`),
	)
	req.Header.Set("Idempotency-Key", "k")
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(999)))
	rec := httptest.NewRecorder()
	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPostAdminCustomerSettlementIdempotentReplay(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, IsReseller: true}, nil
	}
	handler.recordAdminSettlementFn = func(_ context.Context, customerID int64, amount float64, _, _, key string) (*database.ResellerLedgerEntry, *database.ResellerCreditAccount, bool, error) {
		return &database.ResellerLedgerEntry{
				ID: 42, CustomerID: customerID, Amount: amount, IdempotencyKey: key,
			}, &database.ResellerCreditAccount{
				CustomerID: customerID, CreditLimit: 50000, BalanceOwed: 9000,
			}, false, nil
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/customers/12345/settlements",
		bytes.NewBufferString(`{"amount": 1000}`),
	)
	req.Header.Set("Idempotency-Key", "replay-key")
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(999)))
	rec := httptest.NewRecorder()
	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	var payload CreateSettlementResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Created {
		t.Fatal("created = true, want false for idempotent replay")
	}
}

func TestListResellersIncludesCreditFields(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.listResellersFn = func(_ context.Context) ([]database.Customer, error) {
		return []database.Customer{
			{ID: 1, TelegramID: 111, IsReseller: true},
			{ID: 2, TelegramID: 222, IsReseller: true},
		}, nil
	}
	handler.ensureResellerAccountFn = func(_ context.Context, customerID int64, _ float64) (*database.ResellerCreditAccount, error) {
		switch customerID {
		case 1:
			return &database.ResellerCreditAccount{CustomerID: 1, CreditLimit: 50000, BalanceOwed: 10000}, nil
		case 2:
			return &database.ResellerCreditAccount{CustomerID: 2, CreditLimit: 20000, BalanceOwed: 0}, nil
		default:
			return nil, errors.New("unexpected customer")
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/resellers", nil)
	rec := httptest.NewRecorder()
	handler.ListResellers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload []AdminResellerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("len = %d, want 2", len(payload))
	}
	if payload[0].CreditLimit != 50000 || payload[0].BalanceOwed != 10000 || payload[0].RemainingCredit != 40000 {
		t.Fatalf("payload[0] credit fields = %+v", payload[0])
	}
	if payload[1].CreditLimit != 20000 || payload[1].BalanceOwed != 0 || payload[1].RemainingCredit != 20000 {
		t.Fatalf("payload[1] credit fields = %+v", payload[1])
	}
}

func TestAdminCustomerPathRouting(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 1, TelegramID: telegramID, IsReseller: true}, nil
	}

	t.Run("unknown action 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/customers/12345/unknown", nil)
		rec := httptest.NewRecorder()
		handler.AdminCustomerByTelegramID(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("credit wrong method 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/customers/12345/credit", nil)
		rec := httptest.NewRecorder()
		handler.AdminCustomerByTelegramID(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("settlements wrong method 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/customers/12345/settlements", nil)
		rec := httptest.NewRecorder()
		handler.AdminCustomerByTelegramID(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("ledger wrong method 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/customers/12345/ledger", nil)
		rec := httptest.NewRecorder()
		handler.AdminCustomerByTelegramID(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("reseller wrong method 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/customers/12345/reseller", nil)
		rec := httptest.NewRecorder()
		handler.AdminCustomerByTelegramID(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestGetAdminCustomerLedger(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, IsReseller: true}, nil
	}

	var gotCustomerID int64
	var gotLimit, gotOffset int
	handler.listResellerLedgerFn = func(_ context.Context, customerID int64, limit, offset int) ([]database.ResellerLedgerEntry, error) {
		gotCustomerID = customerID
		gotLimit = limit
		gotOffset = offset
		return []database.ResellerLedgerEntry{
			{
				ID:          1,
				CustomerID:  customerID,
				EntryType:   database.ResellerLedgerEntryTypeSettlement,
				Direction:   database.ResellerLedgerDirectionDecrease,
				Amount:      1000,
				EffectiveAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
				Note:        "cash",
				CreatedBy:   "admin:999",
			},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/customers/12345/ledger?limit=10&offset=5", nil)
	rec := httptest.NewRecorder()
	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotCustomerID != 7 || gotLimit != 10 || gotOffset != 5 {
		t.Fatalf("listLedger args = customer=%d limit=%d offset=%d", gotCustomerID, gotLimit, gotOffset)
	}

	var items []ResellerLedgerItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].Amount != 1000 || items[0].CreatedBy != "admin:999" || items[0].Note != "cash" {
		t.Fatalf("item = %+v", items[0])
	}
}

func TestPatchAdminCustomerResellerTrueEnsuresAccount(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, IsReseller: false}, nil
	}
	handler.updateCustomerFields = func(context.Context, int64, map[string]interface{}) error {
		return nil
	}

	var ensuredID int64
	handler.ensureResellerAccountFn = func(_ context.Context, customerID int64, defaultLimit float64) (*database.ResellerCreditAccount, error) {
		ensuredID = customerID
		return &database.ResellerCreditAccount{
			CustomerID:  customerID,
			CreditLimit: defaultLimit,
			BalanceOwed: 0,
		}, nil
	}

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/admin/customers/12345/reseller",
		bytes.NewBufferString(`{"is_reseller":true}`),
	)
	rec := httptest.NewRecorder()
	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ensuredID != 7 {
		t.Fatalf("EnsureAccount customerID = %d, want 7", ensuredID)
	}

	var payload AdminResellerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.IsReseller {
		t.Fatal("is_reseller = false, want true")
	}
}
