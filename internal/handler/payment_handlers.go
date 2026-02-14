package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func formatPrice(price int) string {
	s := strconv.Itoa(price)
	n := len(s)
	if n <= 3 {
		return s
	}
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

func (h Handler) BuyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode

	plans := config.Plans()
	var priceButtons []models.InlineKeyboardButton

	for i, plan := range plans {
		label := fmt.Sprintf("%s %d Days - %s %s", plan.Label, plan.Days, formatPrice(plan.Price), config.Currency())
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         label,
			CallbackData: fmt.Sprintf("%s?plan=%d", CallbackSell, i),
		})
	}

	keyboard := [][]models.InlineKeyboardButton{}
	// One button per row for better readability
	for _, btn := range priceButtons {
		keyboard = append(keyboard, []models.InlineKeyboardButton{btn})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "pricing_info"),
	})

	if err != nil {
		slog.Error("Error sending buy message", "error", err)
	}
}

func (h Handler) SellCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode
	planIdx := callbackQuery["plan"]

	var keyboard [][]models.InlineKeyboardButton

	if config.IsCryptoPayEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "crypto_button"), CallbackData: fmt.Sprintf("%s?plan=%s&invoiceType=%s", CallbackPayment, planIdx, database.InvoiceTypeCrypto)},
		})
	}

	if config.IsMobileBankingEnabled() {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "mobile_banking_button"), CallbackData: fmt.Sprintf("%s?plan=%s&invoiceType=%s", CallbackPayment, planIdx, database.InvoiceTypeMobileBanking)},
		})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackBuy},
	})

	_, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})

	if err != nil {
		slog.Error("Error sending sell message", "error", err)
	}
}

func (h Handler) PaymentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	planIdx, err := strconv.Atoi(callbackQuery["plan"])
	if err != nil {
		slog.Error("Error getting plan index from query", "error", err)
		return
	}

	plan := config.PlanByIndex(planIdx)
	if plan == nil {
		slog.Error("Invalid plan index", "index", planIdx)
		return
	}

	invoiceType := database.InvoiceType(callbackQuery["invoiceType"])

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "chatID", callback.Chat.ID, "error", err)
		return
	}

	ctxWithUsername := context.WithValue(ctx, "username", update.CallbackQuery.From.Username)
	langCode := update.CallbackQuery.From.LanguageCode

	if invoiceType == database.InvoiceTypeMobileBanking {
		// Mobile banking: create purchase, show instructions, wait for screenshot
		_, purchaseId, err := h.paymentService.CreatePurchase(ctxWithUsername, float64(plan.Price), plan.Days, plan.TrafficLimitGB, customer, database.InvoiceTypeMobileBanking)
		if err != nil {
			slog.Error("Error creating mobile banking purchase", "error", err)
			return
		}

		// Store pending state: telegramID → purchaseID
		h.mobilePayCache.Set(callback.Chat.ID, int(purchaseId))

		instructions := fmt.Sprintf(
			h.translation.GetText(langCode, "mobile_pay_instructions"),
			plan.Price,
			config.MobileBankingPhone(),
		)

		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callback.Chat.ID,
			MessageID: callback.ID,
			ParseMode: models.ParseModeHTML,
			Text:      instructions,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						{Text: h.translation.GetText(langCode, "back_button"), CallbackData: fmt.Sprintf("%s?plan=%d", CallbackSell, planIdx)},
					},
				},
			},
		})
		if err != nil {
			slog.Error("Error sending mobile pay instructions", "error", err)
		}
		return
	}

	// CryptoPay flow
	paymentURL, purchaseId, err := h.paymentService.CreatePurchase(ctxWithUsername, float64(plan.Price), plan.Days, plan.TrafficLimitGB, customer, database.InvoiceTypeCrypto)
	if err != nil {
		slog.Error("Error creating payment", "error", err)
		return
	}

	message, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "pay_button"), URL: paymentURL},
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: fmt.Sprintf("%s?plan=%d", CallbackSell, planIdx)},
				},
			},
		},
	})
	if err != nil {
		slog.Error("Error updating sell message", "error", err)
		return
	}
	h.cache.Set(purchaseId, message.ID)
}

