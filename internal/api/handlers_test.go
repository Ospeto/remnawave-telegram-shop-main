package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/translation"
)

func TestUploadScreenshotFailureResponsePreservesVerificationReason(t *testing.T) {
	result := &payment.VerificationResult{
		Success:   false,
		Reason:    "Screenshot verification is temporarily unavailable right now. Please try again later or contact support.",
		ReasonKey: "mobile_pay_failed_provider_auth",
	}

	resp := uploadScreenshotFailureResponse(result)

	if resp.Status != "failed" {
		t.Fatalf("uploadScreenshotFailureResponse() status = %q, want failed", resp.Status)
	}
	if resp.Message != result.Reason {
		t.Fatalf("uploadScreenshotFailureResponse() message = %q, want %q", resp.Message, result.Reason)
	}
	if resp.Reason != result.ReasonKey {
		t.Fatalf("uploadScreenshotFailureResponse() reason = %q, want %q", resp.Reason, result.ReasonKey)
	}
}

func TestPendingPurchaseConflictResponseCarriesExtendKeyID(t *testing.T) {
	tm := translation.GetInstance()
	if err := tm.InitTranslations(filepath.Join("..", "..", "translations"), "en"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}

	handler := NewAPIHandler(nil, nil, nil, tm, nil, nil, nil, nil, nil)
	extendKeyID := int64(77)

	resp := handler.pendingPurchaseConflictResponse(&database.Customer{
		Language: "en",
	}, &database.Purchase{
		ID:          55,
		InvoiceType: database.InvoiceTypeMobileBanking,
		Amount:      18200,
		Currency:    "MMK",
		ExtendKeyID: &extendKeyID,
	})

	if resp.PendingPurchase.ExtendKeyID == nil || *resp.PendingPurchase.ExtendKeyID != extendKeyID {
		t.Fatalf("pendingPurchaseConflictResponse() extend_key_id = %v, want %d", resp.PendingPurchase.ExtendKeyID, extendKeyID)
	}
	if !strings.Contains(strings.ToLower(resp.Message), "cancel") {
		t.Fatalf("pendingPurchaseConflictResponse() message = %q, want cancel guidance", resp.Message)
	}
}

func TestCompactSubscriptionKeysForDisplayCollapsesDuplicateIdentityBySubscriptionURL(t *testing.T) {
	expire := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	subURL := "https://example.com/sub/abc"
	keys := []database.SubscriptionKey{
		{
			ID:              1,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:              2,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 5, 0, 0, time.UTC),
		},
	}

	compacted := compactSubscriptionKeysForDisplay(keys)
	if len(compacted) != 1 {
		t.Fatalf("compactSubscriptionKeysForDisplay() len = %d, want 1", len(compacted))
	}
	if compacted[0].ID != 2 {
		t.Fatalf("compactSubscriptionKeysForDisplay() kept ID %d, want newest ID 2", compacted[0].ID)
	}
}

func TestCompactSubscriptionKeysForDisplayKeepsDistinctKeysEvenIfPlanShapeMatches(t *testing.T) {
	expire := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	keys := []database.SubscriptionKey{
		{
			ID:              10,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: "https://example.com/sub/key-10",
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:              11,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: "https://example.com/sub/key-11",
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 1, 0, 0, time.UTC),
		},
		{
			ID:              12,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: "https://example.com/sub/key-12",
			Status:          "active",
			TrafficLimitGB:  100,
			ExpireAt:        &expire,
			CreatedAt:       time.Date(2026, 4, 16, 10, 2, 0, 0, time.UTC),
		},
	}

	compacted := compactSubscriptionKeysForDisplay(keys)
	if len(compacted) != 3 {
		t.Fatalf("compactSubscriptionKeysForDisplay() len = %d, want 3", len(compacted))
	}
}

func TestCompactSubscriptionKeysForDisplayPrefersActiveRecordForSameIdentity(t *testing.T) {
	expireActive := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	expireExpired := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	subURL := "https://example.com/sub/xyz"

	keys := []database.SubscriptionKey{
		{
			ID:              20,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "expired",
			ExpireAt:        &expireExpired,
			CreatedAt:       time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:              21,
			RemnawaveUUID:   uuid.New(),
			SubscriptionURL: subURL,
			Status:          "active",
			ExpireAt:        &expireActive,
			CreatedAt:       time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
		},
	}

	compacted := compactSubscriptionKeysForDisplay(keys)
	if len(compacted) != 1 {
		t.Fatalf("compactSubscriptionKeysForDisplay() len = %d, want 1", len(compacted))
	}
	if compacted[0].Status != "active" || compacted[0].ID != 21 {
		t.Fatalf("compactSubscriptionKeysForDisplay() kept %q ID %d, want active ID 21", compacted[0].Status, compacted[0].ID)
	}
}

