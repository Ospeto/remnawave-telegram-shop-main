package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	appSync "remnawave-tg-shop-bot/internal/sync"
)

// syncHandlerTimeout bounds one background sync pass. The pass reads the whole
// panel fleet and rewrites the customer table, so it needs far more room than a
// normal interactive handler.
const syncHandlerTimeout = 2 * time.Minute

// SyncUsersCommandHandler reconciles local customers with the Remnawave panel.
//
// U10: this used to run the whole pass inline and answer a bare "Users synced",
// so a read failure was indistinguishable from a clean run, and the admin's
// Telegram worker was blocked for the entire pass. It now acknowledges
// immediately and reports the real counts (or the errors) when the pass ends.
func (h Handler) SyncUsersCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}
	h.clearAdminFlow(update.Message.From.ID)

	chatID := update.Message.Chat.ID
	if h.syncService == nil {
		h.sendErrorReply(ctx, b, chatID, "en", translationKeyGenericError, fmt.Errorf("sync service is not configured"))
		return
	}

	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "🔄 Syncing panel users… I'll report the counts here when it finishes.",
	}); err != nil {
		slog.Error("Error sending sync acknowledgement", "error", err)
	}

	// Detach from the update context: the Telegram update finishes as soon as
	// this handler returns, which would cancel the pass.
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), syncHandlerTimeout)

	go func() {
		defer cancel()

		result := h.syncService.SyncWithContext(jobCtx)

		if _, err := b.SendMessage(context.WithoutCancel(ctx), &bot.SendMessageParams{
			ChatID:    chatID,
			ParseMode: models.ParseModeHTML,
			Text:      formatSyncResult(result),
		}); err != nil {
			slog.Error("Error sending sync report", "error", err, "failed", result.Failed)
		}
	}()
}

// formatSyncResult renders the counts an admin needs to judge a pass: what the
// panel returned, what changed locally, and what failed.
func formatSyncResult(result appSync.Result) string {
	var report strings.Builder

	report.WriteString("✅ <b>Sync finished</b>\n\n")
	fmt.Fprintf(&report, "Panel users: <b>%d</b>\n", result.RemoteUsers)
	fmt.Fprintf(&report, "Added: <b>%d</b>\n", result.Added)
	fmt.Fprintf(&report, "Updated: <b>%d</b>\n", result.Updated)
	fmt.Fprintf(&report, "Deleted keys: <b>%d</b>\n", result.Deleted)
	fmt.Fprintf(&report, "Failed steps: <b>%d</b>\n", result.Failed)
	fmt.Fprintf(&report, "Duration: <b>%s</b>", result.Duration.Round(time.Millisecond))

	if len(result.Errors) > 0 {
		report.WriteString("\n\n<b>Errors</b>\n")
		for _, entry := range result.Errors {
			fmt.Fprintf(&report, "• %s\n", entry)
		}
	}

	return strings.TrimRight(report.String(), "\n")
}
