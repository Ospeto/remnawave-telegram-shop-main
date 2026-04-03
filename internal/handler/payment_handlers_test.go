package handler

import (
	"path/filepath"
	"strings"
	"testing"

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
