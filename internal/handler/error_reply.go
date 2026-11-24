package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Translation keys the shared failure helper falls back to. Keeping them
// central means a terminal error branch cannot invent English copy that never
// reaches translations/*.json.
const (
	translationKeyGenericError = "generic_error_retry"
	translationKeyPaymentStart = "payment_could_not_start"
)

// sendErrorReply tells the customer a handler failed instead of leaving them on
// a dead keyboard.
//
// U1: the terminal branches used to log through slog.Error and return, so a
// failed purchase or a missing customer row looked like a frozen bot. Every one
// of those branches now answers with localized copy, and the underlying error
// still reaches the logs with the same context.
//
// args are Sprintf arguments for translationKey, so callers can pass an amount
// or a plan name.
func (h Handler) sendErrorReply(ctx context.Context, b *bot.Bot, chatID int64, langCode string, translationKey string, cause error, args ...any) {
	if cause != nil {
		slog.Error("Handler failed; sending localized error reply to customer",
			"chat_id", chatID,
			"translation_key", translationKey,
			"error", cause,
		)
	}

	if b == nil || chatID == 0 {
		return
	}

	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      h.errorReplyText(langCode, translationKey, args...),
		ParseMode: models.ParseModeHTML,
	}); err != nil {
		slog.Error("Failed to send localized error reply", "chat_id", chatID, "error", err)
	}
}

// editErrorReply is sendErrorReply for callback flows: the instruction message
// the customer is looking at is replaced rather than supplemented, so a stale
// keyboard cannot be tapped twice.
func (h Handler) editErrorReply(ctx context.Context, b *bot.Bot, chatID int64, messageID int, langCode string, translationKey string, cause error, args ...any) {
	if cause != nil {
		slog.Error("Handler failed; editing message into a localized error reply",
			"chat_id", chatID,
			"translation_key", translationKey,
			"error", cause,
		)
	}

	if b == nil || chatID == 0 || messageID == 0 {
		return
	}

	if _, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      h.errorReplyText(langCode, translationKey, args...),
		ParseMode: models.ParseModeHTML,
	}); err != nil {
		slog.Error("Failed to edit message into a localized error reply", "chat_id", chatID, "error", err)
	}
}

// errorReplyText resolves localized copy, falling back to the generic retry
// message when a key is missing from the translations (GetText echoes the key
// itself in that case, which must never reach a customer).
func (h Handler) errorReplyText(langCode string, translationKey string, args ...any) string {
	key := translationKey
	if key == "" {
		key = translationKeyGenericError
	}

	text := key
	if h.translation != nil {
		text = h.translation.GetText(langCode, key)
		if text == key {
			text = h.translation.GetText(langCode, translationKeyGenericError)
		}
	}

	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}
