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

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

func TestParsePaymentMethodPostpaid(t *testing.T) {
	got, err := parsePaymentMethod("postpaid")
	if err != nil || got != database.InvoiceTypePostpaid {
		t.Fatalf("got %q err %v, want %q", got, err, database.InvoiceTypePostpaid)
	}
	got, err = parsePaymentMethod(" PostPaid ")
	if err != nil || got != database.InvoiceTypePostpaid {
		t.Fatalf("trimmed/case got %q err %v", got, err)
	}
}

func TestCreatePurchasePostpaidNonResellerRejected(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() { config.SetPlans(original) })

	wholesale := 4000
	config.SetPlans([]config.Plan{
		{
			ID:             "pro",
			Label:          "Pro",
			Days:           30,
			Price:          5000,
			WholesalePrice: &wholesale,
			Active:         true,
		},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         1,
			TelegramID: telegramID,
			Language:   "en",
			IsReseller: false,
		}, nil
	}
	handler.createServicePurchase = func(context.Context, float64, int, int, *database.Customer, database.InvoiceType, string) (string, int64, error) {
		t.Fatal("createServicePurchase must not be called for non-reseller postpaid")
		return "", 0, nil
	}

	body := `{"plan_id":"pro","payment_method":"postpaid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/purchase", bytes.NewBufferString(body)).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.CreatePurchase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreatePurchase() status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "postpaid is only available for resellers") {
		t.Fatalf("CreatePurchase() body = %q, want non-reseller rejection", rec.Body.String())
	}
}

func TestCreatePurchasePostpaidResellerCallsCreateWithPostpaidType(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() { config.SetPlans(original) })

	wholesale := 4000
	config.SetPlans([]config.Plan{
		{
			ID:             "pro",
			Label:          "Pro",
			Days:           30,
			Price:          5000,
			WholesalePrice: &wholesale,
			TrafficLimitGB: 100,
			SortOrder:      0,
			Active:         true,
		},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         7,
			TelegramID: telegramID,
			Language:   "en",
			IsReseller: true,
		}, nil
	}

	var gotAmount float64
	var gotInvoiceType database.InvoiceType
	var gotTier string
	handler.createServicePurchase = func(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, invoiceType database.InvoiceType, promoCode string) (string, int64, error) {
		gotAmount = amount
		gotInvoiceType = invoiceType
		gotTier = payment.PricingTierFromContext(ctx)
		if days != 30 || trafficLimitGB != 100 {
			t.Fatalf("createServicePurchase days/traffic = %d/%d, want 30/100", days, trafficLimitGB)
		}
		if promoCode != "" {
			t.Fatalf("promoCode = %q, want empty", promoCode)
		}
		if customer == nil || customer.ID != 7 {
			t.Fatalf("customer = %+v, want id 7", customer)
		}
		return "", 99, nil
	}
	handler.getPurchaseByID = func(_ context.Context, id int64) (*database.Purchase, error) {
		if id != 99 {
			t.Fatalf("getPurchaseByID id = %d, want 99", id)
		}
		return &database.Purchase{
			ID:          99,
			Amount:      gotAmount,
			Currency:    "MMK",
			InvoiceType: database.InvoiceTypePostpaid,
			PlanLabel:   "Pro",
			PricingTier: gotTier,
		}, nil
	}

	body := `{"plan_id":"pro","payment_method":"postpaid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/purchase", bytes.NewBufferString(body)).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.CreatePurchase(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePurchase() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotInvoiceType != database.InvoiceTypePostpaid {
		t.Fatalf("invoiceType = %q, want postpaid", gotInvoiceType)
	}
	if gotAmount != 4000 {
		t.Fatalf("CreatePurchase charged amount = %v, want wholesale 4000", gotAmount)
	}
	if gotTier != config.PricingTierWholesale {
		t.Fatalf("CreatePurchase pricing tier context = %q, want %q", gotTier, config.PricingTierWholesale)
	}

	var resp CreatePurchaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body=%s", err, rec.Body.String())
	}
	if resp.Amount != 4000 {
		t.Fatalf("response amount = %d, want 4000", resp.Amount)
	}
	if resp.InvoiceType != string(database.InvoiceTypePostpaid) {
		t.Fatalf("response invoice_type = %q, want postpaid", resp.InvoiceType)
	}
	if resp.PricingTier != config.PricingTierWholesale {
		t.Fatalf("response pricing_tier = %q, want %q", resp.PricingTier, config.PricingTierWholesale)
	}
	if resp.Instructions != "" {
		t.Fatalf("postpaid response must not include mobile banking instructions, got %q", resp.Instructions)
	}
}

func TestCreatePurchasePostpaidInsufficientCreditMapped(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() { config.SetPlans(original) })

	config.SetPlans([]config.Plan{
		{ID: "pro", Label: "Pro", Days: 30, Price: 5000, Active: true},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 1, TelegramID: telegramID, Language: "en", IsReseller: true}, nil
	}
	handler.createServicePurchase = func(context.Context, float64, int, int, *database.Customer, database.InvoiceType, string) (string, int64, error) {
		return "", 0, fmt.Errorf("charge: %w", database.ErrResellerInsufficientCredit)
	}

	body := `{"plan_id":"pro","payment_method":"postpaid"}`
	req := httptest.NewRequest(http.MethodPost, "/api/purchase", bytes.NewBufferString(body)).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	handler.CreatePurchase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Insufficient reseller credit") {
		t.Fatalf("body = %q, want insufficient credit message", rec.Body.String())
	}
}

