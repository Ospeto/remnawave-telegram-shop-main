package handler

import (
	"fmt"
	"html"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func TestBuildDirectSubscriptionKeyboard(t *testing.T) {
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	subURL := "https://example.com/sub/token123"
	markup := h.buildDirectSubscriptionKeyboard("en", subURL)

	if len(markup) < 2 {
		t.Fatalf("buildDirectSubscriptionKeyboard() rows = %d, want at least 2", len(markup))
	}

	// First row should be Happ Proxy direct link
	happRow := markup[0]
	if len(happRow) != 1 {
		t.Fatalf("first row button count = %d, want 1", len(happRow))
	}
	happBtn := happRow[0]
	if happBtn.URL != subURL {
		t.Fatalf("happBtn.URL = %q, want %q", happBtn.URL, subURL)
	}
	if !strings.Contains(happBtn.Text, "Happ") {
		t.Fatalf("happBtn.Text = %q, want Happ proxy text", happBtn.Text)
	}

	// Last row should be back button
	backRow := markup[len(markup)-1]
	if len(backRow) != 1 || backRow[0].CallbackData != CallbackStart {
		t.Fatalf("backRow = %#v, want back button with CallbackStart", backRow)
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
