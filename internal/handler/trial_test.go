package handler

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

func TestBuildDirectSubscriptionKeyboard(t *testing.T) {
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	subURL := "https://example.com/sub/token123"

	// 1. Test with Mini App URL configured: Happ button should route through /redirect?sub=...
	restoreMiniApp := config.SetMiniAppURLForTesting("https://shop.wavypremium.xyz")
	defer restoreMiniApp()

	markup := h.buildDirectSubscriptionKeyboard("en", subURL)

	if len(markup) < 3 {
		t.Fatalf("buildDirectSubscriptionKeyboard() rows = %d, want at least 3", len(markup))
	}

	// First row should be Happ Proxy 1-click import link via /redirect
	happRow := markup[0]
	if len(happRow) != 1 {
		t.Fatalf("first row button count = %d, want 1", len(happRow))
	}
	happBtn := happRow[0]
	wantURL := "https://shop.wavypremium.xyz/redirect?sub=" + url.QueryEscape(subURL)
	if happBtn.URL != wantURL {
		t.Fatalf("happBtn.URL = %q, want %q", happBtn.URL, wantURL)
	}
	if !strings.Contains(happBtn.Text, "Happ") {
		t.Fatalf("happBtn.Text = %q, want Happ proxy text", happBtn.Text)
	}

	// 2. Test fallback when Mini App URL is empty: Happ button falls back to raw subURL
	restoreEmpty := config.SetMiniAppURLForTesting("")
	markupFallback := h.buildDirectSubscriptionKeyboard("en", subURL)
	if markupFallback[0][0].URL != subURL {
		t.Fatalf("fallback happBtn.URL = %q, want raw %q", markupFallback[0][0].URL, subURL)
	}
	restoreEmpty()

	// Second row should contain iOS App Store and Android Play Store download buttons
	downloadRow := markup[1]
	if len(downloadRow) != 2 {
		t.Fatalf("download row button count = %d, want 2", len(downloadRow))
	}
	iosBtn := downloadRow[0]
	if iosBtn.URL != "https://apps.apple.com/us/app/happ-proxy-utility/id6504287215" {
		t.Fatalf("iosBtn.URL = %q, want Apple App Store URL", iosBtn.URL)
	}
	if !strings.Contains(iosBtn.Text, "iOS") {
		t.Fatalf("iosBtn.Text = %q, want iOS label", iosBtn.Text)
	}
	androidBtn := downloadRow[1]
	if androidBtn.URL != "https://play.google.com/store/apps/details?id=com.happproxy&hl=en_US" {
		t.Fatalf("androidBtn.URL = %q, want Google Play Store URL", androidBtn.URL)
	}
	if !strings.Contains(androidBtn.Text, "Android") {
		t.Fatalf("androidBtn.Text = %q, want Android label", androidBtn.Text)
	}

	// Last row should be back button
	backRow := markup[len(markup)-1]
	if len(backRow) != 1 || backRow[0].CallbackData != CallbackStart {
		t.Fatalf("backRow = %#v, want back button with CallbackStart", backRow)
	}
}

func TestBuildOneClickHappURL(t *testing.T) {
	subURL := "https://sub.wavypremium.xyz/test-key-123"

	// With base URL with trailing slash
	restore := config.SetMiniAppURLForTesting("https://shop.wavypremium.xyz/")
	defer restore()

	got := buildOneClickHappURL(subURL)
	want := "https://shop.wavypremium.xyz/redirect?sub=" + url.QueryEscape(subURL)
	if got != want {
		t.Fatalf("buildOneClickHappURL() = %q, want %q", got, want)
	}

	// With query parameters in base URL that should be stripped
	restoreWithQuery := config.SetMiniAppURLForTesting("https://shop.wavypremium.xyz/app?param=1&foo=bar")
	defer restoreWithQuery()

	gotQuery := buildOneClickHappURL(subURL)
	if gotQuery != want {
		t.Fatalf("buildOneClickHappURL(with queries) = %q, want %q", gotQuery, want)
	}

	// With empty base URL: fallback to subURL
	restoreEmpty := config.SetMiniAppURLForTesting("")
	defer restoreEmpty()

	gotEmpty := buildOneClickHappURL(subURL)
	if gotEmpty != subURL {
		t.Fatalf("buildOneClickHappURL(empty base) = %q, want fallback %q", gotEmpty, subURL)
	}
}

