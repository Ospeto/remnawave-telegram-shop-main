package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
)

func referralDisplayID(ctx context.Context, repo interface {
	FindById(context.Context, int64) (*database.Customer, error)
	FindByTelegramId(context.Context, int64) (*database.Customer, error)
}, customerID int64) int64 {
	customer, err := database.ResolveReferralCustomer(ctx, repo, customerID)
	if err != nil || customer == nil {
		return customerID
	}
	return customer.TelegramID
}

func referralHistoryTime(ref database.Referral) time.Time {
	return database.ReferralActivityAt(ref).In(time.UTC)
}

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
	referrals, err := h.referralRepository.FindByReferrerAny(ctx, database.ReferralIdentityValues(customer.ID, customer.TelegramID)...)
	if err != nil {
		slog.Error("error fetching referrals", "error", err)
		return
	}
	referrals, err = database.NormalizeReferralsByReferee(ctx, referrals, h.customerRepository)
	if err != nil {
		slog.Error("error normalizing referrals", "error", err)
		return
	}

	totalEarned := 0.0
	if h.paymentService != nil {
		totalEarned, err = h.paymentService.ReferralEarnedTotal(ctx, customer.ID)
		if err != nil {
			slog.Error("error computing referral earnings", "error", err)
			return
		}
	}
	var historyLines []string

	if len(referrals) == 0 {
		historyLines = append(historyLines, h.translation.GetText(langCode, "referral_history_empty"))
	} else {
		for _, ref := range referrals {
			refereeDisplayID := referralDisplayID(ctx, h.customerRepository, ref.RefereeID)
			referralTime := referralHistoryTime(ref).Format("02 Jan 2006")
			if ref.BonusGranted {
				historyLines = append(historyLines,
					fmt.Sprintf(h.translation.GetText(langCode, "referral_history_item_done"),
						maskID(refereeDisplayID),
						referralTime),
				)
			} else {
				historyLines = append(historyLines,
					fmt.Sprintf(h.translation.GetText(langCode, "referral_history_item_pending"),
						maskID(refereeDisplayID),
						referralTime),
				)
			}
		}
	}

	header := fmt.Sprintf(h.translation.GetText(langCode, "referral_text"), len(referrals), int(totalEarned))
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
