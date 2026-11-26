package handler

import (
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

// TestGreetingOnlyPromisesATrialThatCanActivate pins the shipped-default defect:
// .env.sample ships TRIAL_DAYS=0, and the greeting advertised a free trial anyway.
func TestGreetingOnlyPromisesATrialThatCanActivate(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}

	restoreOff := config.SetTrialConfigForTesting(0, 0)
	trialOff := h.buildGreeting("en")
	restoreOff()

	restoreOn := config.SetTrialConfigForTesting(7, 10)
	trialOn := h.buildGreeting("en")
	restoreOn()

	if strings.Contains(trialOff, h.translation.GetText("en", "trial_button")) {
		t.Fatalf("greeting with TRIAL_DAYS=0 still advertises the trial button: %q", trialOff)
	}
	if strings.Contains(strings.ToLower(trialOff), "free trial") {
		t.Fatalf("greeting with TRIAL_DAYS=0 still promises a free trial: %q", trialOff)
	}
	if !strings.Contains(trialOn, "7-day") {
		t.Fatalf("greeting with TRIAL_DAYS=7 = %q, want the configured trial length", trialOn)
	}
}

// TestGreetingUsesTheConfiguredReferralAmount proves the amount is formatted from
// configuration rather than hard-coded into prose.
func TestGreetingUsesTheConfiguredReferralAmount(t *testing.T) {
	original := payment.GetReferralBonusAmount()
	t.Cleanup(func() { payment.SetReferralBonusAmount(original) })

	h := Handler{translation: loadHandlerTestTranslations(t)}

	payment.SetReferralBonusAmount(2500)
	text := h.buildGreeting("en")
	if !strings.Contains(text, "2,500") {
		t.Fatalf("greeting = %q, want the configured referral amount 2,500", text)
	}
	if strings.Contains(text, "1000") {
		t.Fatalf("greeting still carries the hard-coded amount: %q", text)
	}
}

// TestGreetingDropsTheUnsupportedServerClaim keeps the bot from restating a
// promise the panel cannot support.
func TestGreetingDropsTheUnsupportedServerClaim(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}

	restore := config.SetTrialConfigForTesting(7, 10)
	defer restore()

	for _, lang := range []string{"en", "my"} {
		text := h.buildGreeting(lang)
		if strings.Contains(text, "5 stable servers") || strings.Contains(text, "server ၅ ခု") {
			t.Fatalf("%s greeting still claims a fixed server count: %q", lang, text)
		}
	}
}

// TestGreetingOrientesTheCustomerAtTheHandoff is the Unit 3 handoff test: the bot
// explains what happens after the tap, mirroring the Mini App's step_1..step_4.
func TestGreetingOrientsTheCustomerAtTheHandoff(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}

	for _, lang := range []string{"en", "my"} {
		text := h.buildGreeting(lang)
		for _, key := range []string{"greeting_ready", "greeting_steps", "greeting_referral", "greeting_cta"} {
			copy := h.translation.GetText(lang, key)
			if copy == key {
				t.Fatalf("%s is missing the %s key", lang, key)
			}
		}
		// The rendered greeting must carry no leftover formatting verbs.
		if strings.Contains(text, "%s") || strings.Contains(text, "%d") {
			t.Fatalf("%s greeting carries an unformatted placeholder: %q", lang, text)
		}
		if !strings.Contains(text, "Happ Proxy") {
			t.Fatalf("%s greeting does not name what to have ready: %q", lang, text)
		}
	}
}

// TestConnectAlwaysTeachesTheHappSetup is the Unit 3 /connect test.
//
// buildConnectText rendered the walkthrough only when the Mini App URL was empty,
// so the one command a stuck paying customer reaches for stopped teaching as soon
// as the Mini App was configured.
func TestConnectAlwaysTeachesTheHappSetup(t *testing.T) {
	restore := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restore()

	expireAt := time.Now().Add(20 * 24 * time.Hour)
	subURL := "https://sub.example.com/key"
	customer := &database.Customer{ID: 1, TelegramID: 42, ExpireAt: &expireAt, SubscriptionLink: &subURL}

	text := buildConnectText(customer, "en")

	if !strings.Contains(text, subURL) {
		t.Fatalf("connect text = %q, want the key it is supposed to teach", text)
	}
	if !strings.Contains(text, translationText(t, "en", "happ_proxy_instructions")) {
		t.Fatalf("connect text = %q, want the Happ walkthrough even with the Mini App configured", text)
	}
}

