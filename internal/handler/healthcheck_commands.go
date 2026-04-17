package handler

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/service/healthcheck"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *Handler) HealthcheckCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	args := strings.Fields(update.Message.Text)
	if len(args) < 2 || args[1] != "run" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /healthbot run",
		})
		return
	}

	if h.healthcheckService == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "E2E healthcheck is not configured in this runtime.",
		})
		return
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Running synthetic E2E healthcheck. This checks analyzer readiness and a disposable fulfillment canary.",
	})

	report := h.healthcheckService.Run(ctx)
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      formatHealthcheckReport(report),
		ParseMode: models.ParseModeHTML,
	})
}

func formatHealthcheckReport(report *healthcheck.Report) string {
	if report == nil {
		return "E2E Healthcheck: FAIL\nNo report returned."
	}

	status := "FAIL"
	if report.Success {
		status = "PASS"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🩺 <b>E2E Healthcheck: %s</b>\n", status))
	sb.WriteString(fmt.Sprintf("Duration: <code>%s</code>\n\n", report.Duration.Round(time.Millisecond)))
	for _, step := range report.Steps {
		sb.WriteString(fmt.Sprintf("%s <b>%s</b>\n%s\n\n", stepStatusEmoji(step.Status), html.EscapeString(step.Name), html.EscapeString(step.Detail)))
	}
	return strings.TrimSpace(sb.String())
}

func stepStatusEmoji(status healthcheck.StepStatus) string {
	switch status {
	case healthcheck.StepPass:
		return "✅"
	case healthcheck.StepWarn:
		return "⚠️"
	case healthcheck.StepSkip:
		return "⏭️"
	default:
		return "❌"
	}
}
