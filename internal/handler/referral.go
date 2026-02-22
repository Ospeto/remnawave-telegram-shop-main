package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) ReferralCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	customer, err := h.customerRepository.FindByTelegramId(ctx, update.CallbackQuery.From.ID)
	if err != nil || customer == nil {
		slog.Error("referral handler: customer not found", "error", err)
		return
	}
	langCode := update.CallbackQuery.From.LanguageCode

	// Build share link. Bot username comes from the message sender (the bot itself).
	refCode := customer.TelegramID
	refLink := fmt.Sprintf("https://telegram.me/share/url?url=https://t.me/%s?start=ref_%d",
		update.CallbackQuery.Message.Message.From.Username, refCode)

	// Fetch all referrals made by this customer
	referrals, err := h.referralRepository.FindByReferrer(ctx, customer.TelegramID)
	if err != nil {
		slog.Error("error fetching referrals", "error", err)
		return
	}

	totalEarned := 0
	var historyLines []string

	if len(referrals) == 0 {
		historyLines = append(historyLines, h.translation.GetText(langCode, "referral_history_empty"))
	} else {
		for _, ref := range referrals {
			if ref.BonusGranted {
				totalEarned += 1000
				historyLines = append(historyLines,
					fmt.Sprintf(h.translation.GetText(langCode, "referral_history_item_done"),
						maskID(ref.RefereeID),
						ref.UsedAt.In(time.UTC).Format("02 Jan 2006")),
				)
			} else {
				historyLines = append(historyLines,
					fmt.Sprintf(h.translation.GetText(langCode, "referral_history_item_pending"),
						maskID(ref.RefereeID),
						ref.UsedAt.In(time.UTC).Format("02 Jan 2006")),
				)
			}
		}
	}

	header := fmt.Sprintf(h.translation.GetText(langCode, "referral_text"), len(referrals), totalEarned)
	var text string
	if len(historyLines) > 0 {
		text = header + "\n\n" + strings.Join(historyLines, "\n")
	} else {
		text = header
	}

	callbackMessage := update.CallbackQuery.Message.Message
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: h.translation.GetText(langCode, "share_referral_button"), URL: refLink},
			},
			{
				{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
			},
		}},
	})
	if err != nil {
		slog.Error("Error sending referral message", "error", err)
	}
}

// maskID returns the last 4 digits of a telegram ID for privacy-safe display.
func maskID(id int64) string {
	s := fmt.Sprintf("%d", id)
	if len(s) > 4 {
		return "***" + s[len(s)-4:]
	}
	return s
}
