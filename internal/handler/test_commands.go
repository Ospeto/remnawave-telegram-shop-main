package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *Handler) TestCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	args := strings.Split(update.Message.Text, " ")
	if len(args) < 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /test enable OR /test disable",
		})
		return
	}

	arg := args[1]
	if arg == "enable" {
		h.paymentService.SetTestMode(true)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("✅ Test Mode ENABLED.\n\nMagic Transaction ID: %s\nScreenshots with this ID will be auto-approved.", h.paymentService.GetTestTransactionID()),
		})
	} else if arg == "disable" {
		h.paymentService.SetTestMode(false)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Test Mode DISABLED.\nSystem returned to normal verification.",
		})
	} else {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /test enable OR /test disable",
		})
	}
}

func (h *Handler) NotiCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	args := strings.Split(update.Message.Text, " ")
	if len(args) < 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /noti <telegram_id> or /notify <telegram_id>",
		})
		return
	}

	telegramIDStr := args[1]
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Invalid Telegram ID format.",
		})
		return
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, telegramID)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("Customer with ID %d not found in database.", telegramID),
		})
		return
	}

	if h.subscriptionService == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Subscription service not initialized in handler.",
		})
		return
	}

	keys, err := h.subKeyRepo.FindByCustomerID(ctx, customer.ID)
	if err != nil || len(keys) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Customer has no active subscription keys.",
		})
		return
	}

	err = h.subscriptionService.SendNotification(ctx, keys[0], *customer)
	if err != nil {
		slog.Error("Failed to send test notification", "error", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("Failed to send notification: %v", err),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("✅ Notification sent to %d successfully.", telegramID),
	})
}
