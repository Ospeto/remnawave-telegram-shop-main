package handler

import (
	"context"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/internal/config"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
)

func (h Handler) ConnectCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		// U9: the flow used to log and return, so the customer saw nothing after
		// tapping a button that clearly did something.
		h.sendErrorReply(ctx, b, update.Message.Chat.ID, resolveEffectiveLanguage(nil, update.Message.From.LanguageCode),
			translationKeyGenericError, err)
		return
	}
	if customer == nil {
		h.sendErrorReply(ctx, b, update.Message.Chat.ID, resolveEffectiveLanguage(nil, update.Message.From.LanguageCode),
			translationKeyGenericError, fmt.Errorf("no customer row for telegram id %s", utils.MaskHalfInt64(update.Message.Chat.ID)))
		h.sendMissingCustomerHint(ctx, b, update.Message.Chat.ID, update.Message.From.LanguageCode)
		return
	}
	h.applyCanonicalConnectState(ctx, customer)

	langCode := resolveEffectiveLanguage(customer, update.Message.From.LanguageCode)
	h.sendConnectionMessage(ctx, b, update.Message.Chat.ID, customer, langCode)
}

// sendMissingCustomerHint follows a "we could not find your account" error with
// the one action that fixes it: starting the bot, which creates the customer row.
func (h Handler) sendMissingCustomerHint(ctx context.Context, b *bot.Bot, chatID int64, telegramLang string) {
	if b == nil {
		return
	}
	langCode := resolveEffectiveLanguage(nil, telegramLang)
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   h.translation.GetText(langCode, "connect_no_customer_hint"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	}); err != nil {
		slog.Error("Error sending missing-customer hint", "error", err)
	}
}

// connectMarkup builds the /connect keyboard. The Mini App stays the primary
// action; the one-click import is offered next to it whenever there is a live
// SubscriptionLink, because the walkthrough text now always explains it.
func (h Handler) connectMarkup(customer *database.Customer, langCode string) [][]models.InlineKeyboardButton {
	var markup [][]models.InlineKeyboardButton

	hasLiveLink := customer != nil && customer.SubscriptionLink != nil &&
		*customer.SubscriptionLink != "" && customer.ExpireAt != nil && customer.ExpireAt.After(time.Now())

	if config.GetMiniAppURL() != "" {
		markup = append(markup, []models.InlineKeyboardButton{{
			Text:   h.translation.GetText(langCode, "connect_button"),
			WebApp: &models.WebAppInfo{URL: config.GetMiniAppURL()},
		}})
	} else if config.IsWebAppLinkEnabled() && hasLiveLink {
		markup = append(markup, []models.InlineKeyboardButton{{
			Text:   h.translation.GetText(langCode, "connect_button"),
			WebApp: &models.WebAppInfo{URL: *customer.SubscriptionLink},
		}})
	}

	if hasLiveLink {
		markup = append(markup, []models.InlineKeyboardButton{{
			Text: h.translation.GetText(langCode, "happ_proxy_button"),
			URL:  buildOneClickHappURL(*customer.SubscriptionLink),
		}})
	}

	markup = append(markup, []models.InlineKeyboardButton{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}})

	return markup
}

func (h Handler) sendConnectionMessage(ctx context.Context, b *bot.Bot, chatID int64, customer *database.Customer, langCode string) {
	if b == nil {
		return
	}
	langCode = resolveEffectiveLanguage(customer, langCode)
	markup := h.connectMarkup(customer, langCode)

	isDisabled := true
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      buildConnectText(customer, langCode),
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: markup,
		},
	})

	if err != nil {
		slog.Error("Error sending connect message", "error", err)
	}
}

func (h Handler) ConnectCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message

	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		h.editErrorReply(ctx, b, callback.Chat.ID, callback.ID, resolveEffectiveLanguage(nil, update.CallbackQuery.From.LanguageCode),
			translationKeyGenericError, err)
		return
	}
	if customer == nil {
		h.editErrorReply(ctx, b, callback.Chat.ID, callback.ID, resolveEffectiveLanguage(nil, update.CallbackQuery.From.LanguageCode),
			"connect_no_customer", fmt.Errorf("no customer row for telegram id %s", utils.MaskHalfInt64(callback.Chat.ID)))
		return
	}
	h.applyCanonicalConnectState(ctx, customer)

	langCode := resolveEffectiveLanguage(customer, update.CallbackQuery.From.LanguageCode)
	markup := h.connectMarkup(customer, langCode)

	isDisabled := true
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		Text:      buildConnectText(customer, langCode),
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: markup,
		},
	})

	if err != nil {
		slog.Error("Error sending connect message", "error", err)
	}
}

// formatExpiryDateTime renders an expiry in the customer's own date convention.
//
// U9: the connect screen hardcoded 02.01.2006 15:04 for every language, so an
// English reader saw "03.04.2026" and had to guess whether that was March or
// April. English now gets an unambiguous month name; Myanmar keeps the day-first
// numeric form its readers expect.
func formatExpiryDateTime(expireAt time.Time, langCode string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(langCode)), "en") {
		return expireAt.Format("Jan 2, 2006 15:04")
	}
	return expireAt.Format("02.01.2006 15:04")
}

func buildConnectText(customer *database.Customer, langCode string) string {
	var info strings.Builder

	tm := translation.GetInstance()

	if customer.ExpireAt != nil {
		currentTime := time.Now()

		if currentTime.Before(*customer.ExpireAt) {
			// U9: the expiry used to be rendered with one hardcoded German-style
			// layout (02.01.2006), which an English reader parses as month-first.
			formattedDate := formatExpiryDateTime(*customer.ExpireAt, langCode)

			subscriptionActiveText := tm.GetText(langCode, "subscription_active")
			info.WriteString(fmt.Sprintf(subscriptionActiveText, formattedDate))

			if customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
				// The walkthrough used to render only when the Mini App URL was empty,
				// so the one command a stuck paying customer reaches for stopped
				// teaching as soon as the Mini App was configured. The key and the
				// steps are the answer to "/connect", so they always render; the
				// keyboard still leads with the Mini App.
				subscriptionLinkText := tm.GetText(langCode, "subscription_link")
				info.WriteString(fmt.Sprintf(subscriptionLinkText, *customer.SubscriptionLink))
				info.WriteString(tm.GetText(langCode, "happ_proxy_instructions"))
			}
		} else {
			noSubscriptionText := tm.GetText(langCode, "no_subscription")
			info.WriteString(noSubscriptionText)
		}
	} else {
		noSubscriptionText := tm.GetText(langCode, "no_subscription")
		info.WriteString(noSubscriptionText)
	}

	return info.String()
}

func (h Handler) applyCanonicalConnectState(ctx context.Context, customer *database.Customer) {
	if h.subKeyRepo == nil || customer == nil {
		return
	}

	keys, err := h.subKeyRepo.FindByCustomerID(ctx, customer.ID)
	if err != nil {
		slog.Error("Error loading subscription keys for connect flow", "customer_id", customer.ID, "error", err)
		return
	}
	if len(keys) == 0 {
		return
	}

	primaryKey := database.PrimarySubscriptionKey(keys)
	if primaryKey == nil {
		customer.SubscriptionLink = nil
		customer.ExpireAt = nil
		return
	}

	if strings.TrimSpace(primaryKey.SubscriptionURL) == "" {
		customer.SubscriptionLink = nil
	} else {
		link := primaryKey.SubscriptionURL
		customer.SubscriptionLink = &link
	}
	customer.ExpireAt = primaryKey.ExpireAt
}
