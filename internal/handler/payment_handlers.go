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
	"remnawave-tg-shop-bot/internal/payment"
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

	keyboard := h.buildPricingKeyboard(langCode)

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

func (h Handler) buildPricingKeyboard(langCode string) [][]models.InlineKeyboardButton {
	plans := config.Plans()
	var keyboard [][]models.InlineKeyboardButton

	for i, plan := range plans {
		label := fmt.Sprintf("%s %d Days - %s %s", plan.Label, plan.Days, formatPrice(plan.Price), config.Currency())
		keyboard = append(keyboard, []models.InlineKeyboardButton{{
			Text:         label,
			CallbackData: fmt.Sprintf("%s?plan=%d", CallbackSell, i),
		}})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	return keyboard
}

func (h Handler) SellCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode
	planIdx := callbackQuery["plan"]

	keyboard := h.buildPaymentMethodKeyboard(langCode, planIdx)

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

func (h Handler) buildPaymentMethodKeyboard(langCode string, planIdx string) [][]models.InlineKeyboardButton {
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

	return keyboard
}

func (h Handler) PaymentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	planIdx, err := strconv.Atoi(callbackQuery["plan"])
	if err != nil {
		slog.Error("Error getting plan index from query", "error", err)
		return
	}

	if planIdx < 0 {
		slog.Error("Invalid plan index (negative)", "index", planIdx)
		return
	}

	plan := config.PlanByIndex(planIdx)
	if plan == nil {
		slog.Error("Invalid plan index", "index", planIdx)
		return
	}

	invoiceType := database.InvoiceType(callbackQuery["invoiceType"])

	dbCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	customer, err := h.customerRepository.FindByTelegramId(dbCtx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("customer not exist", "chatID", callback.Chat.ID, "error", err)
		return
	}

	ctxWithUsername := context.WithValue(dbCtx, "username", update.CallbackQuery.From.Username)
	langCode := update.CallbackQuery.From.LanguageCode

	if invoiceType == database.InvoiceTypeMobileBanking {
		h.handleMobileBankingPayment(ctxWithUsername, b, callback, plan, customer, planIdx, langCode)
		return
	}

	h.handleCryptoPayment(ctxWithUsername, b, callback, plan, customer, planIdx, langCode)
}

func (h Handler) handleMobileBankingPayment(ctx context.Context, b *bot.Bot, callback *models.Message, plan *config.Plan, customer *database.Customer, planIdx int, langCode string) {
	_, purchaseId, err := h.paymentService.CreatePurchase(ctx, float64(plan.Price), plan.Days, plan.TrafficLimitGB, customer, database.InvoiceTypeMobileBanking, "")
	if err != nil {
		slog.Error("Error creating mobile banking purchase", "error", err)
		return
	}

	// Store pending state: telegramID → purchaseID
	h.mobilePayCache.Set(callback.Chat.ID, int(purchaseId))

	instructions := fmt.Sprintf(
		h.translation.GetText(langCode, "mobile_pay_instructions"),
		plan.Price,
		payment.GetFirstPaymentPhone(),
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
}

func (h Handler) handleCryptoPayment(ctx context.Context, b *bot.Bot, callback *models.Message, plan *config.Plan, customer *database.Customer, planIdx int, langCode string) {
	paymentURL, purchaseId, err := h.paymentService.CreatePurchase(ctx, float64(plan.Price), plan.Days, plan.TrafficLimitGB, customer, database.InvoiceTypeCrypto, "")
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

// downloadTelegramFile downloads a file from Telegram and returns its bytes and mime type.
func (h Handler) downloadTelegramFile(ctx context.Context, b *bot.Bot, fileID string) ([]byte, string, error) {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", fmt.Errorf("getting file info: %w", err)
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", config.TelegramToken(), file.FilePath)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating download request: %w", err)
	}
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("downloading photo: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status downloading photo: %d", httpResp.StatusCode)
	}

	imageBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading photo bytes: %w", err)
	}

	mimeType := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(file.FilePath), ".png") {
		mimeType = "image/png"
	}

	return imageBytes, mimeType, nil
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

	// Clear the pending state so user can't double-submit
	h.mobilePayCache.Delete(chatID)

	defer func() {
		// Delete the "verifying" message later
		if verifyMsg != nil {
			_, _ = b.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
				ChatID:    chatID,
				MessageID: verifyMsg.ID,
			})
		}
	}()

	// Get the highest resolution photo
	photo := update.Message.Photo[len(update.Message.Photo)-1]

	imageBytes, mimeType, err := h.downloadTelegramFile(ctx, b, photo.FileID)
	if err != nil {
		slog.Error("Error downloading telegram file", "error", err)
		h.sendMobilePayResult(ctx, b, chatID, langCode, "mobile_pay_failed_generic", 0)
		return
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

	if result.Success {
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