func parseCallbackData(data string) map[string]string {
	result := make(map[string]string)

	parts := strings.Split(data, "?")
	if len(parts) < 2 {
		return result
	}

	params := strings.Split(parts[1], "&")
	for _, param := range params {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}

	return result
}

// MobilePayScreenshotHandler handles photo messages from users with pending mobile banking payments.
func (h Handler) MobilePayScreenshotHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || len(update.Message.Photo) == 0 {
		return
	}

	chatID := update.Message.Chat.ID
	langCode := update.Message.From.LanguageCode

	// Check if this user has a pending mobile banking purchase
	purchaseID, hasPending := h.mobilePayCache.Get(chatID)
	if !hasPending {
		// No pending payment — ignore the photo
		return
	}

	// Send "verifying" message
	verifyMsg, _ := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   h.translation.GetText(langCode, "mobile_pay_verifying"),
	})

	// Get the highest resolution photo
	photo := update.Message.Photo[len(update.Message.Photo)-1]

	// Download the photo from Telegram
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: photo.FileID})
	if err != nil {
		slog.Error("Error getting file from Telegram", "error", err)
		h.sendMobilePayResult(ctx, b, chatID, langCode, "mobile_pay_failed_generic", 0)
		return
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", config.TelegramToken(), file.FilePath)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		slog.Error("Error creating download request", "error", err)
		h.sendMobilePayResult(ctx, b, chatID, langCode, "mobile_pay_failed_generic", 0)
		return
	}
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		slog.Error("Error downloading photo", "error", err)
		h.sendMobilePayResult(ctx, b, chatID, langCode, "mobile_pay_failed_generic", 0)
		return
	}
	defer httpResp.Body.Close()

	imageBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		slog.Error("Error reading photo bytes", "error", err)
		h.sendMobilePayResult(ctx, b, chatID, langCode, "mobile_pay_failed_generic", 0)
		return
	}

	// Determine MIME type
	mimeType := "image/jpeg"
	if strings.HasSuffix(file.FilePath, ".png") {
		mimeType = "image/png"
	}

	// Verify the payment
	verifyCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := h.paymentService.VerifyMobilePayment(verifyCtx, int64(purchaseID), imageBytes, mimeType)
	if err != nil {
		slog.Error("Error verifying mobile payment", "error", err)
		h.sendMobilePayResult(ctx, b, chatID, langCode, "mobile_pay_failed_generic", 0)
		return
	}

	// Delete the "verifying" message
	if verifyMsg != nil {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: verifyMsg.ID,
		})
	}

	if result.Success {
		// Clear the pending state so user can't re-submit
		h.mobilePayCache.Delete(chatID)
		h.sendMobilePayResult(ctx, b, chatID, langCode, result.ReasonKey, 0)
		return
	}

	// For amount mismatch, pass the expected purchase amount
	var expectedAmount int
	if result.ReasonKey == "mobile_pay_failed_amount" {
		purchase, pErr := h.purchaseRepository.FindById(ctx, int64(purchaseID))
		if pErr == nil && purchase != nil {
			expectedAmount = int(purchase.Amount)
		}
	}
	h.sendMobilePayResult(ctx, b, chatID, langCode, result.ReasonKey, expectedAmount)
}

func (h Handler) sendMobilePayResult(ctx context.Context, b *bot.Bot, chatID int64, langCode string, translationKey string, amount int) {
	text := h.translation.GetText(langCode, translationKey)
	if translationKey == "mobile_pay_failed_amount" && amount > 0 {
		text = fmt.Sprintf(h.translation.GetText(langCode, translationKey), amount)
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		Text:      text,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
				},
			},
		},
	})
	if err != nil {
		slog.Error("Error sending mobile pay result", "error", err)
	}
}