func TestCreatePurchasePostpaidPaymentLayerErrorsMapped(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() { config.SetPlans(original) })
	config.SetPlans([]config.Plan{
		{ID: "pro", Label: "Pro", Days: 30, Price: 5000, Active: true},
	})

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "non-reseller from payment",
			err:        errors.New("postpaid is only available for resellers"),
			wantStatus: http.StatusBadRequest,
			wantBody:   "postpaid is only available for resellers",
		},
		{
			name:       "amount not positive",
			err:        errors.New("postpaid amount must be positive"),
			wantStatus: http.StatusBadRequest,
			wantBody:   "postpaid amount must be positive",
		},
		{
			name:       "nil repo sanitized 500",
			err:        errors.New("reseller credit repository is not configured"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create purchase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
				return &database.Customer{ID: 1, TelegramID: telegramID, Language: "en", IsReseller: true}, nil
			}
			handler.createServicePurchase = func(context.Context, float64, int, int, *database.Customer, database.InvoiceType, string) (string, int64, error) {
				return "", 0, tt.err
			}

			body := `{"plan_id":"pro","payment_method":"postpaid"}`
			req := httptest.NewRequest(http.MethodPost, "/api/purchase", bytes.NewBufferString(body)).
				WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
			rec := httptest.NewRecorder()
			handler.CreatePurchase(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body=%q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want substring %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRegisterHandlersProtectsResellerRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "account", method: http.MethodGet, path: "/api/reseller/account"},
		{name: "ledger", method: http.MethodGet, path: "/api/reseller/ledger"},
		{name: "settlements", method: http.MethodPost, path: "/api/reseller/settlements"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://example.com"+tt.path, bytes.NewBufferString(`{"amount":1,"payment_method":"wallet"}`))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestMapCreatePurchasePostpaidError(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantOK     bool
	}{
		{database.ErrResellerInsufficientCredit, http.StatusBadRequest, true},
		{fmt.Errorf("wrap: %w", database.ErrResellerInsufficientCredit), http.StatusBadRequest, true},
		{errors.New("postpaid is only available for resellers"), http.StatusBadRequest, true},
		{errors.New("postpaid amount must be positive"), http.StatusBadRequest, true},
		{errors.New("something else"), 0, false},
	}
	for _, tt := range tests {
		status, _, ok := mapCreatePurchasePostpaidError(tt.err)
		if ok != tt.wantOK || (ok && status != tt.wantStatus) {
			t.Fatalf("err=%v -> status=%d ok=%v, want status=%d ok=%v", tt.err, status, ok, tt.wantStatus, tt.wantOK)
		}
	}
}

// selfSettlementHandlerWithNilPoolDeps returns a handler whose repo pointers are
// non-nil so CreateResellerSettlement can exercise early validation (amount /
// idempotency) without a live Postgres pool.
func selfSettlementHandlerWithNilPoolDeps() *APIHandler {
	handler := NewAPIHandler(
		database.NewCustomerRepository(nil),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	handler.SetResellerCreditDeps(
		database.NewResellerCreditRepository(nil),
		database.NewWalletTransactionRepository(nil),
	)
	return handler
}

func TestCreateResellerSettlementRequiresIdempotencyKey(t *testing.T) {
	handler := selfSettlementHandlerWithNilPoolDeps()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/reseller/settlements",
		bytes.NewBufferString(`{"amount":1000,"payment_method":"wallet"}`),
	)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.CreateResellerSettlement(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Idempotency-Key") && !strings.Contains(rec.Body.String(), "idempotency_key") {
		t.Fatalf("body = %q, want idempotency key required message", rec.Body.String())
	}
}

func TestCreateResellerSettlementRejectsNonPositiveAmount(t *testing.T) {
	handler := selfSettlementHandlerWithNilPoolDeps()

	for _, body := range []string{
		`{"amount":0,"payment_method":"wallet","idempotency_key":"k"}`,
		`{"amount":-1,"payment_method":"wallet","idempotency_key":"k"}`,
		`{"amount":0.001,"payment_method":"wallet","idempotency_key":"k"}`, // rounds to 0
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/reseller/settlements", bytes.NewBufferString(body))
		req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
		rec := httptest.NewRecorder()
		handler.CreateResellerSettlement(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d want 400 body=%q", body, rec.Code, rec.Body.String())
		}
	}
}

func TestCreateResellerSettlementRejectsNonWalletMethod(t *testing.T) {
	handler := selfSettlementHandlerWithNilPoolDeps()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/reseller/settlements",
		bytes.NewBufferString(`{"amount":1000,"payment_method":"cash","idempotency_key":"k"}`),
	)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	handler.CreateResellerSettlement(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%q", rec.Code, rec.Body.String())
	}
}

func TestPostAdminCustomerSettlementRequiresIdempotencyKey(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 7, TelegramID: telegramID, IsReseller: true}, nil
	}
	handler.recordAdminSettlementFn = func(context.Context, int64, float64, string, string, string) (*database.ResellerLedgerEntry, *database.ResellerCreditAccount, bool, error) {
		t.Fatal("recordAdminSettlementFn must not be called without idempotency key")
		return nil, nil, false, nil
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/customers/12345/settlements",
		bytes.NewBufferString(`{"amount": 1000}`),
	)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(999)))
	rec := httptest.NewRecorder()
	handler.AdminCustomerByTelegramID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
