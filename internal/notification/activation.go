package notification

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/translation"
)

// DefaultSupportURL mirrors config.DefaultSupportURL so callers that already
// import this package do not have to reach into config for the fallback.
const DefaultSupportURL = config.DefaultSupportURL

// Download URLs for the Happ proxy client.
const (
	HappIOSDownloadURL     = "https://apps.apple.com/us/app/happ-proxy-utility/id6504287215"
	HappAndroidDownloadURL = "https://sub.wavypremium.xyz/downloads/Happ.apk"
)

// HappImportURL rewrites the Mini App URL into the one-click Happ import
// redirect. Without a configured Mini App it returns the raw subscription URL,
// which Happ still imports from the clipboard.
func HappImportURL(subURL string) string {
	baseURL := strings.TrimSpace(config.GetMiniAppURL())
	if baseURL == "" || subURL == "" {
		return subURL
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return subURL
	}

	parsed.RawQuery = ""
	parsed.Path = "/redirect"
	query := url.Values{}
	query.Set("sub", subURL)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

// SupportURL resolves the operator's support contact with a working fallback.
// The fallback lives in config so every surface resolves it identically.
func SupportURL() string {
	return config.SupportURL()
}

// FormatKeyMessage renders a key-bearing message from a translation key. The
// subscription URL is inserted escaped, so it is safe inside <code>.
func FormatKeyMessage(tm *translation.Manager, lang, translationKey, subURL string) string {
	return fmt.Sprintf(tm.GetText(lang, translationKey), html.EscapeString(subURL))
}

// BuildSubscriptionGuidanceKeyboard is the keyboard a customer needs to start
// using the key they were just handed: one-click import, both app downloads,
// a way back into the product, support, and a route to the start menu.
//
// Shared by the trial message and the paid activation message so a paying
// customer is never given less guidance than a trial customer.
func BuildSubscriptionGuidanceKeyboard(tm *translation.Manager, lang, subURL string) [][]models.InlineKeyboardButton {
	var markup [][]models.InlineKeyboardButton

	if subURL != "" {
		markup = append(markup, []models.InlineKeyboardButton{
			{Text: tm.GetText(lang, "happ_proxy_button"), URL: HappImportURL(subURL)},
		})
	}

	markup = append(markup, []models.InlineKeyboardButton{
		{Text: tm.GetText(lang, "download_happ_ios"), URL: HappIOSDownloadURL},
		{Text: tm.GetText(lang, "download_happ_android"), URL: HappAndroidDownloadURL},
	})

	markup = append(markup, []models.InlineKeyboardButton{connectButton(tm, lang)})

	markup = append(markup, []models.InlineKeyboardButton{
		{Text: tm.GetText(lang, "trial_support_button"), URL: SupportURL()},
	})

	markup = append(markup, []models.InlineKeyboardButton{
		{Text: tm.GetText(lang, "back_button"), CallbackData: "start"},
	})
	return markup
}

// connectButton opens the Mini App when it is configured, and otherwise asks
// the bot for connection details in the chat.
func connectButton(tm *translation.Manager, lang string) models.InlineKeyboardButton {
	if miniAppURL := strings.TrimSpace(config.GetMiniAppURL()); miniAppURL != "" {
		return models.InlineKeyboardButton{
			Text:   tm.GetText(lang, "connect_button"),
			WebApp: &models.WebAppInfo{URL: miniAppURL},
		}
	}
	return models.InlineKeyboardButton{
		Text:         tm.GetText(lang, "connect_button"),
		CallbackData: "connect",
	}
}
