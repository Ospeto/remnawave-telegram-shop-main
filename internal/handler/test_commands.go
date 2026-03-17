package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"strconv"
	"strings"
	"time"

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

	err = h.subscriptionService.SendNotification(ctx, *customer)
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

func (h *Handler) APICheckCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	report := h.paymentService.ReceiptAnalyzerHealthReport(checkCtx)
	if len(report) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Receipt AI is not configured.",
		})
		return
	}

	var body strings.Builder
	overallOK := false
	body.WriteString("Receipt AI health report\n\n")
	for _, item := range report {
		if item.Err == nil {
			overallOK = true
			body.WriteString(fmt.Sprintf("✅ %s: %s\n", item.Role, item.Name))
			continue
		}
		body.WriteString(fmt.Sprintf("❌ %s: %s\nError: %v\n\n", item.Role, item.Name, item.Err))
	}

	if !overallOK {
		body.WriteString("No provider is currently healthy.")
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   strings.TrimSpace(body.String()),
	})
}

func (h *Handler) HealthCheckCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/healthcheck", config.GetHealthCheckPort())
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, url, nil)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Failed to build healthcheck request: %v", err),
		})
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Healthcheck request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Failed to read healthcheck response: %v", err),
		})
		return
	}

	text := fmt.Sprintf("Healthcheck status: %s\n%s", resp.Status, strings.TrimSpace(string(body)))
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   text,
	})
}
