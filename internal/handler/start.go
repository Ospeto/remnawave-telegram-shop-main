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
	ctxWithTime, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	langCode := update.Message.From.LanguageCode
	existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, update.Message.Chat.ID)
	if err != nil {
		slog.Error("error finding customer by telegram id", "error", err)
		return
	}

	if existingCustomer == nil {
		existingCustomer, err = h.customerRepository.Create(ctxWithTime, &database.Customer{
			TelegramID: update.Message.Chat.ID,
			Language:   langCode,
		})
		if err != nil {
			slog.Error("error creating customer", "error", err)
			return
		}
	} else {
		updates := map[string]interface{}{
			"language": langCode,
		}

		err = h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates)
		if err != nil {
			slog.Error("Error updating customer", "error", err)
			return
		}
	}

	// === Referral recording ===
	// Works for BOTH new and existing users, as long as:
	// 1. The message contains a ref_ code
	// 2. The user has never made a paid purchase
	// 3. The user doesn't already have a referral record (UNIQUE constraint)
	slog.Info("[REFERRAL-DEBUG] /start handler entry",
		"message_text", update.Message.Text,
		"chat_id", update.Message.Chat.ID,
		"customer_id", existingCustomer.ID,
		"telegram_id", existingCustomer.TelegramID,
		"contains_ref", strings.Contains(update.Message.Text, "ref_"),
	)

	if strings.Contains(update.Message.Text, "ref_") {
		parts := strings.Split(update.Message.Text, " ")
		slog.Info("[REFERRAL-DEBUG] ref_ detected in message", "parts_count", len(parts), "parts", parts)

		if len(parts) > 1 {
			arg := parts[1]
			slog.Info("[REFERRAL-DEBUG] parsing argument", "arg", arg, "has_ref_prefix", strings.HasPrefix(arg, "ref_"))

			if strings.HasPrefix(arg, "ref_") {
				code := strings.TrimPrefix(arg, "ref_")
				referrerId, err := strconv.ParseInt(code, 10, 64)
				if err != nil {
					slog.Error("[REFERRAL-DEBUG] FAILED: error parsing referrer id", "code", code, "error", err)
				} else if referrerId == existingCustomer.TelegramID {
					slog.Warn("[REFERRAL-DEBUG] BLOCKED: self-referral attempt", "telegram_id", referrerId)
				} else {
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
					} else {
						// Check if user has ever made a paid purchase
						paidPurchase, purchaseErr := h.purchaseRepository.FindSuccessfulPaidPurchaseByCustomer(ctx, existingCustomer.ID)
						slog.Info("[REFERRAL-DEBUG] FindSuccessfulPaidPurchaseByCustomer result",
							"customer_id", existingCustomer.ID,
							"has_paid_purchase", paidPurchase != nil,
							"error", purchaseErr,
						)

						if paidPurchase != nil {
							slog.Info("[REFERRAL-DEBUG] SKIPPED: user already has paid purchase, referral not eligible",
								"purchase_id", paidPurchase.ID,
								"purchase_status", paidPurchase.Status,
							)
						} else {
							// Verify referrer exists
							referrer, referrerErr := h.customerRepository.FindByTelegramId(ctx, referrerId)
							slog.Info("[REFERRAL-DEBUG] Referrer lookup result",
								"referrer_telegram_id", referrerId,
								"referrer_found", referrer != nil,
								"error", referrerErr,
							)

							if referrerErr == nil && referrer != nil {
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
							} else {
								slog.Warn("[REFERRAL-DEBUG] SKIPPED: referrer not found in database", "referrer_telegram_id", referrerId)
							}
						}
					}
				}
			}
		} else {
			slog.Warn("[REFERRAL-DEBUG] SKIPPED: referral link malformed, no space-separated argument", "text", update.Message.Text)
		}
	} else {
		slog.Info("[REFERRAL-DEBUG] No ref_ in message, skipping referral logic")
	}

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, "my")

	m, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "🧹",
		ReplyMarkup: models.ReplyKeyboardRemove{
			RemoveKeyboard: true,
		},
	})

	if err != nil {
		slog.Error("Error sending removing reply keyboard", "error", err)
		return
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.Message.Chat.ID,
		MessageID: m.ID,
	})

	if err != nil {
		slog.Error("Error deleting message", "error", err)
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText("my", "greeting"),
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

	inlineKeyboard := h.buildStartKeyboard(existingCustomer, "my")

	_, err = b.EditMessageText(ctxWithTime, &bot.EditMessageTextParams{
		ChatID:    callback.Message.Message.Chat.ID,
		MessageID: callback.Message.Message.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		},
		Text: h.translation.GetText("my", "greeting"),
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
	return [][]models.InlineKeyboardButton{{
		{
			Text: h.translation.GetText(langCode, "buy_button"),
			WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			},
		},
	}}
}
