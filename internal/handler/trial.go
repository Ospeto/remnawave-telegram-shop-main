package handler

import (
	"context"
	"errors"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/payment"
)

func (h Handler) TrialCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if config.TrialDays() == 0 {
		return
	}
	eligible, err := h.paymentService.CanActivateTrial(ctx, update.CallbackQuery.From.ID)
	if err != nil {
		slog.Error("Error checking trial eligibility", "error", err)
		return
	}
	if !eligible {
		return
	}
	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		Text:      h.translation.GetText(langCode, "trial_text"),
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
	_, err := h.paymentService.ActivateTrial(ctxWithUsername, update.CallbackQuery.From.ID)
	if err != nil {
		if errors.Is(err, payment.ErrTrialAlreadyUsed) || errors.Is(err, payment.ErrCustomerNotFound) {
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
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      callback.Chat.ID,
		MessageID:   callback.ID,
		Text:        h.translation.GetText(langCode, "trial_activated"),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: h.createConnectKeyboard(langCode)},
	})
	if err != nil {
		slog.Error("Error sending /trial message", "error", err)
	}
}

func (h Handler) createConnectKeyboard(lang string) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton
	inlineCustomerKeyboard = append(inlineCustomerKeyboard, h.resolveConnectButton(lang))

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(lang, "back_button"), CallbackData: CallbackStart},
	})
	return inlineCustomerKeyboard
}