// TestConnectKeyboardLeadsWithTheMiniApp keeps the Mini App the primary action
// while the walkthrough teaches it.
func TestConnectKeyboardLeadsWithTheMiniApp(t *testing.T) {
	restore := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restore()

	expireAt := time.Now().Add(20 * 24 * time.Hour)
	subURL := "https://sub.example.com/key"
	customer := &database.Customer{ID: 1, TelegramID: 42, ExpireAt: &expireAt, SubscriptionLink: &subURL}

	h := Handler{translation: loadHandlerTestTranslations(t)}
	markup := h.connectMarkup(customer, "en")

	if len(markup) < 2 {
		t.Fatalf("connect keyboard rows = %d, want the app plus the import", len(markup))
	}
	if markup[0][0].WebApp == nil || markup[0][0].WebApp.URL != "https://shop.example.com" {
		t.Fatalf("first row = %#v, want the Mini App as the primary action", markup[0][0])
	}

	var importURL string
	for _, row := range markup {
		for _, button := range row {
			if button.Text == h.translation.GetText("en", "happ_proxy_button") {
				importURL = button.URL
			}
		}
	}
	if !strings.Contains(importURL, "sub=") {
		t.Fatalf("import button url = %q, want the one-click import link", importURL)
	}
}

// TestConnectKeyboardOmitsImportWithoutALiveKey keeps a button from pointing at a
// dead key.
func TestConnectKeyboardOmitsImportWithoutALiveKey(t *testing.T) {
	restore := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restore()

	expired := time.Now().Add(-24 * time.Hour)
	subURL := "https://sub.example.com/key"

	h := Handler{translation: loadHandlerTestTranslations(t)}

	for _, customer := range []*database.Customer{
		{ID: 1, TelegramID: 42, ExpireAt: &expired, SubscriptionLink: &subURL},
		{ID: 2, TelegramID: 42},
	} {
		markup := h.connectMarkup(customer, "en")
		for _, row := range markup {
			for _, button := range row {
				if button.Text == h.translation.GetText("en", "happ_proxy_button") {
					t.Fatalf("customer %d got an import button without a live key", customer.ID)
				}
			}
		}
	}
}

// TestStartMenuCarriesTheSupportDoor is the captain's addition: a customer who
// needs a human must find that door on the first screen, and the button must open
// a real support contact rather than the bot they are already talking to.
func TestStartMenuCarriesTheSupportDoor(t *testing.T) {
	tm := loadHandlerTestTranslations(t)
	h := Handler{translation: tm}

	customer := &database.Customer{ID: 1, TelegramID: 42, Language: "en"}

	findSupport := func(lang string) (text, url string) {
		for _, row := range h.buildStartKeyboard(customer, lang) {
			for _, button := range row {
				if strings.Contains(button.Text, "Support") || button.Text == tm.GetText("my", "support_button") {
					return button.Text, button.URL
				}
			}
		}
		return "", ""
	}

	// Shipped default: no SUPPORT_URL configured, so the fallback must still work.
	defaultText, defaultURL := findSupport("en")
	if defaultText == "" {
		t.Fatal("start keyboard has no support button")
	}
	if defaultURL != config.DefaultSupportURL {
		t.Fatalf("support button url = %q, want %q", defaultURL, config.DefaultSupportURL)
	}

	// A configured SUPPORT_URL wins on the very same button.
	restore := config.SetSupportURLForTesting("https://t.me/shophelpdesk")
	defer restore()

	if _, configuredURL := findSupport("en"); configuredURL != "https://t.me/shophelpdesk" {
		t.Fatalf("configured support button url = %q, want the SUPPORT_URL value", configuredURL)
	}

	// The Burmese keyboard carries the door too, in Burmese.
	burmeseText, burmeseURL := findSupport("my")
	if burmeseURL != "https://t.me/shophelpdesk" {
		t.Fatalf("Burmese support button url = %q, want the configured SUPPORT_URL", burmeseURL)
	}
	if burmeseText != tm.GetText("my", "support_button") {
		t.Fatalf("Burmese support button text = %q, want %q", burmeseText, tm.GetText("my", "support_button"))
	}
}

func translationText(t *testing.T, lang, key string) string {
	t.Helper()
	return loadHandlerTestTranslations(t).GetText(lang, key)
}
