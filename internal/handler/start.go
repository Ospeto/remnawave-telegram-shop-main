package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func (h Handler) StartCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, created, err := h.ensureCustomer(ctx, update.Message.Chat.ID, update.Message.From.LanguageCode)
	if err != nil {
		return
	}

	if created {
		h.processReferral(ctx, update.Message.Text, customer)
	}
	h.sendStartMenu(ctx, b, update.Message.Chat.ID, customer, update.Message.From.LanguageCode)
}

func (h Handler) ensureCustomer(ctx context.Context, telegramID int64, langCode string) (*database.Customer, bool, error) {
	existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return nil, false, err
	}

	if existingCustomer == nil {
		ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: telegramID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer", "error", err)
			return nil, false, err
		}
		return existingCustomer, true, nil
	} else {
		updates := map[string]interface{}{
			"language": langCode,
		}
		err = h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates)
		if err != nil {
			slog.Error("Error updating customer", "error", err)
			return nil, false, err
		}
	}

	return existingCustomer, false, nil
}

func (h Handler) processReferral(ctx context.Context, messageText string, existingCustomer *database.Customer) {
	slog.Info("[REFERRAL-DEBUG] /start handler entry",
		"message_text", messageText,
		"customer_id", existingCustomer.ID,
		"telegram_id", existingCustomer.TelegramID,
		"contains_ref", strings.Contains(messageText, "ref_"),
	)

	if !strings.Contains(messageText, "ref_") {
		slog.Info("[REFERRAL-DEBUG] No ref_ in message, skipping referral logic")
		return
	}

	parts := strings.Split(messageText, " ")
	slog.Info("[REFERRAL-DEBUG] ref_ detected in message", "parts_count", len(parts), "parts", parts)

	if len(parts) <= 1 {
		slog.Warn("[REFERRAL-DEBUG] SKIPPED: referral link malformed, no space-separated argument", "text", messageText)
		return
	}

	arg := parts[1]
	slog.Info("[REFERRAL-DEBUG] parsing argument", "arg", arg, "has_ref_prefix", strings.HasPrefix(arg, "ref_"))

	if !strings.HasPrefix(arg, "ref_") {
		return
	}

	code := strings.TrimPrefix(arg, "ref_")
	referrerId, err := strconv.ParseInt(code, 10, 64)
	if err != nil {
		slog.Error("[REFERRAL-DEBUG] FAILED: error parsing referrer id", "code", code, "error", err)
		return
	}

	if referrerId == existingCustomer.TelegramID {
		slog.Warn("[REFERRAL-DEBUG] BLOCKED: self-referral attempt", "telegram_id", referrerId)
		return
	}

	slog.Info("[REFERRAL-DEBUG] Valid referrer ID parsed", "referrer_telegram_id", referrerId, "referee_telegram_id", existingCustomer.TelegramID)

	// Check if user already has a referral (prevents duplicate attempts)
	existingRef, refErr := h.referralRepository.FindByReferee(ctx, existingCustomer.TelegramID)
	slog.Info("[REFERRAL-DEBUG] FindByReferee result", "existing_ref", existingRef, "error", refErr)

	if existingRef != nil {
		slog.Info("[REFERRAL-DEBUG] SKIPPED: referral already exists for this user",
			"referral_id", existingRef.ID,
			"existing_referrer", existingRef.ReferrerID,
			"bonus_granted", existingRef.BonusGranted,
			"referee_bonus_granted", existingRef.RefereeBonusGranted,
		)
		return
	}

	// No existing referral — eligible! Verify referrer exists.
	referrer, referrerErr := h.customerRepository.FindByTelegramId(ctx, referrerId)
	slog.Info("[REFERRAL-DEBUG] Referrer lookup result",
		"referrer_telegram_id", referrerId,
		"referrer_found", referrer != nil,
		"error", referrerErr,
	)

	if referrerErr != nil || referrer == nil {
		slog.Warn("[REFERRAL-DEBUG] SKIPPED: referrer not found in database", "referrer_telegram_id", referrerId)
		return
	}

	ctxRef := context.WithoutCancel(ctx)
	createdRef, createErr := h.referralRepository.Create(ctxRef, referrerId, existingCustomer.TelegramID)
	if createErr != nil {
		slog.Error("[REFERRAL-DEBUG] FAILED: error creating referral", "error", createErr)
	} else {
		slog.Info("[REFERRAL-DEBUG] SUCCESS: referral created!",
			"referral_id", createdRef.ID,
			"referrer_telegram_id", referrerId,
			"referee_telegram_id", existingCustomer.TelegramID,
		)
	}
}

func (h Handler) sendStartMenu(ctx context.Context, b *bot.Bot, chatID int64, customer *database.Customer, langCode string) {
	inlineKeyboard := h.buildStartKeyboard(customer, langCode)

	m, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "🧹",
		ReplyMarkup: models.ReplyKeyboardRemove{
			RemoveKeyboard: true,
		},
	})

	if err == nil {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: m.ID,
		})
	} else {
		slog.Error("Error sending removing reply keyboard", "error", err)
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText(langCode, "greeting"),
	})
	if err != nil {
		slog.Error("Error sending /start message", "error", err)
	}
}

func (h Handler) StartCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	callback := update.CallbackQuery

	existingCustomer, err := h.customerRepository.FindByTelegramId(ctxWithTime, callback.From.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}
	if existingCustomer == nil {
		slog.Error("customer not found for start callback", "telegram_id", callback.From.ID)
		return
	}

	langCode := callback.From.LanguageCode
	inlineKeyboard := h.buildStartKeyboard(existingCustomer, langCode)

	_, err = b.EditMessageText(ctxWithTime, &bot.EditMessageTextParams{
		ChatID:    callback.Message.Message.Chat.ID,
		MessageID: callback.Message.Message.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText(langCode, "greeting"),
	})
	if err != nil {
		slog.Error("Error sending /start message", "error", err)
	}
}

func (h Handler) resolveConnectButton(lang string) []models.InlineKeyboardButton {
	var inlineKeyboard []models.InlineKeyboardButton

	if config.GetMiniAppURL() != "" {
		inlineKeyboard = []models.InlineKeyboardButton{
			{Text: h.translation.GetText(lang, "connect_button"), WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			}},
		}
	} else {
		inlineKeyboard = []models.InlineKeyboardButton{
			{Text: h.translation.GetText(lang, "connect_button"), CallbackData: CallbackConnect},
		}
	}
	return inlineKeyboard
}

func (h Handler) buildStartKeyboard(existingCustomer *database.Customer, langCode string) [][]models.InlineKeyboardButton {
	shareURL := "https://t.me/share/url?url=" + config.BotURL() + "?start=ref_" + strconv.FormatInt(existingCustomer.TelegramID, 10)
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

	return [][]models.InlineKeyboardButton{
		{
			buyButton,
		},
		{
			{
				Text: h.translation.GetText(langCode, "referral_button"),
				URL:  shareURL,
			},
		},
	}
}
