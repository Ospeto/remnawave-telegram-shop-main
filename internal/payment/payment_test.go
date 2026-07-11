package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/gemini"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
)

func TestCanonicalCustomerSubscriptionStatePrefersLatestExpiryKey(t *testing.T) {
	now := time.Now()
	earlier := now.Add(24 * time.Hour)
	later := now.Add(72 * time.Hour)

	link, expireAt := canonicalCustomerSubscriptionState([]database.SubscriptionKey{
		{
			ID:              1,
			SubscriptionURL: "https://example.com/early",
			ExpireAt:        &earlier,
			Status:          "active",
			CreatedAt:       now.Add(-2 * time.Hour),
		},
		{
			ID:              2,
			SubscriptionURL: "https://example.com/late",
			ExpireAt:        &later,
			Status:          "active",
			CreatedAt:       now.Add(-1 * time.Hour),
		},
	}, "https://example.com/fallback", now.Add(12*time.Hour))

	if link == nil || *link != "https://example.com/late" {
		t.Fatalf("canonicalCustomerSubscriptionState() link = %v, want latest key URL", link)
	}
	if expireAt == nil || !expireAt.Equal(later) {
		t.Fatalf("canonicalCustomerSubscriptionState() expireAt = %v, want %v", expireAt, later)
	}
}

func TestCanonicalCustomerSubscriptionStateUsesFallbackWhenKeyMissing(t *testing.T) {
	fallbackExpireAt := time.Now().Add(48 * time.Hour)

	link, expireAt := canonicalCustomerSubscriptionState([]database.SubscriptionKey{
		{
			ID:        1,
			Status:    "deleted",
			CreatedAt: time.Now(),
		},
	}, "https://example.com/fallback", fallbackExpireAt)

	if link == nil || *link != "https://example.com/fallback" {
		t.Fatalf("canonicalCustomerSubscriptionState() link = %v, want fallback URL", link)
	}
	if expireAt == nil || !expireAt.Equal(fallbackExpireAt) {
		t.Fatalf("canonicalCustomerSubscriptionState() expireAt = %v, want fallback expiry", expireAt)
	}
}

func TestCustomerForPostPurchaseNotificationsFallsBackToOriginal(t *testing.T) {
	original := &database.Customer{ID: 1, TelegramID: 123, Language: "en"}

	if got := customerForPostPurchaseNotifications(original, nil); got != original {
		t.Fatalf("customerForPostPurchaseNotifications() = %v, want original customer", got)
	}

	refreshed := &database.Customer{ID: 1, TelegramID: 456, Language: "my"}
	if got := customerForPostPurchaseNotifications(original, refreshed); got != refreshed {
		t.Fatalf("customerForPostPurchaseNotifications() = %v, want refreshed customer", got)
	}
}

type fakeExtendedSubscriptionKeyStore struct {
	calls              []string
	expiryErr          error
	canceledCtxSeen    bool
	keyID              int64
	expireAt           time.Time
	subscriptionURL    string
	trafficLimitGB     int
	renewalPlanDays    int
	renewalPlanTraffic int
}

func (f *fakeExtendedSubscriptionKeyStore) recordCall(ctx context.Context, name string) {
	f.calls = append(f.calls, name)
	if ctx.Err() != nil {
		f.canceledCtxSeen = true
	}
}

func (f *fakeExtendedSubscriptionKeyStore) UpdateExpiry(ctx context.Context, id int64, expireAt time.Time) error {
	f.recordCall(ctx, "expiry")
	f.keyID = id
	f.expireAt = expireAt
	return f.expiryErr
}

func (f *fakeExtendedSubscriptionKeyStore) UpdateSubscriptionURL(ctx context.Context, id int64, url string) error {
	f.recordCall(ctx, "url")
	f.keyID = id
	f.subscriptionURL = url
	return nil
}

func (f *fakeExtendedSubscriptionKeyStore) UpdateTrafficLimit(ctx context.Context, id int64, trafficLimitGB int) error {
	f.recordCall(ctx, "traffic")
	f.keyID = id
	f.trafficLimitGB = trafficLimitGB
	return nil
}

func (f *fakeExtendedSubscriptionKeyStore) UpdateAutoRenewPlan(ctx context.Context, keyID int64, days int, planTrafficGB int) error {
	f.recordCall(ctx, "renewal_plan")
	f.keyID = keyID
	f.renewalPlanDays = days
	f.renewalPlanTraffic = planTrafficGB
	return nil
}

func TestPersistExtendedSubscriptionKeyRequiresExpiryWrite(t *testing.T) {
	expireAt := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)
	store := &fakeExtendedSubscriptionKeyStore{expiryErr: errors.New("database unavailable")}

	err := persistExtendedSubscriptionKey(context.Background(), store, 77, &remapi.User{
		ExpireAt:          expireAt,
		SubscriptionUrl:   "https://sub.example.com/key",
		TrafficLimitBytes: remapi.NewOptInt(1000),
	}, 30, 50, 100)
	if err == nil || !strings.Contains(err.Error(), "expiry") {
		t.Fatalf("persistExtendedSubscriptionKey() error = %v, want expiry persistence error", err)
	}
	if got := strings.Join(store.calls, ","); got != "expiry" {
		t.Fatalf("persistExtendedSubscriptionKey() calls = %q, want expiry only", got)
	}
}

func TestPersistExtendedSubscriptionKeyUsesContextWithoutCancel(t *testing.T) {
	expireAt := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)
	store := &fakeExtendedSubscriptionKeyStore{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := persistExtendedSubscriptionKey(ctx, store, 77, &remapi.User{
		ExpireAt:        expireAt,
		SubscriptionUrl: "https://sub.example.com/key",
	}, 30, 50, 100)
	if err != nil {
		t.Fatalf("persistExtendedSubscriptionKey() error = %v, want nil", err)
	}
	if store.canceledCtxSeen {
		t.Fatal("persistExtendedSubscriptionKey() used a canceled context for local persistence")
	}
	if got := strings.Join(store.calls, ","); got != "expiry,url,traffic,renewal_plan" {
		t.Fatalf("persistExtendedSubscriptionKey() calls = %q, want all persistence calls", got)
	}
	if store.keyID != 77 || !store.expireAt.Equal(expireAt) || store.subscriptionURL != "https://sub.example.com/key" || store.trafficLimitGB != 100 {
		t.Fatalf("persistExtendedSubscriptionKey() persisted wrong values: %+v", store)
	}
	if store.renewalPlanDays != 30 || store.renewalPlanTraffic != 50 {
		t.Fatalf("persistExtendedSubscriptionKey() renewal plan = %d/%d, want 30/50", store.renewalPlanDays, store.renewalPlanTraffic)
	}
}

