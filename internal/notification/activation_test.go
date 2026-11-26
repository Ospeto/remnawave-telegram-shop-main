package notification

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
)

func loadTranslations(t *testing.T) *translation.Manager {
	t.Helper()
	tm := translation.GetInstance()
	if err := tm.InitTranslations(filepath.Join("..", "..", "translations"), "en"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}
	return tm
}

func TestHappImportURL(t *testing.T) {
	restore := config.SetMiniAppURLForTesting("https://shop.example.com/app?x=1")
	defer restore()

	sub := "https://sub.example.com/key"
	got := HappImportURL(sub)
	want := "https://shop.example.com/redirect?sub=" + url.QueryEscape(sub)
	if got != want {
		t.Fatalf("HappImportURL() = %q, want %q", got, want)
	}

	restoreEmpty := config.SetMiniAppURLForTesting("")
	defer restoreEmpty()
	if got := HappImportURL(sub); got != sub {
		t.Fatalf("HappImportURL() without Mini App = %q, want raw %q", got, sub)
	}
}

func TestBuildSubscriptionGuidanceKeyboardIncludesSetupSteps(t *testing.T) {
	tm := loadTranslations(t)
	restoreMini := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restoreMini()

	sub := "https://sub.example.com/key"
	markup := BuildSubscriptionGuidanceKeyboard(tm, "en", sub)

	if len(markup) != 5 {
		t.Fatalf("rows = %d, want 5", len(markup))
	}
	if got, want := markup[0][0].URL, HappImportURL(sub); got != want {
		t.Fatalf("import button URL = %q, want %q", got, want)
	}
	if markup[0][0].Text != tm.GetText("en", "happ_proxy_button") {
		t.Fatalf("import button text = %q", markup[0][0].Text)
	}

	downloads := markup[1]
	if len(downloads) != 2 {
		t.Fatalf("download row buttons = %d, want 2", len(downloads))
	}
	if downloads[0].URL != HappIOSDownloadURL {
		t.Fatalf("iOS URL = %q", downloads[0].URL)
	}
	if downloads[1].URL != HappAndroidDownloadURL {
		t.Fatalf("Android URL = %q", downloads[1].URL)
	}

	connect := markup[2][0]
	if connect.WebApp == nil || connect.WebApp.URL != "https://shop.example.com" {
		t.Fatalf("connect button = %#v, want Mini App web app", connect)
	}

	if got, want := markup[3][0].URL, SupportURL(); got != want {
		t.Fatalf("support URL = %q, want %q", got, want)
	}
	if markup[4][0].CallbackData != "start" {
		t.Fatalf("back callback = %q, want start", markup[4][0].CallbackData)
	}
}

func TestBuildSubscriptionGuidanceKeyboardWithoutMiniAppFallsBackToConnectCallback(t *testing.T) {
	tm := loadTranslations(t)
	restore := config.SetMiniAppURLForTesting("")
	defer restore()

	markup := BuildSubscriptionGuidanceKeyboard(tm, "my", "https://sub.example.com/key")
	connect := markup[2][0]
	if connect.WebApp != nil {
		t.Fatalf("connect button = %#v, want no web app when Mini App URL is unset", connect)
	}
	if connect.CallbackData != "connect" {
		t.Fatalf("connect callback = %q, want connect", connect.CallbackData)
	}
}

func TestSupportURLFallsBackWhenUnset(t *testing.T) {
	restore := config.SetSupportURLForTesting("")
	defer restore()
	if got := SupportURL(); got != DefaultSupportURL {
		t.Fatalf("SupportURL() = %q, want %q", got, DefaultSupportURL)
	}

	restoreCustom := config.SetSupportURLForTesting("https://t.me/custom")
	defer restoreCustom()
	if got := SupportURL(); got != "https://t.me/custom" {
		t.Fatalf("SupportURL() = %q, want the configured value", got)
	}
}

func TestFormatKeyMessageEscapesSubscriptionURL(t *testing.T) {
	tm := loadTranslations(t)
	sub := "https://sub.example.com/a?b=1&c=2"

	for _, lang := range []string{"en", "my"} {
		got := FormatKeyMessage(tm, lang, "key_ready_message", sub)
		if strings.Contains(got, "%!") {
			t.Fatalf("%s: message formatting failed: %q", lang, got)
		}
		if !strings.Contains(got, "&amp;c=2") {
			t.Fatalf("%s: expected escaped URL inside the code block, got %q", lang, got)
		}
		if strings.Contains(got, "<code>https://sub.example.com/a?b=1&c=2</code>") {
			t.Fatalf("%s: raw ampersand reached the HTML payload", lang)
		}
	}
}

func TestKeyReadyMessageExistsInEveryLanguage(t *testing.T) {
	tm := loadTranslations(t)
	for _, lang := range []string{"en", "my"} {
		text := tm.GetText(lang, "key_ready_message")
		if text == "key_ready_message" || strings.TrimSpace(text) == "" {
			t.Fatalf("%s is missing key_ready_message", lang)
		}
		if strings.Count(text, "%s") != 1 {
			t.Fatalf("%s key_ready_message has %d %%s placeholders, want 1", lang, strings.Count(text, "%s"))
		}
	}
	if tm.GetText("en", "key_ready_message") == tm.GetText("en", "trial_success_message") {
		t.Fatal("key_ready_message must not reuse the trial message")
	}
}