func TestResolveEffectiveLanguage_Trial(t *testing.T) {
	tests := []struct {
		name         string
		customer     *database.Customer
		telegramLang string
		want         string
	}{
		{"en lang defaults to my", nil, "en", "my"},
		{"empty lang defaults to my", nil, "", "my"},
		{"ru lang returns ru", nil, "ru", "ru"},
		{"ru-RU lang returns ru", nil, "ru-RU", "ru"},
		{"customer en with telegram en defaults to my", &database.Customer{Language: "en"}, "en", "my"},
		{"customer en with telegram empty defaults to my", &database.Customer{Language: "en"}, "", "my"},
		{"customer en with telegram ru returns ru", &database.Customer{Language: "en"}, "ru", "ru"},
		{"customer my returns my", &database.Customer{Language: "my"}, "en", "my"},
		{"customer ru returns ru", &database.Customer{Language: "ru"}, "en", "ru"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEffectiveLanguage(tt.customer, tt.telegramLang)
			if got != tt.want {
				t.Fatalf("resolveEffectiveLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrialOfferAndSuccessFormatting(t *testing.T) {
	tm := loadHandlerTestTranslations(t)

	// Test English trial offer formatting
	days := 7
	trafficGB := 10
	offerTemplateEn := tm.GetText("en", "trial_offer_text")
	offerEn := fmt.Sprintf(offerTemplateEn, days, trafficGB)
	if !strings.Contains(offerEn, "7 days") || !strings.Contains(offerEn, "10 GB") {
		t.Fatalf("offerEn = %q, want 7 days and 10 GB", offerEn)
	}

	// Test Myanmar trial offer formatting
	offerTemplateMy := tm.GetText("my", "trial_offer_text")
	offerMy := fmt.Sprintf(offerTemplateMy, days, trafficGB)
	if !strings.Contains(offerMy, "7") || !strings.Contains(offerMy, "10") {
		t.Fatalf("offerMy = %q, want 7 days and 10 GB", offerMy)
	}

	// Test trial success message formatting with <code> tag
	subURL := "https://example.com/sub/v2ray"
	escapedURL := html.EscapeString(subURL)
	successTemplateEn := tm.GetText("en", "trial_success_message")
	successEn := fmt.Sprintf(successTemplateEn, escapedURL)
	expectedCode := fmt.Sprintf("<code>%s</code>", escapedURL)
	if !strings.Contains(successEn, expectedCode) {
		t.Fatalf("successEn = %q, want <code>%s</code> block", successEn, expectedCode)
	}

	// Test Myanmar trial success message
	successTemplateMy := tm.GetText("my", "trial_success_message")
	successMy := fmt.Sprintf(successTemplateMy, escapedURL)
	if !strings.Contains(successMy, expectedCode) {
		t.Fatalf("successMy = %q, want <code>%s</code> block", successMy, expectedCode)
	}
}

func TestBuildStartKeyboardWithoutTrial(t *testing.T) {
	restore := config.SetTrialConfigForTesting(0, 0)
	defer restore()

	tm := loadHandlerTestTranslations(t)
	h := Handler{
		translation: tm,
	}

	customer := &database.Customer{
		ID:         1,
		TelegramID: 1234567,
		Language:   "en",
	}

	keyboard := h.buildStartKeyboard(customer, "en")
	for _, row := range keyboard {
		for _, btn := range row {
			if btn.CallbackData == CallbackTrial {
				t.Fatalf("buildStartKeyboard() contains trial button when trialDays is 0")
			}
		}
	}
}

func TestRenderTrialIneligible(t *testing.T) {
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	now := time.Now()
	activeExpire := now.Add(24 * time.Hour)
	expiredExpire := now.Add(-24 * time.Hour)
	subLink := "https://example.com/sub/active123"

	// Case 1: Active subscription with future expiration
	activeCustomer := &database.Customer{
		ID:               1,
		SubscriptionLink: &subLink,
		ExpireAt:         &activeExpire,
	}
	text, markup := h.renderTrialIneligible(activeCustomer, "en")
	if !strings.Contains(text, "already have an active") {
		t.Fatalf("activeCustomer text = %q, want 'already have an active' copy", text)
	}
	if !strings.Contains(text, "<code>https://example.com/sub/active123</code>") {
		t.Fatalf("activeCustomer text = %q, want code block with sub link", text)
	}
	// Markup should include Happ button and connect button
	hasHapp := false
	for _, row := range markup {
		for _, btn := range row {
			if btn.URL == subLink {
				hasHapp = true
			}
		}
	}
	if !hasHapp {
		t.Fatalf("activeCustomer markup missing direct Happ subscription link button")
	}

	// Case 2: Expired subscription link - should NOT be treated as active!
	expiredCustomer := &database.Customer{
		ID:               2,
		SubscriptionLink: &subLink,
		ExpireAt:         &expiredExpire,
	}
	expText, expMarkup := h.renderTrialIneligible(expiredCustomer, "en")
	if strings.Contains(expText, "already have an active") {
		t.Fatalf("expiredCustomer text = %q, should NOT say 'already have an active'", expText)
	}
	if !strings.Contains(expText, "already used your free trial") {
		t.Fatalf("expiredCustomer text = %q, want 'already used your free trial' notice", expText)
	}
	// Expired customer should get Buy button + Back button
	hasBuy := false
	for _, row := range expMarkup {
		for _, btn := range row {
			if btn.CallbackData == CallbackBuy || (btn.WebApp != nil && btn.WebApp.URL != "") {
				hasBuy = true
			}
		}
	}
	if !hasBuy {
		t.Fatalf("expiredCustomer markup missing buy button")
	}

	// Case 3: Used trial with no link
	usedCustomer := &database.Customer{
		ID:          3,
		TrialUsedAt: &now,
	}
	usedText, usedMarkup := h.renderTrialIneligible(usedCustomer, "en")
	if !strings.Contains(usedText, "already used your free trial") {
		t.Fatalf("usedCustomer text = %q, want 'already used your free trial' notice", usedText)
	}
	hasBuy = false
	for _, row := range usedMarkup {
		for _, btn := range row {
			if btn.CallbackData == CallbackBuy || (btn.WebApp != nil && btn.WebApp.URL != "") {
				hasBuy = true
			}
		}
	}
	if !hasBuy {
		t.Fatalf("usedCustomer markup missing buy button")
	}
}

func TestSendActivatedTrialMessage(t *testing.T) {
	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	subURL := "https://example.com/sub/active-key-999"
	err := h.sendActivatedTrialMessage(context.Background(), b, 8888, "en", subURL)
	if err != nil {
		t.Fatalf("sendActivatedTrialMessage() error = %v", err)
	}

	if len(*captured) != 1 {
		t.Fatalf("captured calls = %d, want 1", len(*captured))
	}

	call := (*captured)[0]
	text, _ := call.Body["text"].(string)
	expectedCode := fmt.Sprintf("<code>%s</code>", html.EscapeString(subURL))
	if !strings.Contains(text, expectedCode) {
		t.Fatalf("message text = %q, want %q", text, expectedCode)
	}

	// Verify nil bot error
	err = h.sendActivatedTrialMessage(context.Background(), nil, 8888, "en", subURL)
	if err == nil {
		t.Fatal("sendActivatedTrialMessage() with nil bot want error, got nil")
	}
}

func TestTryAutoActivateAndSendTrial_Table(t *testing.T) {
	tm := loadHandlerTestTranslations(t)

	t.Run("trial days is 0", func(t *testing.T) {
		restore := config.SetTrialConfigForTesting(0, 0)
		defer restore()

		b, captured := newTestBot(t)
		h := Handler{
			translation:    tm,
			paymentService: &payment.PaymentService{},
		}

		activated := h.TryAutoActivateAndSendTrial(context.Background(), b, 123, 123, "user", "en")
		if activated {
			t.Fatal("TryAutoActivateAndSendTrial() = true, want false when trialDays is 0")
		}
		if len(*captured) != 0 {
			t.Fatalf("captured calls = %d, want 0", len(*captured))
		}
	})

	t.Run("nil payment service", func(t *testing.T) {
		restore := config.SetTrialConfigForTesting(7, 10)
		defer restore()

		b, _ := newTestBot(t)
		h := Handler{
			translation:    tm,
			paymentService: nil,
		}

		activated := h.TryAutoActivateAndSendTrial(context.Background(), b, 123, 123, "user", "en")
		if activated {
			t.Fatal("TryAutoActivateAndSendTrial() = true, want false when paymentService is nil")
		}
	})

	t.Run("customer ineligible", func(t *testing.T) {
		restore := config.SetTrialConfigForTesting(7, 10)
		defer restore()

		b, captured := newTestBot(t)
		ps := &payment.PaymentService{}
		ps.SetTestTrialHooks(
			func(ctx context.Context, telegramID int64) (bool, error) {
				return false, nil
			},
			nil,
		)
		h := Handler{
			translation:    tm,
			paymentService: ps,
		}

		activated := h.TryAutoActivateAndSendTrial(context.Background(), b, 123, 123, "user", "en")
		if activated {
			t.Fatal("TryAutoActivateAndSendTrial() = true, want false when ineligible")
		}
		if len(*captured) != 0 {
			t.Fatalf("captured calls = %d, want 0", len(*captured))
		}
	})

	t.Run("eligibility check returns error", func(t *testing.T) {
		restore := config.SetTrialConfigForTesting(7, 10)
		defer restore()

		b, captured := newTestBot(t)
		ps := &payment.PaymentService{}
		ps.SetTestTrialHooks(
			func(ctx context.Context, telegramID int64) (bool, error) {
				return false, errors.New("database connection refused")
			},
			nil,
		)
		h := Handler{
			translation:    tm,
			paymentService: ps,
		}

		activated := h.TryAutoActivateAndSendTrial(context.Background(), b, 123, 123, "user", "en")
		if activated {
			t.Fatal("TryAutoActivateAndSendTrial() = true, want false on eligibility error")
		}
		if len(*captured) != 0 {
			t.Fatalf("captured calls = %d, want 0", len(*captured))
		}
	})

	t.Run("activation returns error", func(t *testing.T) {
		restore := config.SetTrialConfigForTesting(7, 10)
		defer restore()

		b, captured := newTestBot(t)
		ps := &payment.PaymentService{}
		ps.SetTestTrialHooks(
			func(ctx context.Context, telegramID int64) (bool, error) {
				return true, nil
			},
			func(ctx context.Context, telegramID int64) (string, error) {
				return "", errors.New("upstream remnawave error")
			},
		)
		h := Handler{
			translation:    tm,
			paymentService: ps,
		}

		activated := h.TryAutoActivateAndSendTrial(context.Background(), b, 123, 123, "user", "en")
		if activated {
			t.Fatal("TryAutoActivateAndSendTrial() = true, want false on activation error")
		}
		if len(*captured) != 0 {
			t.Fatalf("captured calls = %d, want 0", len(*captured))
		}
	})

	t.Run("successful auto activation", func(t *testing.T) {
		restore := config.SetTrialConfigForTesting(7, 10)
		defer restore()

		b, captured := newTestBot(t)
		subURL := "https://example.com/sub/success-trial-token"
		ps := &payment.PaymentService{}
		ps.SetTestTrialHooks(
			func(ctx context.Context, telegramID int64) (bool, error) {
				return true, nil
			},
			func(ctx context.Context, telegramID int64) (string, error) {
				// Verify username in context
				username, _ := ctx.Value(payment.UsernameCtxKey).(string)
				if username != "myusername" {
					t.Errorf("username in context = %q, want 'myusername'", username)
				}
				return subURL, nil
			},
		)
		h := Handler{
			translation:    tm,
			paymentService: ps,
		}

		activated := h.TryAutoActivateAndSendTrial(context.Background(), b, 999, 999, "myusername", "en")
		if !activated {
			t.Fatal("TryAutoActivateAndSendTrial() = false, want true on success")
		}
		if len(*captured) != 1 {
			t.Fatalf("captured calls = %d, want 1", len(*captured))
		}

		call := (*captured)[0]
		text, _ := call.Body["text"].(string)
		if !strings.Contains(text, "<code>"+subURL+"</code>") {
			t.Fatalf("message text = %q, want code block with %q", text, subURL)
		}
	})

	t.Run("activation succeeds even if send fails", func(t *testing.T) {
		restore := config.SetTrialConfigForTesting(7, 10)
		defer restore()

		ps := &payment.PaymentService{}
		ps.SetTestTrialHooks(
			func(ctx context.Context, telegramID int64) (bool, error) {
				return true, nil
			},
			func(ctx context.Context, telegramID int64) (string, error) {
				return "https://example.com/sub/send-fail-token", nil
			},
		)
		h := Handler{
			translation:    tm,
			paymentService: ps,
		}

		activated := h.TryAutoActivateAndSendTrial(context.Background(), nil, 999, 999, "user", "en")
		if !activated {
			t.Fatal("TryAutoActivateAndSendTrial() = false after ActivateTrial succeeded, want true so callers do not fall through to start menu")
		}
	})
}

func TestTrialResetCommandHandler_RejectsNonAdmin(t *testing.T) {
	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)
	h := Handler{
		translation: tm,
	}

	// Normal user telegram ID, not the admin
	nonAdminID := int64(987654321)
	update := &models.Update{
		Message: &models.Message{
			ID: 10,
			Chat: models.Chat{
				ID: 12345,
			},
			From: &models.User{
				ID:           nonAdminID,
				LanguageCode: "en",
			},
			Text: "/trialreset",
		},
	}

	h.TrialResetCommandHandler(context.Background(), b, update)

	if len(*captured) != 1 {
		t.Fatalf("captured calls = %d, want 1 unauthorized message", len(*captured))
	}
	text, _ := (*captured)[0].Body["text"].(string)
	if !strings.Contains(text, "Unauthorized") && !strings.Contains(text, "admin only") {
		t.Fatalf("expected unauthorized message, got %q", text)
	}
}

func TestTrialResetCommandHandler_AdminSuccess(t *testing.T) {
	adminID := int64(532666374)
	restoreAdmin := config.SetAdminTelegramIdForTesting(adminID)
	defer restoreAdmin()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	// In unit test without real db connection, customerRepository is nil.
	// When repository is nil, it reports customer repository is not configured.
	h := Handler{
		translation: tm,
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 11,
			Chat: models.Chat{
				ID: 12345,
			},
			From: &models.User{
				ID:           adminID,
				LanguageCode: "my",
			},
			Text: "/trialreset",
		},
	}

	h.TrialResetCommandHandler(context.Background(), b, update)

	if len(*captured) != 1 {
		t.Fatalf("captured calls = %d, want 1 message", len(*captured))
	}
	text, _ := (*captured)[0].Body["text"].(string)
	// Without customerRepo, it safely outputs not configured error
	if !strings.Contains(text, "Customer repository is not configured") {
		t.Fatalf("expected repo unconfigured message, got %q", text)
	}
}
