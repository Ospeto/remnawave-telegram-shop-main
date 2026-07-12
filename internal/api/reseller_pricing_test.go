package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

func intPtr(v int) *int { return &v }

func TestGetMeReturnsIsReseller(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         1,
			TelegramID: telegramID,
			Language:   "en",
			IsReseller: true,
		}, nil
	}
	handler.getCachedSyncKeys = func(int64) ([]syncKeyStats, bool) { return nil, false }
	handler.triggerSyncKeys = func(context.Context, int64, int64) {}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.GetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMe() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_reseller":true`) {
		t.Fatalf("GetMe() body = %s, want is_reseller=true", rec.Body.String())
	}
}

func TestGetPlansPublicNeverLeaksWholesale(t *testing.T) {
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
			TrafficLimitGB: 0,
			SortOrder:      0,
			Active:         true,
		},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/plans", nil)
	rec := httptest.NewRecorder()

	handler.GetPlans(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetPlans() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := rec.Body.String()
	if strings.Contains(body, "wholesale_price") {
		t.Fatalf("GetPlans() public body leaked wholesale_price: %s", body)
	}
	if strings.Contains(body, `"pricing_tier"`) {
		t.Fatalf("GetPlans() public body should omit pricing_tier: %s", body)
	}

	var payload []PlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body=%s", err, body)
	}
	if len(payload) != 1 {
		t.Fatalf("GetPlans() len = %d, want 1", len(payload))
	}
	if payload[0].Price != 5000 {
		t.Fatalf("GetPlans() price = %d, want retail 5000", payload[0].Price)
	}
	if payload[0].PricingTier != "" {
		t.Fatalf("GetPlans() pricing_tier = %q, want empty", payload[0].PricingTier)
	}
}

func TestGetPlansResellerSeesEffectiveWholesalePrice(t *testing.T) {
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
			TrafficLimitGB: 0,
			SortOrder:      0,
			Active:         true,
		},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         1,
			TelegramID: telegramID,
			Language:   "en",
			IsReseller: true,
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/plans", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.GetPlans(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetPlans() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload []PlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v body=%s", err, rec.Body.String())
	}
	if len(payload) != 1 {
		t.Fatalf("GetPlans() len = %d, want 1", len(payload))
	}
	if payload[0].Price != 4000 {
		t.Fatalf("GetPlans() price = %d, want wholesale 4000", payload[0].Price)
	}
	if payload[0].PricingTier != config.PricingTierWholesale {
		t.Fatalf("GetPlans() pricing_tier = %q, want %q", payload[0].PricingTier, config.PricingTierWholesale)
	}
	if strings.Contains(rec.Body.String(), "wholesale_price") {
		t.Fatalf("GetPlans() must not expose wholesale_price field: %s", rec.Body.String())
	}
}

func TestCreatePurchaseResellerChargesWholesaleAndReturnsTier(t *testing.T) {
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
	var gotTier string
	handler.createServicePurchase = func(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, invoiceType database.InvoiceType, promoCode string) (string, int64, error) {
		gotAmount = amount
		gotTier = payment.PricingTierFromContext(ctx)
		if days != 30 || trafficLimitGB != 100 {
			t.Fatalf("createServicePurchase days/traffic = %d/%d, want 30/100", days, trafficLimitGB)
		}
		if invoiceType != database.InvoiceTypeWalletPayment {
			t.Fatalf("invoiceType = %q, want wallet_payment", invoiceType)
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
			ID:           99,
			Amount:       gotAmount,
			Currency:     "MMK",
			InvoiceType:  database.InvoiceTypeWalletPayment,
			PlanLabel:    "Pro",
			PricingTier: gotTier,
		}, nil
	}

	body := `{"plan_id":"pro","payment_method":"wallet"}`
	req := httptest.NewRequest(http.MethodPost, "/api/purchase", bytes.NewBufferString(body)).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.CreatePurchase(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("CreatePurchase() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
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
	if resp.PricingTier != config.PricingTierWholesale {
		t.Fatalf("response pricing_tier = %q, want %q", resp.PricingTier, config.PricingTierWholesale)
	}
}

func TestCreatePurchaseResellerWithPromoRejected(t *testing.T) {
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
			IsReseller: true,
		}, nil
	}
	handler.createServicePurchase = func(context.Context, float64, int, int, *database.Customer, database.InvoiceType, string) (string, int64, error) {
		t.Fatal("createServicePurchase must not be called when reseller+promo")
		return "", 0, nil
	}

	body := `{"plan_id":"pro","payment_method":"mobile_banking","promo_code":"SAVE10"}`
	req := httptest.NewRequest(http.MethodPost, "/api/purchase", bytes.NewBufferString(body)).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.CreatePurchase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreatePurchase() status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Reseller pricing cannot combine with promo codes") {
		t.Fatalf("CreatePurchase() body = %q, want reseller+promo rejection message", rec.Body.String())
	}
}

func TestValidatePromoResellerRejected(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         1,
			TelegramID: telegramID,
			Language:   "en",
			IsReseller: true,
		}, nil
	}
	handler.findPromoByCode = func(context.Context, string) (*database.PromoCode, error) {
		t.Fatal("findPromoByCode must not be called for reseller")
		return nil, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/promo/validate?code=SAVE10", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.ValidatePromo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ValidatePromo() status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Reseller pricing cannot combine with promo codes") {
		t.Fatalf("ValidatePromo() body = %q, want reseller rejection message", rec.Body.String())
	}
}

func TestValidatePromoNonResellerStillWorks(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.findCustomerByTelegramID = func(_ context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         1,
			TelegramID: telegramID,
			Language:   "en",
			IsReseller: false,
		}, nil
	}
	handler.findPromoByCode = func(_ context.Context, code string) (*database.PromoCode, error) {
		return &database.PromoCode{
			Code:            code,
			DiscountPercent: 10,
			MaxUses:         100,
			UsedCount:       0,
			ValidUntil:      time.Now().Add(24 * time.Hour),
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/promo/validate?code=SAVE10", nil).
		WithContext(context.WithValue(context.Background(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()

	handler.ValidatePromo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ValidatePromo() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"valid":true`) {
		t.Fatalf("ValidatePromo() body = %s, want valid=true", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"discount_percent":10`) {
		t.Fatalf("ValidatePromo() body = %s, want discount_percent=10", rec.Body.String())
	}
}
