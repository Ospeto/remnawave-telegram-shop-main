package handler

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

func (h Handler) renderTrialIneligible(customer *database.Customer, langCode string) (string, [][]models.InlineKeyboardButton) {
	hasActiveSub := customer != nil && customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" &&
		customer.ExpireAt != nil && customer.ExpireAt.After(time.Now())

	if hasActiveSub {
		var subURL string
		if customer.SubscriptionLink != nil {
			subURL = *customer.SubscriptionLink
		}
		escapedURL := html.EscapeString(subURL)
		notice := h.translation.GetText(langCode, "trial_already_active")
		text := fmt.Sprintf("%s\n\n<code>%s</code>", notice, escapedURL)
		markup := h.buildDirectSubscriptionKeyboard(langCode, subURL)
		return text, markup
	}

	text := h.translation.GetText(langCode, "trial_already_used_notice")
	var buyButton models.InlineKeyboardButton
	if config.GetMiniAppURL() != "" {
		buyButton = models.InlineKeyboardButton{
			Text: h.translation.GetText(langCode, "buy_button"),
			WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			},
		}
	} else {
		buyButton = models.InlineKeyboardButton{
			Text:         h.translation.GetText(langCode, "buy_button"),
			CallbackData: CallbackBuy,
		}
	}

	markup := [][]models.InlineKeyboardButton{
		{buyButton},
		{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
	}
	return text, markup
}

func (h Handler) TrialCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}
	chatID := update.Message.Chat.ID
	telegramID := update.Message.From.ID
	langCode := update.Message.From.LanguageCode

	if config.TrialDays() == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      h.translation.GetText(langCode, "trial_failed"),
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			}},
		})
		if err != nil {
			slog.Error("Error sending trial unavailable message", "error", err)
		}
		return
	}

	eligible, err := h.paymentService.CanActivateTrial(ctx, telegramID)
	if err != nil {
		slog.Error("Error checking trial eligibility", "error", err)
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      h.translation.GetText(langCode, "trial_failed"),
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			}},
		})
		if err != nil {
			slog.Error("Error sending trial error message", "error", err)
		}
		return
	}

	if eligible {
		trafficGB := config.TrialTrafficLimit() / (1024 * 1024 * 1024)
		text := fmt.Sprintf(h.translation.GetText(langCode, "trial_offer_text"), config.TrialDays(), trafficGB)
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "activate_trial_button"), CallbackData: CallbackActivateTrial}},
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			}},
		})
		if err != nil {
			slog.Error("Error sending trial offer message", "error", err)
		}
		return
	}

	// User not eligible — either already has active subscription/trial or already used trial
	customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Error finding customer for trial command", "error", err)
	}
	if customer != nil {
		h.applyCanonicalConnectState(ctx, customer)
	}

	text, markup := h.renderTrialIneligible(customer, langCode)
	isDisabled := true
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: markup},
	})
	if err != nil {
		slog.Error("Error sending trial ineligible message", "error", err)
	}
}

func (h Handler) TrialCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if config.TrialDays() == 0 {
		return
	}
	eligible, err := h.paymentService.CanActivateTrial(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error checking trial eligibility", "error", err)
		return
	}
	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode

	if !eligible {
		// If not eligible, show existing subscription or used notice
		telegramID := update.CallbackQuery.From.ID
		customer, _ := h.customerRepository.FindByTelegramId(ctx, telegramID)
		if customer != nil {
			h.applyCanonicalConnectState(ctx, customer)
		}
		text, markup := h.renderTrialIneligible(customer, langCode)
		isDisabled := true
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callback.Chat.ID,
			MessageID: callback.ID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{
				IsDisabled: &isDisabled,
			},
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: markup},
		})
		return
	}

	trafficGB := config.TrialTrafficLimit() / (1024 * 1024 * 1024)
	text := fmt.Sprintf(h.translation.GetText(langCode, "trial_offer_text"), config.TrialDays(), trafficGB)
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: h.translation.GetText(langCode, "activate_trial_button"), CallbackData: CallbackActivateTrial}},
			{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
		}},
	})
	if err != nil {
		slog.Error("Error sending /trial message", "error", err)
	}
}

func (h Handler) ActivateTrialCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if config.TrialDays() == 0 {
		return
	}
	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	ctxWithUsername := context.WithValue(ctx, payment.UsernameCtxKey, update.CallbackQuery.From.Username)
	subURL, err := h.paymentService.ActivateTrial(ctxWithUsername, update.CallbackQuery.From.ID)
	if err != nil {
		if errors.Is(err, payment.ErrTrialAlreadyUsed) || errors.Is(err, payment.ErrCustomerNotFound) {
			telegramID := update.CallbackQuery.From.ID
			customer, _ := h.customerRepository.FindByTelegramId(ctx, telegramID)
			if customer != nil {
				h.applyCanonicalConnectState(ctx, customer)
			}
			text, markup := h.renderTrialIneligible(customer, langCode)
			isDisabled := true
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    callback.Chat.ID,
				MessageID: callback.ID,
				Text:      text,
				ParseMode: models.ParseModeHTML,
				LinkPreviewOptions: &models.LinkPreviewOptions{
					IsDisabled: &isDisabled,
				},
				ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: markup},
			})
			return
		}
		slog.Error("Error activating trial", "error", err)
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callback.Chat.ID,
			MessageID: callback.ID,
			Text:      h.translation.GetText(langCode, "trial_failed"),
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			}},
		})
		if err != nil {
			slog.Error("Error sending trial failure message", "error", err)
		}
		return
	}

	escapedURL := html.EscapeString(subURL)
	successText := fmt.Sprintf(h.translation.GetText(langCode, "trial_success_message"), escapedURL)
	markup := h.buildDirectSubscriptionKeyboard(langCode, subURL)

	isDisabled := true
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      callback.Chat.ID,
		MessageID:   callback.ID,
		Text:        successText,
		ParseMode:   models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: markup},
	})
	if err != nil {
		slog.Error("Error sending trial activated message", "error", err)
	}
}

func (h Handler) buildDirectSubscriptionKeyboard(lang string, subURL string) [][]models.InlineKeyboardButton {
	var markup [][]models.InlineKeyboardButton

	if subURL != "" {
		markup = append(markup, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(lang, "happ_proxy_button"), URL: subURL},
		})
	}

	markup = append(markup, h.resolveConnectButton(lang))

	markup = append(markup, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(lang, "back_button"), CallbackData: CallbackStart},
	})
	return markup
}

func (h Handler) createConnectKeyboard(lang string) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton
	inlineCustomerKeyboard = append(inlineCustomerKeyboard, h.resolveConnectButton(lang))

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(lang, "back_button"), CallbackData: CallbackStart},
	})
	return inlineCustomerKeyboard
}
