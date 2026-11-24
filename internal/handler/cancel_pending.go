package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
)

// CancelPendingPurchaseCallbackHandler cancels a purchase that is still waiting
// for a receipt screenshot, without leaving the chat.
//
// U2: the pending-payment message told the customer to "open the app to cancel
// it", and the keyboard held only a WebApp link, so someone who paid to the
// wrong account was stuck until the purchase was reaped. The first tap asks for
// confirmation; the confirmed tap releases the purchase through the same
// CancelAwaitingVerificationPurchase path the Mini App uses.
func (h Handler) CancelPendingPurchaseCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return
	}

	message := update.CallbackQuery.Message.Message
	chatID := message.Chat.ID
	params := parseCallbackData(update.CallbackQuery.Data)

	purchaseID, err := strconv.ParseInt(params["id"], 10, 64)
	if err != nil || purchaseID <= 0 {
		slog.Error("Invalid purchase id in cancel callback", "error", err, "data", update.CallbackQuery.Data)
		h.editErrorReply(ctx, b, chatID, message.ID, update.CallbackQuery.From.LanguageCode,
			"cancel_pending_not_found", err)
		return
	}

	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	customer, err := h.customerRepository.FindByTelegramId(dbCtx, chatID)
	if err != nil || customer == nil {
		h.editErrorReply(ctx, b, chatID, message.ID, update.CallbackQuery.From.LanguageCode,
			translationKeyGenericError, err)
		return
	}

	langCode := resolveEffectiveLanguage(customer, update.CallbackQuery.From.LanguageCode)

	purchase, err := h.purchaseRepository.FindById(dbCtx, purchaseID)
	if err != nil || purchase == nil || purchase.CustomerID != customer.ID {
		h.editErrorReply(ctx, b, chatID, message.ID, langCode, "cancel_pending_not_found", err)
		return
	}

	if params["confirm"] != "1" {
		h.editCancelConfirmation(ctx, b, chatID, message.ID, langCode, purchase)
		return
	}

	cancelled, err := h.paymentService.CancelAwaitingVerificationPurchase(dbCtx, purchaseID, customer.ID)
	if err != nil {
		h.editErrorReply(ctx, b, chatID, message.ID, langCode, translationKeyGenericError, err)
		return
	}

	// Release the chat's cached pending purchase so a later photo does not try to
	// verify against a cancelled row.
	if h.mobilePayCache != nil {
		h.mobilePayCache.Delete(chatID)
	}

	if !cancelled {
		// The row was already paid, already cancelled, or reaped back to pending:
		// say so plainly instead of pretending the tap worked.
		h.editErrorReply(ctx, b, chatID, message.ID, langCode, "cancel_pending_gone", nil)
		return
	}

	slog.Info("Customer cancelled pending purchase from chat",
		"purchase_id", purchaseID,
		"telegram_id", chatID,
	)

	if _, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: message.ID,
		ParseMode: models.ParseModeHTML,
		Text:      h.translation.GetText(langCode, "cancel_pending_done"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackBuy}},
				{{Text: h.translation.GetText(langCode, "cancel_pending_button"), CallbackData: CallbackStart}},
			},
		},
	}); err != nil {
		slog.Error("Error sending cancel confirmation", "error", err)
	}
}

// editCancelConfirmation replaces the instruction message with a yes/no prompt.
func (h Handler) editCancelConfirmation(ctx context.Context, b *bot.Bot, chatID int64, messageID int, langCode string, purchase *database.Purchase) {
	text := fmt.Sprintf(
		h.translation.GetText(langCode, "cancel_pending_confirm"),
		int(purchase.Amount),
	)

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		ParseMode: models.ParseModeHTML,
		Text:      text,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{
					Text:         h.translation.GetText(langCode, "cancel_pending_confirm_yes"),
					CallbackData: fmt.Sprintf("%s?id=%d&confirm=1", CallbackCancelPendingPurchase, purchase.ID),
				}},
				{{
					Text:         h.translation.GetText(langCode, "cancel_pending_confirm_no"),
					CallbackData: CallbackStart,
				}},
			},
		},
	})
	if err != nil {
		slog.Error("Error sending cancel confirmation prompt", "error", err)
	}
}