func TestValidatePromoForPurchase(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		promo   *database.PromoCode
		wantErr error
	}{
		{
			name:    "missing promo is invalid",
			promo:   nil,
			wantErr: ErrInvalidPromoCode,
		},
		{
			name: "expired promo is invalid",
			promo: &database.PromoCode{
				ID:         1,
				MaxUses:    5,
				UsedCount:  1,
				ValidUntil: now.Add(-time.Minute),
			},
			wantErr: ErrInvalidPromoCode,
		},
		{
			name: "exhausted promo is invalid",
			promo: &database.PromoCode{
				ID:         2,
				MaxUses:    3,
				UsedCount:  3,
				ValidUntil: now.Add(time.Hour),
			},
			wantErr: ErrInvalidPromoCode,
		},
		{
			name: "available promo is valid",
			promo: &database.PromoCode{
				ID:         3,
				MaxUses:    10,
				UsedCount:  2,
				ValidUntil: now.Add(time.Hour),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePromoForPurchase(tt.promo, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validatePromoForPurchase() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyDiscountPercentRoundsToNearestWholeAmount(t *testing.T) {
	if got := applyDiscountPercent(9999, 15); got != 8499 {
		t.Fatalf("applyDiscountPercent() = %.0f, want 8499", got)
	}
}

func TestSupportsScreenshotVerification(t *testing.T) {
	tests := []struct {
		name        string
		invoiceType database.InvoiceType
		want        bool
	}{
		{name: "mobile banking", invoiceType: database.InvoiceTypeMobileBanking, want: true},
		{name: "wallet topup", invoiceType: database.InvoiceTypeWalletTopUp, want: true},
		{name: "crypto", invoiceType: database.InvoiceTypeCrypto, want: false},
		{name: "wallet payment", invoiceType: database.InvoiceTypeWalletPayment, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsScreenshotVerification(tt.invoiceType); got != tt.want {
				t.Fatalf("SupportsScreenshotVerification(%q) = %v, want %v", tt.invoiceType, got, tt.want)
			}
		})
	}
}

func TestTriggersReferralConversion(t *testing.T) {
	tests := []struct {
		name        string
		invoiceType database.InvoiceType
		amount      float64
		want        bool
	}{
		// A5: zero/free must not mint referral bonuses even for wallet_payment / mobile types.
		{name: "free zero wallet payment", invoiceType: database.InvoiceTypeWalletPayment, amount: 0, want: false},
		{name: "free zero mobile banking", invoiceType: database.InvoiceTypeMobileBanking, amount: 0, want: false},
		{name: "negative amount wallet payment", invoiceType: database.InvoiceTypeWalletPayment, amount: -1, want: false},
		// Paid paths still convert.
		{name: "paid mobile banking service purchase", invoiceType: database.InvoiceTypeMobileBanking, amount: 12000, want: true},
		{name: "paid wallet service purchase", invoiceType: database.InvoiceTypeWalletPayment, amount: 5000, want: true},
		// Top-up never converts (cash in, not service purchase).
		{name: "wallet topup", invoiceType: database.InvoiceTypeWalletTopUp, amount: 30000, want: false},
		{name: "unknown", invoiceType: database.InvoiceType("bogus"), amount: 1000, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triggersReferralConversion(tt.invoiceType, tt.amount); got != tt.want {
				t.Fatalf("triggersReferralConversion(%q, %v) = %v, want %v", tt.invoiceType, tt.amount, got, tt.want)
			}
		})
	}
}

func TestAwaitingReceiptVerificationErrorCarriesPendingPurchase(t *testing.T) {
	pending := &database.Purchase{
		ID:          42,
		InvoiceType: database.InvoiceTypeWalletTopUp,
		Amount:      30000,
	}

	err := awaitingReceiptVerificationError(pending)
	if !errors.Is(err, ErrAwaitingReceiptVerification) {
		t.Fatalf("awaitingReceiptVerificationError() does not wrap ErrAwaitingReceiptVerification: %v", err)
	}

	var pendingErr *AwaitingReceiptVerificationError
	if !errors.As(err, &pendingErr) {
		t.Fatal("awaitingReceiptVerificationError() does not expose AwaitingReceiptVerificationError")
	}
	if pendingErr.Purchase != pending {
		t.Fatalf("AwaitingReceiptVerificationError.Purchase = %#v, want original pending purchase", pendingErr.Purchase)
	}
}

func TestAccumulatedTrafficLimitGB(t *testing.T) {
	const gib = 1073741824

	tests := []struct {
		name             string
		existing         database.SubscriptionKey
		updatedUser      *remapi.User
		purchasedTraffic int
		want             int
	}{
		{
			name:             "falls back to local accumulation when remnawave limit missing",
			existing:         database.SubscriptionKey{TrafficLimitGB: 100},
			updatedUser:      &remapi.User{},
			purchasedTraffic: 100,
			want:             200,
		},
		{
			name:     "prefers remnawave reported total when available",
			existing: database.SubscriptionKey{TrafficLimitGB: 100},
			updatedUser: &remapi.User{
				TrafficLimitBytes: remapi.NewOptInt(200 * gib),
			},
			purchasedTraffic: 100,
			want:             200,
		},
		{
			name:             "keeps unlimited when existing key is unlimited",
			existing:         database.SubscriptionKey{TrafficLimitGB: 0},
			updatedUser:      &remapi.User{},
			purchasedTraffic: 100,
			want:             0,
		},
		{
			name:     "keeps unlimited when remnawave reports unlimited",
			existing: database.SubscriptionKey{TrafficLimitGB: 100},
			updatedUser: &remapi.User{
				TrafficLimitBytes: remapi.NewOptInt(0),
			},
			purchasedTraffic: 100,
			want:             0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accumulatedTrafficLimitGB(tt.existing, tt.updatedUser, tt.purchasedTraffic); got != tt.want {
				t.Fatalf("accumulatedTrafficLimitGB() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSyncedTrafficLimit(t *testing.T) {
	const gib = 1073741824

	t.Run("missing remote traffic keeps local limit and skips persistence", func(t *testing.T) {
		limitBytes, limitGB, persist := syncedTrafficLimit(100, remapi.User{})
		if limitBytes != 100*gib {
			t.Fatalf("syncedTrafficLimit() limitBytes = %d, want %d", limitBytes, 100*gib)
		}
		if limitGB != 100 {
			t.Fatalf("syncedTrafficLimit() limitGB = %d, want 100", limitGB)
		}
		if persist {
			t.Fatal("syncedTrafficLimit() persist = true, want false")
		}
	})

	t.Run("remote traffic overrides local value when present", func(t *testing.T) {
		limitBytes, limitGB, persist := syncedTrafficLimit(100, remapi.User{
			TrafficLimitBytes: remapi.NewOptInt(200 * gib),
		})
		if limitBytes != 200*gib {
			t.Fatalf("syncedTrafficLimit() limitBytes = %d, want %d", limitBytes, 200*gib)
		}
		if limitGB != 200 {
			t.Fatalf("syncedTrafficLimit() limitGB = %d, want 200", limitGB)
		}
		if !persist {
			t.Fatal("syncedTrafficLimit() persist = false, want true")
		}
	})
}

func TestOpenRouterAuthFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "openrouter auth failure",
			err: &gemini.ProviderError{
				Provider: "openrouter",
				Class:    gemini.ErrorClassAuth,
				Message:  "unauthorized",
			},
			want: true,
		},
		{
			name: "openrouter fallback auth failure",
			err: &gemini.ProviderError{
				Provider: "openrouter-fallback",
				Class:    gemini.ErrorClassAuth,
				Message:  "unauthorized",
			},
			want: true,
		},
		{
			name: "prefixed provider auth failure",
			err: &gemini.ProviderError{
				Provider: "OpenRouter-EU",
				Class:    gemini.ErrorClassAuth,
				Message:  "unauthorized",
			},
			want: true,
		},
		{
			name: "non-auth openrouter failure",
			err: &gemini.ProviderError{
				Provider: "openrouter",
				Class:    gemini.ErrorClassRateLimit,
				Message:  "too many requests",
			},
			want: false,
		},
		{
			name: "other provider auth failure",
			err: &gemini.ProviderError{
				Provider: "gemini",
				Class:    gemini.ErrorClassAuth,
				Message:  "unauthorized",
			},
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("boom"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr, got := openRouterAuthFailure(tt.err)
			if got != tt.want {
				t.Fatalf("openRouterAuthFailure() matched = %v, want %v", got, tt.want)
			}
			if tt.want && gotErr == nil {
				t.Fatal("openRouterAuthFailure() returned nil provider error for matching case")
			}
		})
	}
}

func TestProviderAuthVerificationResult(t *testing.T) {
	result := providerAuthVerificationResult()

	if result.Success {
		t.Fatal("providerAuthVerificationResult() should fail verification")
	}
	if result.ReasonKey != mobilePayProviderAuthReasonKey {
		t.Fatalf("providerAuthVerificationResult() reason key = %q, want %q", result.ReasonKey, mobilePayProviderAuthReasonKey)
	}
	if !strings.Contains(strings.ToLower(result.Reason), "temporarily unavailable") {
		t.Fatalf("providerAuthVerificationResult() reason = %q, want temporary-unavailable guidance", result.Reason)
	}
	if strings.Contains(strings.ToLower(result.Reason), "openrouter") {
		t.Fatalf("providerAuthVerificationResult() reason leaked provider name: %q", result.Reason)
	}
}

func TestScreenshotVerificationStatusFailure(t *testing.T) {
	if got := screenshotVerificationStatusFailure(database.PurchaseStatusPending); got != nil {
		t.Fatalf("pending status failure = %#v, want nil", got)
	}
	if got := screenshotVerificationStatusFailure(database.PurchaseStatusNew); got != nil {
		t.Fatalf("new status failure = %#v, want nil", got)
	}

	paid := screenshotVerificationStatusFailure(database.PurchaseStatusPaid)
	if paid == nil || paid.Success || paid.Reason != "Purchase already completed" {
		t.Fatalf("paid status failure = %#v, want already-completed failure", paid)
	}

	cancelled := screenshotVerificationStatusFailure(database.PurchaseStatusCancel)
	if cancelled == nil || cancelled.Success || cancelled.Reason != "Purchase is not awaiting verification" {
		t.Fatalf("cancel status failure = %#v, want not-awaiting-verification failure", cancelled)
	}
}

func TestDuplicateTransactionResultFromError(t *testing.T) {
	err := fmt.Errorf("insert mobile_payment_verification: %w", &pgconn.PgError{Code: "23505"})

	result, ok := duplicateTransactionResultFromError(err)
	if !ok {
		t.Fatal("duplicateTransactionResultFromError() ok = false, want true")
	}
	if result.Success {
		t.Fatal("duplicate transaction result should fail verification")
	}
	if result.ReasonKey != mobilePayDuplicateReasonKey {
		t.Fatalf("duplicate transaction reason key = %q, want %q", result.ReasonKey, mobilePayDuplicateReasonKey)
	}

	if result, ok := duplicateTransactionResultFromError(errors.New("other error")); ok || result != nil {
		t.Fatalf("non-unique error result = %#v, ok = %v; want nil false", result, ok)
	}
}

func TestVisionAlertCooldown(t *testing.T) {
	service := &PaymentService{
		visionAlertLastSent: make(map[string]time.Time),
	}
	key := "openrouter:auth"
	start := time.Now()

	if !service.claimVisionAlertSlot(key, start) {
		t.Fatal("claimVisionAlertSlot() first claim = false, want true")
	}
	if service.claimVisionAlertSlot(key, start.Add(visionProviderAlertCooldown/2)) {
		t.Fatal("claimVisionAlertSlot() within cooldown = true, want false")
	}
	if !service.claimVisionAlertSlot(key, start.Add(visionProviderAlertCooldown+time.Second)) {
		t.Fatal("claimVisionAlertSlot() after cooldown = false, want true")
	}
	if service.claimVisionAlertSlot("", start) {
		t.Fatal("claimVisionAlertSlot() empty key = true, want false")
	}
}

func TestReleaseVisionAlertSlot(t *testing.T) {
	service := &PaymentService{
		visionAlertLastSent: make(map[string]time.Time),
	}
	key := "openrouter:auth"
	start := time.Now()

	if !service.claimVisionAlertSlot(key, start) {
		t.Fatal("claimVisionAlertSlot() first claim = false, want true")
	}

	service.releaseVisionAlertSlot(key, start.Add(time.Second))
	if service.claimVisionAlertSlot(key, start.Add(2*time.Second)) {
		t.Fatal("releaseVisionAlertSlot() removed slot for mismatched timestamp")
	}

	service.releaseVisionAlertSlot(key, start)
	if !service.claimVisionAlertSlot(key, start.Add(2*time.Second)) {
		t.Fatal("releaseVisionAlertSlot() did not free slot for matching timestamp")
	}
}

func TestBuildVisionProviderAuthAlertEscapesHTML(t *testing.T) {
	alert := buildVisionProviderAuthAlert(&gemini.ProviderError{
		Provider: "openrouter-fallback",
		Class:    gemini.ErrorClassAuth,
		Message:  `<bad&token>`,
	})

	if strings.Contains(alert, "<bad&token>") {
		t.Fatalf("buildVisionProviderAuthAlert() leaked unescaped HTML: %q", alert)
	}
	if !strings.Contains(alert, "&lt;bad&amp;token&gt;") {
		t.Fatalf("buildVisionProviderAuthAlert() missing escaped error text: %q", alert)
	}
	if !strings.Contains(alert, "OPENROUTER_API_KEY") {
		t.Fatalf("buildVisionProviderAuthAlert() missing credential hint: %q", alert)
	}
}

func TestFormatShadowFailureReason(t *testing.T) {
	tests := []struct {
		name    string
		failure *verificationFailure
		want    string
	}{
		{
			name:    "nil failure means shadow pass",
			failure: nil,
			want:    "shadow_pass",
		},
		{
			name:    "empty failure means shadow pass",
			failure: &verificationFailure{},
			want:    "shadow_pass",
		},
		{
			name: "reason key only",
			failure: &verificationFailure{
				reasonKey: "mobile_pay_failed_amount",
			},
			want: "shadow_fail: mobile_pay_failed_amount",
		},
		{
			name: "reason and reason key",
			failure: &verificationFailure{
				reason:    "Amount mismatch: expected 12000, got 10000",
				reasonKey: "mobile_pay_failed_amount",
			},
			want: "shadow_fail: mobile_pay_failed_amount | Amount mismatch: expected 12000, got 10000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatShadowFailureReason(tt.failure); got != tt.want {
				t.Fatalf("formatShadowFailureReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"international with plus", "+959123456789", "9123456789"},
		{"international without plus", "959123456789", "9123456789"},
		{"local format", "09123456789", "9123456789"},
		{"local without country code", "9123456789", "9123456789"},
		{"with spaces", "+959 123 456 789", "9123456789"},
		{"with dashes", "+959-123-456-789", "9123456789"},
		{"with parens", "+959(123)456789", "9123456789"},
		{"with asterisks (masked)", "+959*****6789", "9596789"},
		{"already clean", "9123456789", "9123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePhone(tt.input)
			if got != tt.want {
				t.Errorf("normalizePhone(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPhoneMatchesSuffix(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		n        int
		want     bool
	}{
		{"exact match", "9123456789", "9123456789", 4, true},
		{"suffix match last 4", "9123456789", "6789", 4, true},
		{"suffix mismatch", "9123456789", "1234", 4, false},
		{"empty actual", "9123456789", "", 4, false},
		{"actual shorter than n but matches suffix", "9123456789", "89", 4, false},
		{"actual equals expected exact", "9876", "9876", 4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phoneMatchesSuffix(tt.expected, tt.actual, tt.n)
			if got != tt.want {
				t.Errorf("phoneMatchesSuffix(%q, %q, %d) = %v; want %v",
					tt.expected, tt.actual, tt.n, got, tt.want)
			}
		})
	}
}

func TestNormalizeRecipientName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"english spaces", "Aung Aung", "aungaung"},
		{"english punctuation", "Maung-Maung", "maungmaung"},
		{"burmese spacing", "အောင် အောင်", "အောင်အောင်"},
		{"mixed symbols", "  Mg. Mg / 123 ", "mgmg123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRecipientName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRecipientName(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchPaymentRecipient(t *testing.T) {
	origPhoneKPay, origPhoneWavePay, origPhoneAyaPay := PhoneKPay, PhoneWavePay, PhoneAyaPay
	origNameKPay, origNameWave, origNameAya := AccountNameKPay, AccountNameWave, AccountNameAya
	t.Cleanup(func() {
		PhoneKPay, PhoneWavePay, PhoneAyaPay = origPhoneKPay, origPhoneWavePay, origPhoneAyaPay
		AccountNameKPay, AccountNameWave, AccountNameAya = origNameKPay, origNameWave, origNameAya
	})

	PhoneKPay = "09111111111"
	PhoneWavePay = "09222222222"
	PhoneAyaPay = ""
	AccountNameKPay = "Maung Maung"
	AccountNameWave = "Aung Aung"
	AccountNameAya = "Aya Receiver"

	tests := []struct {
		name          string
		provider      string
		phone         string
		recipient     string
		wantKey       string
		wantMatched   bool
		wantMatchedBy string
	}{
		{
			name:          "match wave by provider-specific name",
			provider:      "wavepay",
			phone:         "",
			recipient:     "AungAung",
			wantKey:       "wavepay",
			wantMatched:   true,
			wantMatchedBy: "name",
		},
		{
			name:          "match kpay by phone suffix even if provider alias used",
			provider:      "kbzpay",
			phone:         "09***1111",
			recipient:     "",
			wantKey:       "kpay",
			wantMatched:   true,
			wantMatchedBy: "phone",
		},
		{
			name:          "fall back to any enabled provider",
			provider:      "",
			phone:         "09***2222",
			recipient:     "",
			wantKey:       "wavepay",
			wantMatched:   true,
			wantMatchedBy: "phone",
		},
		{
			name:          "disabled aya is ignored",
			provider:      "ayapay",
			phone:         "",
			recipient:     "Aya Receiver",
			wantKey:       "",
			wantMatched:   false,
			wantMatchedBy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, matchedBy, matched := MatchPaymentRecipient(tt.provider, tt.phone, tt.recipient, 4)
			if matched != tt.wantMatched {
				t.Fatalf("MatchPaymentRecipient() matched = %v; want %v", matched, tt.wantMatched)
			}
			if provider.Key != tt.wantKey {
				t.Errorf("MatchPaymentRecipient() provider = %q; want %q", provider.Key, tt.wantKey)
			}
			if matchedBy != tt.wantMatchedBy {
				t.Errorf("MatchPaymentRecipient() matchedBy = %q; want %q", matchedBy, tt.wantMatchedBy)
			}
		})
	}
}

// --- GetTestTransactionID ---

func TestGetTestTransactionID(t *testing.T) {
	svc := &PaymentService{}
	got := svc.GetTestTransactionID()
	if got == "" {
		t.Fatal("GetTestTransactionID() returned empty string")
	}
	if got != testTransactionID {
		t.Errorf("GetTestTransactionID() = %q, want %q", got, testTransactionID)
	}
}

// --- SetTestMode / IsTestMode concurrency ---

func TestSetTestMode_Concurrent(t *testing.T) {
	svc := &PaymentService{}
	done := make(chan struct{}, 50)

	for i := 0; i < 50; i++ {
		go func(v bool) {
			svc.SetTestMode(v)
		}(i%2 == 0)
	}
	for i := 0; i < 50; i++ {
		go func() {
			_ = svc.IsTestMode()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

// --- syncCacheEntry TTL logic ---

func TestSyncCacheEntry_TTL(t *testing.T) {
	fresh := syncCacheEntry{
		keys:      []KeyStats{{ID: 1}},
		expiresAt: time.Now().Add(syncCacheTTL),
	}
	if !time.Now().Before(fresh.expiresAt) {
		t.Error("fresh entry should not be expired yet")
	}

	expired := syncCacheEntry{
		keys:      []KeyStats{{ID: 2}},
		expiresAt: time.Now().Add(-time.Minute),
	}
	if time.Now().Before(expired.expiresAt) {
		t.Error("stale entry should be expired")
	}
}

func TestWithIdempotencyKeyRoundTrip(t *testing.T) {
	key := uuid.New()
	ctx := WithIdempotencyKey(context.Background(), key)

	got := idempotencyKeyFromContext(ctx)
	if got == nil {
		t.Fatal("expected idempotency key in context")
	}
	if *got != key {
		t.Fatalf("idempotencyKeyFromContext() = %s, want %s", *got, key)
	}
}

func TestIdempotencyKeyFromContextReturnsNilWhenMissing(t *testing.T) {
	if got := idempotencyKeyFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil idempotency key, got %v", got)
	}
}

// --- F3 / B1: Idempotency-Key scoped to customer + request body ---

func TestAssertIdempotentResumeAllowed_CrossUserConflict(t *testing.T) {
	key := uuid.New()
	existing := &database.Purchase{
		ID:             100,
		CustomerID:     1,
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
		Status:         database.PurchaseStatusPending,
		IdempotencyKey: &key,
	}
	candidate := &database.Purchase{
		CustomerID:     2, // different caller
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
	}

	err := assertIdempotentResumeAllowed(existing, candidate)
	if !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("assertIdempotentResumeAllowed() error = %v, want ErrIdempotencyKeyConflict", err)
	}
}

func TestAssertIdempotentResumeAllowed_SameUserSameBodyOK(t *testing.T) {
	key := uuid.New()
	existing := &database.Purchase{
		ID:             100,
		CustomerID:     1,
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
		Status:         database.PurchaseStatusPending,
		IdempotencyKey: &key,
	}
	candidate := &database.Purchase{
		CustomerID:     1,
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
	}

	if err := assertIdempotentResumeAllowed(existing, candidate); err != nil {
		t.Fatalf("assertIdempotentResumeAllowed() error = %v, want nil for same-user same body", err)
	}
}

func TestAssertIdempotentResumeAllowed_SameUserBodyMismatch(t *testing.T) {
	key := uuid.New()
	existing := &database.Purchase{
		ID:             100,
		CustomerID:     1,
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
		Status:         database.PurchaseStatusPending,
		IdempotencyKey: &key,
	}
	candidate := &database.Purchase{
		CustomerID:     1,
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         18000, // different plan/amount
		Days:           30,
		TrafficLimitGB: 100,
	}

	err := assertIdempotentResumeAllowed(existing, candidate)
	if !errors.Is(err, ErrIdempotencyRequestMismatch) {
		t.Fatalf("assertIdempotentResumeAllowed() error = %v, want ErrIdempotencyRequestMismatch", err)
	}
}

func TestIdempotentPurchaseMatchesRequest_RejectsDifferentInvoiceType(t *testing.T) {
	if idempotentPurchaseMatchesRequest(
		&database.Purchase{InvoiceType: database.InvoiceTypeMobileBanking, Amount: 1000},
		&database.Purchase{InvoiceType: database.InvoiceTypeWalletTopUp, Amount: 1000},
	) {
		t.Fatal("idempotentPurchaseMatchesRequest() = true for different invoice types, want false")
	}
}

func TestIdempotentPurchaseMatchesRequest_WalletTopUpAmountOnly(t *testing.T) {
	if !idempotentPurchaseMatchesRequest(
		&database.Purchase{InvoiceType: database.InvoiceTypeWalletTopUp, Amount: 5000},
		&database.Purchase{InvoiceType: database.InvoiceTypeWalletTopUp, Amount: 5000},
	) {
		t.Fatal("idempotentPurchaseMatchesRequest() = false for matching top-up, want true")
	}
	if idempotentPurchaseMatchesRequest(
		&database.Purchase{InvoiceType: database.InvoiceTypeWalletTopUp, Amount: 5000},
		&database.Purchase{InvoiceType: database.InvoiceTypeWalletTopUp, Amount: 9000},
	) {
		t.Fatal("idempotentPurchaseMatchesRequest() = true for different top-up amount, want false")
	}
}

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("expected unique violation to be detected")
	}
	if isUniqueViolation(errors.New("not a pg error")) {
		t.Fatal("unexpected unique violation match")
	}
}

type fakeWalletTopUpTx struct {
	commitErr error
	committed bool
	rollbacks int
}

func (f *fakeWalletTopUpTx) Commit(_ context.Context) error {
	if f.commitErr != nil {
		return f.commitErr
	}
	f.committed = true
	return nil
}

func (f *fakeWalletTopUpTx) Rollback(_ context.Context) error {
	f.rollbacks++
	return nil
}

type fakeWalletTopUpStore struct {
	tx             *fakeWalletTopUpTx
	beginErr       error
	addErr         error
	logErr         error
	addCalls       int
	logCalls       int
	lastCustomerID int64
	lastPurchaseID int64
	lastAmount     float64
}

func (f *fakeWalletTopUpStore) BeginTx(_ context.Context) (walletTopUpTx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	return f.tx, nil
}

func (f *fakeWalletTopUpStore) AddBalance(_ context.Context, _ walletTopUpTx, customerID int64, amount float64) error {
	f.addCalls++
	f.lastCustomerID = customerID
	f.lastAmount = amount
	return f.addErr
}

func (f *fakeWalletTopUpStore) LogTopUp(_ context.Context, _ walletTopUpTx, purchaseID int64, customerID int64, amount float64) error {
	f.logCalls++
	f.lastPurchaseID = purchaseID
	f.lastCustomerID = customerID
	f.lastAmount = amount
	return f.logErr
}

// --- PR 1: finalize-only recovery for stuck processing wallet top-ups ---

type fakeWalletTopUpFinalizeStore struct {
	purchase       *database.Purchase
	findErr        error
	ledgerOK       bool
	topupExists    bool
	refundExists   bool
	existsErr      error
	markPaidErr    error
	markPaidCalls  int
	existsCalls    []database.WalletTransactionType
	lastPurchaseID int64
}

func (f *fakeWalletTopUpFinalizeStore) FindPurchaseByID(_ context.Context, purchaseID int64) (*database.Purchase, error) {
	f.lastPurchaseID = purchaseID
	if f.findErr != nil {
		return nil, f.findErr
	}
	if f.purchase == nil {
		return nil, nil
	}
	// Return a shallow copy so tests can assert status without shared mutation surprises.
	cp := *f.purchase
	return &cp, nil
}

func (f *fakeWalletTopUpFinalizeStore) WalletLedgerAvailable() bool {
	return f.ledgerOK
}

func (f *fakeWalletTopUpFinalizeStore) ExistsByPurchaseIDAndType(_ context.Context, purchaseID int64, txType database.WalletTransactionType) (bool, error) {
	f.lastPurchaseID = purchaseID
	f.existsCalls = append(f.existsCalls, txType)
	if f.existsErr != nil {
		return false, f.existsErr
	}
	switch txType {
	case database.WalletTransactionTypeTopup:
		return f.topupExists, nil
	case database.WalletTransactionTypeRefund:
		return f.refundExists, nil
	default:
		return false, nil
	}
}

func (f *fakeWalletTopUpFinalizeStore) MarkAsPaid(_ context.Context, purchaseID int64) error {
	f.markPaidCalls++
	f.lastPurchaseID = purchaseID
	if f.markPaidErr != nil {
		return f.markPaidErr
	}
	if f.purchase != nil {
		f.purchase.Status = database.PurchaseStatusPaid
	}
	return nil
}

func TestFinalizeProcessingWalletTopUpIfSettled_ProcessingTopUpWithLedgerFinalizes(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          42,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK:    true,
		topupExists: true,
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 42, store)
	if err != nil {
		t.Fatalf("finalizeProcessingWalletTopUpIfSettled() error = %v, want nil", err)
	}
	if store.markPaidCalls != 1 {
		t.Fatalf("MarkAsPaid calls = %d, want 1", store.markPaidCalls)
	}
	if store.purchase.Status != database.PurchaseStatusPaid {
		t.Fatalf("purchase status = %s, want paid", store.purchase.Status)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_MissingTopUpLedgerRefuses(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          42,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK:    true,
		topupExists: false,
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 42, store)
	if !errors.Is(err, ErrPurchaseFinalizationEvidenceMissing) {
		t.Fatalf("error = %v, want ErrPurchaseFinalizationEvidenceMissing", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_RefundLedgerRefuses(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          42,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK:     true,
		topupExists:  true,
		refundExists: true,
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 42, store)
	if !errors.Is(err, ErrPurchaseFinalizationEvidenceMissing) {
		t.Fatalf("error = %v, want ErrPurchaseFinalizationEvidenceMissing", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_PaidNoOp(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          42,
			Status:      database.PurchaseStatusPaid,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK: true,
		// Even if ledger flags are false, paid must no-op before evidence.
		topupExists: false,
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 42, store)
	if err != nil {
		t.Fatalf("error = %v, want nil for already-paid", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0 for paid no-op", store.markPaidCalls)
	}
	if len(store.existsCalls) != 0 {
		t.Fatalf("ExistsByPurchaseIDAndType calls = %v, want none before evidence for paid", store.existsCalls)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_UnsupportedStatuses(t *testing.T) {
	statuses := []database.PurchaseStatus{
		database.PurchaseStatusNew,
		database.PurchaseStatusPending,
		database.PurchaseStatusCancel,
		database.PurchaseStatus("failed"),
		database.PurchaseStatus("refunded"),
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			store := &fakeWalletTopUpFinalizeStore{
				purchase: &database.Purchase{
					ID:          7,
					Status:      status,
					InvoiceType: database.InvoiceTypeWalletTopUp,
				},
				ledgerOK:    true,
				topupExists: true,
			}
			err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 7, store)
			if !errors.Is(err, ErrPurchaseFinalizationNotSupported) {
				t.Fatalf("status %s: error = %v, want ErrPurchaseFinalizationNotSupported", status, err)
			}
			if store.markPaidCalls != 0 {
				t.Fatalf("status %s: MarkAsPaid calls = %d, want 0", status, store.markPaidCalls)
			}
		})
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_WalletPaymentWithPurchaseLedgerRefuses(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          9,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletPayment,
		},
		ledgerOK:    true,
		topupExists: true, // even if a ledger row exists, service path is refused
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 9, store)
	if !errors.Is(err, ErrPurchaseFinalizationNotSupported) {
		t.Fatalf("error = %v, want ErrPurchaseFinalizationNotSupported", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
	if len(store.existsCalls) != 0 {
		t.Fatalf("must not inspect ledger for non-top-up: calls=%v", store.existsCalls)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_MobileBankingRefuses(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          10,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeMobileBanking,
		},
		ledgerOK:    true,
		topupExists: true,
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 10, store)
	if !errors.Is(err, ErrPurchaseFinalizationNotSupported) {
		t.Fatalf("error = %v, want ErrPurchaseFinalizationNotSupported", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_WalletPaymentWithExtendKeyIDRefuses(t *testing.T) {
	extendID := int64(55)
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          11,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletPayment,
			ExtendKeyID: &extendID,
		},
		ledgerOK:    true,
		topupExists: true,
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 11, store)
	if !errors.Is(err, ErrPurchaseFinalizationNotSupported) {
		t.Fatalf("error = %v, want ErrPurchaseFinalizationNotSupported", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_NewKeyLikePurchaseRefuses(t *testing.T) {
	// New-key service purchase: wallet_payment, no ExtendKeyID, days/traffic set.
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:             12,
			Status:         database.PurchaseStatusProcessing,
			InvoiceType:    database.InvoiceTypeWalletPayment,
			Days:           30,
			TrafficLimitGB: 100,
		},
		ledgerOK:    true,
		topupExists: true,
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 12, store)
	if !errors.Is(err, ErrPurchaseFinalizationNotSupported) {
		t.Fatalf("error = %v, want ErrPurchaseFinalizationNotSupported", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
}

func TestFinalizeProcessingWalletTopUpIfSettled_NilWalletRepoRefusesNoPanic(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          13,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK: false, // walletTxRepo nil
	}

	err := finalizeProcessingWalletTopUpIfSettled(context.Background(), 13, store)
	if !errors.Is(err, ErrPurchaseFinalizationEvidenceMissing) {
		t.Fatalf("error = %v, want ErrPurchaseFinalizationEvidenceMissing", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
	if len(store.existsCalls) != 0 {
		t.Fatalf("must not call Exists when ledger unavailable: %v", store.existsCalls)
	}
}

func TestProcessPurchaseWhenAlreadyProcessing_TopUpWithLedgerFinalizes(t *testing.T) {
	// Wired ProcessPurchaseById branch: successful finalize returns nil (not InFlight).
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          99,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK:    true,
		topupExists: true,
	}

	err := processPurchaseWhenAlreadyProcessing(context.Background(), 99, func(ctx context.Context, id int64) error {
		return finalizeProcessingWalletTopUpIfSettled(ctx, id, store)
	})
	if err != nil {
		t.Fatalf("processPurchaseWhenAlreadyProcessing() error = %v, want nil", err)
	}
	if store.markPaidCalls != 1 {
		t.Fatalf("MarkAsPaid calls = %d, want 1", store.markPaidCalls)
	}
}

func TestProcessPurchaseWhenAlreadyProcessing_MarkAsPaidErrorReturnsWrappedInFlight(t *testing.T) {
	markErr := errors.New("mark paid failed")
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          102,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK:    true,
		topupExists: true,
		markPaidErr: markErr,
	}

	err := processPurchaseWhenAlreadyProcessing(context.Background(), 102, func(ctx context.Context, id int64) error {
		return finalizeProcessingWalletTopUpIfSettled(ctx, id, store)
	})
	if !errors.Is(err, ErrPurchaseInFlight) {
		t.Fatalf("error = %v, want ErrPurchaseInFlight for external callers", err)
	}
	if !strings.Contains(err.Error(), markErr.Error()) {
		t.Fatalf("error = %v, want underlying MarkAsPaid error visible", err)
	}
	if store.markPaidCalls != 1 {
		t.Fatalf("MarkAsPaid calls = %d, want 1", store.markPaidCalls)
	}
}

func TestProcessPurchaseWhenAlreadyProcessing_EvidenceMissingReturnsInFlight(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          100,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeWalletTopUp,
		},
		ledgerOK:    true,
		topupExists: false,
	}

	err := processPurchaseWhenAlreadyProcessing(context.Background(), 100, func(ctx context.Context, id int64) error {
		return finalizeProcessingWalletTopUpIfSettled(ctx, id, store)
	})
	if !errors.Is(err, ErrPurchaseInFlight) {
		t.Fatalf("error = %v, want ErrPurchaseInFlight for external callers", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
}

func TestProcessPurchaseWhenAlreadyProcessing_UnsupportedReturnsInFlight(t *testing.T) {
	store := &fakeWalletTopUpFinalizeStore{
		purchase: &database.Purchase{
			ID:          101,
			Status:      database.PurchaseStatusProcessing,
			InvoiceType: database.InvoiceTypeMobileBanking,
		},
		ledgerOK: true,
	}

	err := processPurchaseWhenAlreadyProcessing(context.Background(), 101, func(ctx context.Context, id int64) error {
		return finalizeProcessingWalletTopUpIfSettled(ctx, id, store)
	})
	if !errors.Is(err, ErrPurchaseInFlight) {
		t.Fatalf("error = %v, want ErrPurchaseInFlight", err)
	}
	if store.markPaidCalls != 0 {
		t.Fatalf("MarkAsPaid calls = %d, want 0", store.markPaidCalls)
	}
}

func TestSettleWalletTopUpSuccess(t *testing.T) {
	tx := &fakeWalletTopUpTx{}
	store := &fakeWalletTopUpStore{tx: tx}
	var restored []int64

	err := settleWalletTopUp(
		context.Background(),
		store,
		11,
		22,
		5000,
		database.PurchaseStatusPending,
		func(_ context.Context, purchaseID int64, _ database.PurchaseStatus) {
			restored = append(restored, purchaseID)
		},
	)
	if err != nil {
		t.Fatalf("settleWalletTopUp() error = %v", err)
	}
	if !tx.committed {
		t.Fatal("expected transaction to commit on success")
	}
	if store.addCalls != 1 {
		t.Fatalf("AddBalance() calls = %d, want 1", store.addCalls)
	}
	if store.logCalls != 1 {
		t.Fatalf("LogTopUp() calls = %d, want 1", store.logCalls)
	}
	if len(restored) != 0 {
		t.Fatalf("restore should not be called on success, got %v", restored)
	}
}

func TestSettleWalletTopUpRestoresStateWhenLoggingFails(t *testing.T) {
	tx := &fakeWalletTopUpTx{}
	store := &fakeWalletTopUpStore{
		tx:     tx,
		logErr: errors.New("log failed"),
	}
	var restored []int64

	err := settleWalletTopUp(
		context.Background(),
		store,
		33,
		44,
		9000,
		database.PurchaseStatusNew,
		func(_ context.Context, purchaseID int64, _ database.PurchaseStatus) {
			restored = append(restored, purchaseID)
		},
	)
	if err == nil {
		t.Fatal("settleWalletTopUp() expected error")
	}
	if tx.committed {
		t.Fatal("transaction must not commit when logging fails")
	}
	if store.addCalls != 1 {
		t.Fatalf("AddBalance() calls = %d, want 1", store.addCalls)
	}
	if store.logCalls != 1 {
		t.Fatalf("LogTopUp() calls = %d, want 1", store.logCalls)
	}
	if len(restored) != 1 || restored[0] != 33 {
		t.Fatalf("restore calls = %v, want [33]", restored)
	}
}

func TestVisionDecisionToVerificationFailureMapsAskClearer(t *testing.T) {
	failure := visionDecisionToVerificationFailure(gemini.AnalysisAssessment{
		Action: gemini.OutcomeAskClearer,
		Reason: "clearer_image_required",
	})
	if failure == nil {
		t.Fatal("visionDecisionToVerificationFailure() = nil, want failure")
	}
	if failure.reasonKey != "mobile_pay_failed_unclear_screenshot" {
		t.Fatalf("visionDecisionToVerificationFailure().reasonKey = %q, want %q", failure.reasonKey, "mobile_pay_failed_unclear_screenshot")
	}
}

func TestCanReuseAwaitingVerificationPurchase(t *testing.T) {
	base := &database.Purchase{
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
	}

	if !canReuseAwaitingVerificationPurchase(base, &database.Purchase{
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
	}) {
		t.Fatal("canReuseAwaitingVerificationPurchase() identical receipt purchase = false, want true")
	}

	if canReuseAwaitingVerificationPurchase(base, &database.Purchase{
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         9000,
		Days:           30,
		TrafficLimitGB: 100,
	}) {
		t.Fatal("canReuseAwaitingVerificationPurchase() different amount = true, want false")
	}
}

func TestCanReuseAwaitingVerificationPurchaseRejectsDifferentPromoReservation(t *testing.T) {
	existingPromoID := int64(1)
	candidatePromoID := int64(2)

	if canReuseAwaitingVerificationPurchase(&database.Purchase{
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
		PromoCodeID:    &existingPromoID,
	}, &database.Purchase{
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Amount:         12000,
		Days:           30,
		TrafficLimitGB: 100,
		PromoCodeID:    &candidatePromoID,
	}) {
		t.Fatal("canReuseAwaitingVerificationPurchase() different promo reservation = true, want false")
	}
}

// --- F1 / A2: mobile receipt txn uniqueness after ProcessPurchaseById errors ---
//
// applyMobileTxnCleanupAfterProcessError is the VerifyMobilePayment cleanup seam:
// it either calls DeleteByTransactionID (via deleteFn) or retains the row.

func TestApplyMobileTxnCleanupAfterProcessError_HardFailureDeletes(t *testing.T) {
	var deleteCalls int
	err := applyMobileTxnCleanupAfterProcessError(
		errors.New("remnawave unavailable before mutation"),
		func() error {
			deleteCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("applyMobileTxnCleanupAfterProcessError() error = %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("DeleteByTransactionID calls = %d, want 1 for safe hard failure", deleteCalls)
	}
}

func TestApplyMobileTxnCleanupAfterProcessError_FinalizationPendingRetains(t *testing.T) {
	var deleteCalls int
	err := applyMobileTxnCleanupAfterProcessError(
		ErrPurchaseFinalizationPending,
		func() error {
			deleteCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("applyMobileTxnCleanupAfterProcessError() error = %v", err)
	}
	if deleteCalls != 0 {
		t.Fatalf("DeleteByTransactionID calls = %d, want 0 for finalization-pending", deleteCalls)
	}

	// Wrapped form (as returned through fmt.Errorf %w chains).
	err = applyMobileTxnCleanupAfterProcessError(
		fmt.Errorf("wrap: %w", ErrPurchaseFinalizationPending),
		func() error {
			deleteCalls++
			return nil
		},
	)
	if err != nil || deleteCalls != 0 {
		t.Fatalf("wrapped finalization-pending: err=%v deleteCalls=%d, want nil/0", err, deleteCalls)
	}
}

func TestApplyMobileTxnCleanupAfterProcessError_InFlightRetains(t *testing.T) {
	var deleteCalls int
	err := applyMobileTxnCleanupAfterProcessError(
		ErrPurchaseInFlight,
		func() error {
			deleteCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("applyMobileTxnCleanupAfterProcessError() error = %v", err)
	}
	if deleteCalls != 0 {
		t.Fatalf("DeleteByTransactionID calls = %d, want 0 for in-flight", deleteCalls)
	}
}

func TestApplyMobileTxnCleanupAfterProcessError_TopUpMarkAsPaidFailRetains(t *testing.T) {
	// A4: settle commits balance/log then MarkAsPaid fails → finalization-pending.
	// Same receipt must not be freed for a second credit.
	var deleteCalls int
	_ = applyMobileTxnCleanupAfterProcessError(ErrPurchaseFinalizationPending, func() error {
		deleteCalls++
		return nil
	})
	if deleteCalls != 0 {
		t.Fatalf("top-up MarkAsPaid-fail path must retain txn uniqueness, deleteCalls=%d", deleteCalls)
	}
}

func TestApplyMobileTxnCleanupAfterProcessError_PropagatesDeleteError(t *testing.T) {
	want := errors.New("delete failed")
	err := applyMobileTxnCleanupAfterProcessError(errors.New("hard fail"), func() error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("applyMobileTxnCleanupAfterProcessError() error = %v, want delete error", err)
	}
}

// --- F1 / A1: shared wallet process-error action path (both create paths) ---

func TestApplyWalletProcessErrorActions_InFlightNoRefundCancel(t *testing.T) {
	// Shared by createWalletPurchase and CreatePurchaseWithExtend.
	var cleanupCalls int
	action, treatOK, propErr := applyWalletProcessErrorActions(
		ErrPurchaseInFlight,
		true, // created — still must NOT refund/cancel
		func() error {
			cleanupCalls++
			return nil
		},
	)
	if action != walletProcessPropagateOnly {
		t.Fatalf("action = %v, want propagate-only", action)
	}
	if treatOK {
		t.Fatal("InFlight must not be treated as finalization success")
	}
	if !errors.Is(propErr, ErrPurchaseInFlight) {
		t.Fatalf("propagate err = %v, want ErrPurchaseInFlight", propErr)
	}
	if cleanupCalls != 0 {
		t.Fatalf("refund/cancel calls = %d, want 0 for InFlight", cleanupCalls)
	}

	// Wrapped claim-lost
	cleanupCalls = 0
	action, _, propErr = applyWalletProcessErrorActions(
		fmt.Errorf("claim lost: %w", ErrPurchaseInFlight),
		true,
		func() error {
			cleanupCalls++
			return nil
		},
	)
	if action != walletProcessPropagateOnly || !errors.Is(propErr, ErrPurchaseInFlight) || cleanupCalls != 0 {
		t.Fatalf("wrapped InFlight: action=%v propErr=%v cleanupCalls=%d", action, propErr, cleanupCalls)
	}
}

func TestApplyWalletProcessErrorActions_HardFailureRefundCancelWhenCreated(t *testing.T) {
	hardErr := errors.New("remnawave create failed before mutation")
	var cleanupCalls int
	action, treatOK, propErr := applyWalletProcessErrorActions(
		hardErr,
		true,
		func() error {
			cleanupCalls++
			return nil
		},
	)
	if action != walletProcessRefundAndCancel {
		t.Fatalf("action = %v, want refund-and-cancel", action)
	}
	if treatOK {
		t.Fatal("hard failure must not be treated as success")
	}
	if !errors.Is(propErr, hardErr) {
		t.Fatalf("propagate err = %v, want hardErr", propErr)
	}
	if cleanupCalls != 1 {
		t.Fatalf("refund/cancel calls = %d, want 1 for hard failure when created", cleanupCalls)
	}
}

func TestApplyWalletProcessErrorActions_HardFailureNoCleanupWhenNotCreated(t *testing.T) {
	// Idempotent resume of existing purchase: do not refund on process error.
	var cleanupCalls int
	action, _, propErr := applyWalletProcessErrorActions(
		errors.New("hard"),
		false,
		func() error {
			cleanupCalls++
			return nil
		},
	)
	if action != walletProcessPropagateOnly || propErr == nil || cleanupCalls != 0 {
		t.Fatalf("!created hard fail: action=%v propErr=%v cleanupCalls=%d", action, propErr, cleanupCalls)
	}
}

func TestApplyWalletProcessErrorActions_FinalizationPendingTreatAsSuccess(t *testing.T) {
	var cleanupCalls int
	action, treatOK, propErr := applyWalletProcessErrorActions(
		ErrPurchaseFinalizationPending,
		true,
		func() error {
			cleanupCalls++
			return nil
		},
	)
	if action != walletProcessTreatAsSuccess || !treatOK || propErr != nil || cleanupCalls != 0 {
		t.Fatalf("finalization-pending: action=%v treatOK=%v propErr=%v cleanupCalls=%d",
			action, treatOK, propErr, cleanupCalls)
	}
}

// Cleanup failures wrap the original process error with %w. Callers must not use
// errors.Is(propErr, processErr) to decide which error to return — that swallows
// the refund failure (pre-F1 surfaced refund failure; F1 regression).
func TestWalletPurchaseProcessErrorResult_CleanupFailureSurfacesRefundError(t *testing.T) {
	processErr := errors.New("remnawave create failed before mutation")
	refundErr := errors.New("balance update failed")
	// Same wrapping shape as createWalletPurchase / CreatePurchaseWithExtend cleanupFn.
	cleanupErr := fmt.Errorf("failed to process wallet purchase: %w (refund failed: %v)", processErr, refundErr)

	action, treatOK, propErr := applyWalletProcessErrorActions(processErr, true, func() error {
		return cleanupErr
	})
	if treatOK || action != walletProcessRefundAndCancel {
		t.Fatalf("action=%v treatOK=%v, want refund-and-cancel / false", action, treatOK)
	}
	if propErr == nil || propErr.Error() != cleanupErr.Error() {
		t.Fatalf("applyWalletProcessErrorActions propErr = %v, want cleanupErr", propErr)
	}
	// Sanity: buggy errors.Is check would treat cleanup as "same as process".
	if !errors.Is(propErr, processErr) {
		t.Fatal("expected cleanupErr to wrap processErr (reproduces the swallow bug condition)")
	}

	id, retErr := walletPurchaseProcessErrorResult(42, processErr, propErr, action, treatOK)
	if id != 0 {
		t.Fatalf("purchase id = %d, want 0 after cleanup attempt", id)
	}
	if retErr == nil {
		t.Fatal("expected cleanup/refund failure to be returned")
	}
	// Must surface cleanup failure, not bare process error.
	if retErr == processErr {
		t.Fatal("cleanup failure was swallowed; returned bare process error")
	}
	if !strings.Contains(retErr.Error(), "refund failed") {
		t.Fatalf("returned error %q must include refund failure detail", retErr.Error())
	}
	if !errors.Is(retErr, processErr) {
		// Still wraps process err for diagnostics, but is the cleanup error value.
		t.Fatalf("cleanup error should still wrap process err for context: %v", retErr)
	}
}

func TestWalletPurchaseProcessErrorResult_CleanupSuccessReturnsProcessError(t *testing.T) {
	processErr := errors.New("remnawave create failed before mutation")
	action, treatOK, propErr := applyWalletProcessErrorActions(processErr, true, func() error {
		return nil
	})
	id, retErr := walletPurchaseProcessErrorResult(99, processErr, propErr, action, treatOK)
	if id != 0 {
		t.Fatalf("purchase id = %d, want 0 after successful refund/cancel", id)
	}
	if !errors.Is(retErr, processErr) {
		t.Fatalf("retErr = %v, want original process error after successful cleanup", retErr)
	}
	if strings.Contains(fmt.Sprint(retErr), "refund failed") {
		t.Fatalf("successful cleanup must not invent refund failure: %v", retErr)
	}
}

func TestWalletPurchaseProcessErrorResult_InFlightPropagatesWithoutClearingID(t *testing.T) {
	action, treatOK, propErr := applyWalletProcessErrorActions(ErrPurchaseInFlight, true, func() error {
		t.Fatal("cleanup must not run")
		return nil
	})
	id, retErr := walletPurchaseProcessErrorResult(77, ErrPurchaseInFlight, propErr, action, treatOK)
	if treatOK || id != 77 || !errors.Is(retErr, ErrPurchaseInFlight) {
		t.Fatalf("InFlight: id=%d treatOK=%v retErr=%v", id, treatOK, retErr)
	}
}

func TestWalletPurchaseProcessErrorResult_FinalizationPendingTreatAsSuccess(t *testing.T) {
	action, treatOK, propErr := applyWalletProcessErrorActions(ErrPurchaseFinalizationPending, true, nil)
	id, retErr := walletPurchaseProcessErrorResult(55, ErrPurchaseFinalizationPending, propErr, action, treatOK)
	if !treatOK || id != 55 || retErr != nil {
		t.Fatalf("finalization-pending: id=%d treatOK=%v retErr=%v", id, treatOK, retErr)
	}
}

// --- F1 / A7: sequential best-effort duplicate refund guard ---
//
// Concurrent double-refund is NOT prevented without atomic DB uniqueness on
// (purchase_id, type=refund). This guard only covers sequential cleanup retries.

func TestApplyWalletRefundIfNeeded_SequentialNoDoubleCredit(t *testing.T) {
	credits := 0
	credit := func() error {
		credits++
		return nil
	}

	// First cleanup: no prior refund log → credit once.
	skipped, err := applyWalletRefundIfNeeded(false, credit)
	if err != nil || skipped {
		t.Fatalf("first refund: skipped=%v err=%v, want false/nil", skipped, err)
	}
	if credits != 1 {
		t.Fatalf("credits after first = %d, want 1", credits)
	}

	// Second cleanup: refund already logged → skip credit.
	skipped, err = applyWalletRefundIfNeeded(true, credit)
	if err != nil || !skipped {
		t.Fatalf("second refund: skipped=%v err=%v, want true/nil", skipped, err)
	}
	if credits != 1 {
		t.Fatalf("credits after second = %d, want 1 (no double credit)", credits)
	}
}

func TestApplyWalletRefundIfNeeded_PropagatesCreditError(t *testing.T) {
	want := errors.New("balance update failed")
	skipped, err := applyWalletRefundIfNeeded(false, func() error { return want })
	if skipped || !errors.Is(err, want) {
		t.Fatalf("skipped=%v err=%v, want false/%v", skipped, err, want)
	}
}