func TestBeginScreenshotVerificationRejectsExcessiveAttemptsByCustomer(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	customerID := int64(99)

	for i := 0; i < maxScreenshotVerificationsPerCustomer; i++ {
		purchaseID := int64(i + 1)
		if err := handler.beginScreenshotVerification(purchaseID, customerID); err != nil {
			t.Fatalf("beginScreenshotVerification() attempt %d error = %v", i+1, err)
		}
		handler.finishScreenshotVerification(purchaseID)
	}

	if err := handler.beginScreenshotVerification(999, customerID); err == nil {
		t.Fatal("beginScreenshotVerification() error = nil, want per-customer throttling")
	}
}

func TestWriteSanitizedErrorHidesWrappedDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSanitizedError(rec, http.StatusInternalServerError, "Verification unavailable", errors.New("upstream timeout: token=secret"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("writeSanitizedError() status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body := rec.Body.String(); body != "Verification unavailable\n" {
		t.Fatalf("writeSanitizedError() body = %q, want sanitized public message", body)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("writeSanitizedError() leaked wrapped error details")
	}
}

func captureSlogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return &buf
}

func TestUploadScreenshotLogsMissingTelegramAuthReject(t *testing.T) {
	logs := captureSlogOutput(t)
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload_screenshot?id=55", nil)
	rec := httptest.NewRecorder()

	handler.UploadScreenshot(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("UploadScreenshot() status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	assertUploadRejectLog(t, logs.String(), uploadRejectMissingTelegramAuth)
}

func TestUploadScreenshotLogsInvalidPurchaseIDReject(t *testing.T) {
	logs := captureSlogOutput(t)
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload_screenshot?id=bad", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(123456789)))
	rec := httptest.NewRecorder()

	handler.UploadScreenshot(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("UploadScreenshot() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	assertUploadRejectLog(t, logs.String(), uploadRejectInvalidPurchaseID)
}

func TestUploadScreenshotLogsPurchaseOwnerMismatchReject(t *testing.T) {
	logs := captureSlogOutput(t)
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.getPurchaseByID = func(context.Context, int64) (*database.Purchase, error) {
		return &database.Purchase{
			ID:          55,
			CustomerID:  99,
			Status:      database.PurchaseStatusPending,
			InvoiceType: database.InvoiceTypeMobileBanking,
		}, nil
	}
	handler.findCustomerByTelegramID = func(context.Context, int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         42,
			TelegramID: 123456789,
		}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload_screenshot?id=55", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(123456789)))
	rec := httptest.NewRecorder()

	handler.UploadScreenshot(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("UploadScreenshot() status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	assertUploadRejectLog(t, logs.String(), uploadRejectPurchaseOwnerMismatch)
	if strings.Contains(logs.String(), "123456789") {
		t.Fatalf("UploadScreenshot() log leaked full telegram id: %s", logs.String())
	}
}

func TestUploadScreenshotLogsMissingFileFieldReject(t *testing.T) {
	logs := captureSlogOutput(t)
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.getPurchaseByID = func(context.Context, int64) (*database.Purchase, error) {
		return &database.Purchase{
			ID:          55,
			CustomerID:  42,
			Status:      database.PurchaseStatusPending,
			InvoiceType: database.InvoiceTypeMobileBanking,
		}, nil
	}
	handler.findCustomerByTelegramID = func(context.Context, int64) (*database.Customer, error) {
		return &database.Customer{
			ID:         42,
			TelegramID: 123456789,
		}, nil
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("not_file", "receipt"); err != nil {
		t.Fatalf("WriteField() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload_screenshot?id=55", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(123456789)))
	rec := httptest.NewRecorder()

	handler.UploadScreenshot(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("UploadScreenshot() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	assertUploadRejectLog(t, logs.String(), uploadRejectMissingFileField)
}

func assertUploadRejectLog(t *testing.T, logOutput string, reason string) {
	t.Helper()
	for _, want := range []string{
		"event=upload_screenshot_rejected",
		"reason=" + reason,
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output missing %q:\n%s", want, logOutput)
		}
	}
}

func TestPromoValidationStatus(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	if got := promoValidationStatus(nil, errors.New("db down"), now); got != http.StatusServiceUnavailable {
		t.Fatalf("promoValidationStatus() lookup error = %d, want %d", got, http.StatusServiceUnavailable)
	}

	if got := promoValidationStatus(nil, nil, now); got != http.StatusNotFound {
		t.Fatalf("promoValidationStatus() missing promo = %d, want %d", got, http.StatusNotFound)
	}

	if got := promoValidationStatus(&database.PromoCode{
		MaxUses:    3,
		UsedCount:  3,
		ValidUntil: now.Add(time.Hour),
	}, nil, now); got != http.StatusNotFound {
		t.Fatalf("promoValidationStatus() exhausted promo = %d, want %d", got, http.StatusNotFound)
	}

	if got := promoValidationStatus(&database.PromoCode{
		MaxUses:    5,
		UsedCount:  1,
		ValidUntil: now.Add(time.Hour),
	}, nil, now); got != http.StatusOK {
		t.Fatalf("promoValidationStatus() valid promo = %d, want %d", got, http.StatusOK)
	}
}

func TestUpdateAutoRenewReturnsGone(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/wallet/autorenew", strings.NewReader(`{"enabled":true,"duration":30}`))
	rec := httptest.NewRecorder()

	handler.UpdateAutoRenew(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("UpdateAutoRenew() status = %d, want %d", rec.Code, http.StatusGone)
	}
	if !strings.Contains(rec.Body.String(), "Customer-wide auto-renew has been removed") {
		t.Fatalf("UpdateAutoRenew() body = %q, want deprecation message", rec.Body.String())
	}
}

func TestValidateScreenshotUploadAccessRejectsCryptoPurchase(t *testing.T) {
	purchase := &database.Purchase{
		ID:          10,
		CustomerID:  5,
		Status:      database.PurchaseStatusPending,
		InvoiceType: database.InvoiceTypeCrypto,
	}
	customer := &database.Customer{
		ID:         5,
		TelegramID: 42,
	}

	status, message, reason, ok := validateScreenshotUploadAccess(purchase, customer, 42)

	if ok {
		t.Fatal("validateScreenshotUploadAccess() ok = true, want false for crypto purchase")
	}
	if status != http.StatusConflict {
		t.Fatalf("validateScreenshotUploadAccess() status = %d, want %d", status, http.StatusConflict)
	}
	if reason != uploadRejectUnsupportedInvoiceType {
		t.Fatalf("validateScreenshotUploadAccess() reason = %q, want %q", reason, uploadRejectUnsupportedInvoiceType)
	}
	if !strings.Contains(strings.ToLower(message), "does not accept screenshot") {
		t.Fatalf("validateScreenshotUploadAccess() message = %q, want screenshot rejection", message)
	}
}

func TestValidatePendingPurchaseCancellationAccess(t *testing.T) {
	customer := &database.Customer{
		ID:         5,
		TelegramID: 42,
	}

	t.Run("allows customer pending screenshot purchase", func(t *testing.T) {
		status, message, ok := validatePendingPurchaseCancellationAccess(&database.Purchase{
			ID:          10,
			CustomerID:  5,
			Status:      database.PurchaseStatusPending,
			InvoiceType: database.InvoiceTypeMobileBanking,
		}, customer, 42)

		if !ok {
			t.Fatalf("validatePendingPurchaseCancellationAccess() ok = false, status = %d, message = %q", status, message)
		}
	})

	t.Run("rejects another customer", func(t *testing.T) {
		status, _, ok := validatePendingPurchaseCancellationAccess(&database.Purchase{
			ID:          10,
			CustomerID:  6,
			Status:      database.PurchaseStatusPending,
			InvoiceType: database.InvoiceTypeMobileBanking,
		}, customer, 42)

		if ok {
			t.Fatal("validatePendingPurchaseCancellationAccess() ok = true, want false for another customer")
		}
		if status != http.StatusNotFound {
			t.Fatalf("validatePendingPurchaseCancellationAccess() status = %d, want %d", status, http.StatusNotFound)
		}
	})

	t.Run("rejects paid screenshot purchase", func(t *testing.T) {
		status, _, ok := validatePendingPurchaseCancellationAccess(&database.Purchase{
			ID:          10,
			CustomerID:  5,
			Status:      database.PurchaseStatusPaid,
			InvoiceType: database.InvoiceTypeMobileBanking,
		}, customer, 42)

		if ok {
			t.Fatal("validatePendingPurchaseCancellationAccess() ok = true, want false for paid purchase")
		}
		if status != http.StatusConflict {
			t.Fatalf("validatePendingPurchaseCancellationAccess() status = %d, want %d", status, http.StatusConflict)
		}
	})

	t.Run("rejects non screenshot purchase", func(t *testing.T) {
		status, _, ok := validatePendingPurchaseCancellationAccess(&database.Purchase{
			ID:          10,
			CustomerID:  5,
			Status:      database.PurchaseStatusPending,
			InvoiceType: database.InvoiceTypeWalletPayment,
		}, customer, 42)

		if ok {
			t.Fatal("validatePendingPurchaseCancellationAccess() ok = true, want false for wallet purchase")
		}
		if status != http.StatusConflict {
			t.Fatalf("validatePendingPurchaseCancellationAccess() status = %d, want %d", status, http.StatusConflict)
		}
	})
}

func TestScreenshotVerificationInFlight(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil)

	if handler.screenshotVerificationInFlight(55) {
		t.Fatal("screenshotVerificationInFlight() = true before verification starts")
	}
	if err := handler.beginScreenshotVerification(55, 42); err != nil {
		t.Fatalf("beginScreenshotVerification() error = %v", err)
	}
	if !handler.screenshotVerificationInFlight(55) {
		t.Fatal("screenshotVerificationInFlight() = false while verification is active")
	}
	handler.finishScreenshotVerification(55)
	if handler.screenshotVerificationInFlight(55) {
		t.Fatal("screenshotVerificationInFlight() = true after verification finishes")
	}
}
