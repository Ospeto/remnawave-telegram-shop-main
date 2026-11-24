package handler

import (
	"path/filepath"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
)

func loadHandlerTestTranslations(t *testing.T) *translation.Manager {
	t.Helper()

	tm := translation.GetInstance()
	if err := tm.InitTranslations(filepath.Join("..", "..", "translations"), "en"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}

	return tm
}

func TestMobilePayResultTextUsesProviderAuthTranslation(t *testing.T) {
	tm := loadHandlerTestTranslations(t)

	enText := mobilePayResultText(tm, "en", "mobile_pay_failed_provider_auth", 0)
	if enText == "mobile_pay_failed_provider_auth" {
		t.Fatal("mobilePayResultText() fell back to translation key for English")
	}
	if !strings.Contains(strings.ToLower(enText), "temporarily unavailable") {
		t.Fatalf("mobilePayResultText() English text = %q, want temporary-unavailable guidance", enText)
	}
	if strings.Contains(strings.ToLower(enText), "openrouter") {
		t.Fatalf("mobilePayResultText() English text leaked provider name: %q", enText)
	}

	myText := mobilePayResultText(tm, "my-MM", "mobile_pay_failed_provider_auth", 0)
	if myText == "mobile_pay_failed_provider_auth" {
		t.Fatal("mobilePayResultText() fell back to translation key for Myanmar locale")
	}
	if myText == enText {
		t.Fatalf("mobilePayResultText() Myanmar locale reused English text: %q", myText)
	}
}

func TestBuildMobilePayKeyboardAddsInChatCancelButton(t *testing.T) {
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	markup := h.buildMobilePayKeyboard("en", 2, "https://mini.example.com/app?source=bot", true, 77)

	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("buildMobilePayKeyboard() rows = %d, want 2", len(markup.InlineKeyboard))
	}
	cancel := markup.InlineKeyboard[0][0]
	if cancel.CallbackData != "cancel_pending_purchase?id=77" {
		t.Fatalf("cancel callback = %q, want an in-chat cancel callback for purchase 77", cancel.CallbackData)
	}
	if cancel.WebApp != nil {
		t.Fatalf("cancel button = %+v, want a callback button, not a WebApp that leaves the chat", cancel)
	}

	back := markup.InlineKeyboard[1][0]
	if back.CallbackData != "sell?plan=2" {
		t.Fatalf("back callback = %q, want sell?plan=2", back.CallbackData)
	}
}

func TestBuildMobilePayKeyboardFallsBackToWebAppWithoutPurchaseID(t *testing.T) {
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	markup := h.buildMobilePayKeyboard("en", 2, "https://mini.example.com/app?source=bot", true, 0)

	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("buildMobilePayKeyboard() rows = %d, want 2", len(markup.InlineKeyboard))
	}
	changePlan := markup.InlineKeyboard[0][0]
	if changePlan.WebApp == nil {
		t.Fatal("change-plan button missing WebApp payload")
	}
	if changePlan.WebApp.URL != "https://mini.example.com/app/plans?source=bot" {
		t.Fatalf("change-plan URL = %q, want plans URL", changePlan.WebApp.URL)
	}
}

func TestBuildMobilePayKeyboardCancelWorksWithoutMiniAppURL(t *testing.T) {
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	// No Mini App configured: cancellation must still be reachable, because the
	// in-chat callback needs no WebApp.
	markup := h.buildMobilePayKeyboard("en", 4, "", true, 88)

	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("buildMobilePayKeyboard() rows = %d, want cancel + back", len(markup.InlineKeyboard))
	}
	if markup.InlineKeyboard[0][0].CallbackData != "cancel_pending_purchase?id=88" {
		t.Fatalf("cancel callback = %q, want the purchase id", markup.InlineKeyboard[0][0].CallbackData)
	}
	if markup.InlineKeyboard[1][0].CallbackData != "sell?plan=4" {
		t.Fatalf("back callback = %q, want sell?plan=4", markup.InlineKeyboard[1][0].CallbackData)
	}
}

func TestMiniAppURLWithPath(t *testing.T) {
	got := miniAppURLWithPath("https://mini.example.com/base/?v=123", "plans")
	if got != "https://mini.example.com/base/plans?v=123" {
		t.Fatalf("miniAppURLWithPath() = %q, want path appended with query preserved", got)
	}
}

func TestMobilePaySuccessTranslationKey(t *testing.T) {
	if got := mobilePaySuccessTranslationKey(database.InvoiceTypeWalletTopUp); got != "mobile_pay_topup_success" {
		t.Fatalf("wallet top-up success key = %q, want mobile_pay_topup_success", got)
	}
	if got := mobilePaySuccessTranslationKey(database.InvoiceTypeMobileBanking); got != "mobile_pay_success" {
		t.Fatalf("mobile banking success key = %q, want mobile_pay_success", got)
	}
}
