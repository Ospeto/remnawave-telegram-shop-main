package payment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/gemini"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
	"strings"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type PaymentService struct {
	purchaseRepository  *database.PurchaseRepository
	remnawaveClient     *remnawave.Client
	customerRepository  *database.CustomerRepository
	telegramBot         *bot.Bot
	translation         *translation.Manager
	cryptoPayClient     *cryptopay.Client
	referralRepository  *database.ReferralRepository
	cache               *cache.Cache
	geminiClient        *gemini.Client
	mobilePaymentRepo   *database.MobilePaymentRepository
	subKeyRepo          *database.SubscriptionKeyRepository
	promoCodeRepository *database.PromoCodeRepository
}

func NewPaymentService(
	translation *translation.Manager,
	purchaseRepository *database.PurchaseRepository,
	remnawaveClient *remnawave.Client,
	customerRepository *database.CustomerRepository,
	telegramBot *bot.Bot,
	cryptoPayClient *cryptopay.Client,
	referralRepository *database.ReferralRepository,
	cache *cache.Cache,
	geminiClient *gemini.Client,
	mobilePaymentRepo *database.MobilePaymentRepository,
	subKeyRepo *database.SubscriptionKeyRepository,
	promoCodeRepository *database.PromoCodeRepository,
) *PaymentService {
	return &PaymentService{
		purchaseRepository:  purchaseRepository,
		remnawaveClient:     remnawaveClient,
		customerRepository:  customerRepository,
		telegramBot:         telegramBot,
		translation:         translation,
		cryptoPayClient:     cryptoPayClient,
		referralRepository:  referralRepository,
		cache:               cache,
		geminiClient:        geminiClient,
		mobilePaymentRepo:   mobilePaymentRepo,
		subKeyRepo:          subKeyRepo,
		promoCodeRepository: promoCodeRepository,
	}
}

// GetPurchaseRepository exposes the purchase repository for external use (e.g. API handlers).
func (s PaymentService) GetPurchaseRepository() *database.PurchaseRepository {
	return s.purchaseRepository
}

func (s PaymentService) SyncKeys(ctx context.Context, customerID int64, telegramID int64) ([]KeyStats, error) {
	if s.subKeyRepo == nil {
		return nil, nil
	}

	// 1. Fetch fresh keys from Remnawave
	users, err := s.remnawaveClient.GetUsersByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Failed to fetch users from Remnawave", "error", err)
		// Fallback: return nothing or let handler use local DB
		return nil, err
	}

	// 2. Fetch local keys
	localKeys, err := s.subKeyRepo.FindByCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}

	// Create a map of remote users for easy lookup
	remoteMap := make(map[string]remapi.User)
	for _, u := range users {
		remoteMap[u.UUID.String()] = u
	}

	var result []KeyStats

	for _, localKey := range localKeys {
		remoteUser, exists := remoteMap[localKey.RemnawaveUUID.String()]

		// If key exists locally but not remotely -> DELETED
		if !exists {
			if localKey.Status != "deleted" {
				_ = s.subKeyRepo.UpdateStatus(ctx, localKey.ID, "deleted")
			}
			continue
		}

		// Key matches. Update local fields if changed.
		isExpired := remoteUser.ExpireAt.Before(time.Now())
		newStatus := "active"
		if isExpired {
			newStatus = "expired"
		}

		// Sync Expiry
		if localKey.ExpireAt == nil || !localKey.ExpireAt.Equal(remoteUser.ExpireAt) || localKey.Status != newStatus {
			_ = s.subKeyRepo.UpdateExpiry(ctx, localKey.ID, remoteUser.ExpireAt)
			// UpdateExpiry sets status to 'active', so we need to set it correctly if expired
			if newStatus != "active" {
				_ = s.subKeyRepo.UpdateStatus(ctx, localKey.ID, newStatus)
			}
		}

		// Build stats object
		limit := 0
		if remoteUser.TrafficLimitBytes.IsSet() {
			limit = remoteUser.TrafficLimitBytes.Value
		}

		stats := KeyStats{
			ID:                localKey.ID,
			TrafficUsedBytes:  remoteUser.UserTraffic.UsedTrafficBytes,
			TrafficLimitBytes: limit,
			ExpireAt:          remoteUser.ExpireAt,
			Status:            newStatus,
		}
		result = append(result, stats)
	}

	return result, nil
}

type KeyStats struct {
	ID                int64
	TrafficUsedBytes  float64
	TrafficLimitBytes int
	ExpireAt          time.Time
	Status            string
}

func (s PaymentService) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer %s not found", utils.MaskHalfInt64(purchase.CustomerID))
	}

	// Idempotency / Race Condition Check
	if purchase.Status == database.PurchaseStatusPaid {
		slog.Info("Purchase already paid, skipping processing", "purchase_id", purchaseId)
		return nil
	}

	if messageId, b := s.cache.Get(purchase.ID); b {
		_, err = s.telegramBot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    customer.TelegramID,
			MessageID: messageId,
		})
		if err != nil {
			slog.Error("Error deleting message", "error", err)
		}
	}

	const bytesInGB = 1073741824

	if purchase.ExtendKeyID != nil {
		// EXTEND existing key
		existingKey, err := s.subKeyRepo.FindByID(ctx, *purchase.ExtendKeyID)
		if err != nil || existingKey == nil {
			return fmt.Errorf("subscription key %d not found", *purchase.ExtendKeyID)
		}
		// Extend the specific Remnawave user by UUID (adds days and traffic)
		remnawaveUser, err := s.remnawaveClient.ExtendUser(ctx, existingKey.RemnawaveUUID, purchase.TrafficLimitGB*bytesInGB, purchase.Days)
		if err != nil {
			return err
		}
		// Update the subscription_key expiry
		if s.subKeyRepo != nil {
			if err := s.subKeyRepo.UpdateExpiry(ctx, existingKey.ID, remnawaveUser.ExpireAt); err != nil {
				slog.Error("Failed to update key expiry (non-fatal)", "key_id", existingKey.ID, "error", err)
			}
			if err := s.subKeyRepo.UpdateSubscriptionURL(ctx, existingKey.ID, remnawaveUser.SubscriptionUrl); err != nil {
				slog.Error("Failed to update subscription URL (non-fatal)", "key_id", existingKey.ID, "error", err)
			}
		}
		// Update customer expire_at to the latest
		customerFilesToUpdate := map[string]interface{}{
			"subscription_link": remnawaveUser.SubscriptionUrl,
			"expire_at":         remnawaveUser.ExpireAt,
		}
		if err := s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate); err != nil {
			slog.Error("Failed to update customer fields after extend (non-fatal)", "error", err)
		}
	} else {
		// CREATE new key — always creates a fresh Remnawave user
		keyCount, _ := s.subKeyRepo.CountByCustomerID(ctx, customer.ID)
		keyIndex := int(keyCount) + 1

		remnawaveUser, err := s.remnawaveClient.ForceCreateNewUser(ctx, customer.ID, customer.TelegramID, purchase.TrafficLimitGB*bytesInGB, purchase.Days, keyIndex, purchase.TransactionID)
		if err != nil {
			return err
		}
		// Insert into subscription_key table
		if s.subKeyRepo != nil {
			txnSuffix := ""
			if len(purchase.TransactionID) >= 4 {
				txnSuffix = purchase.TransactionID[len(purchase.TransactionID)-4:]
			} else if len(purchase.TransactionID) > 0 {
				txnSuffix = purchase.TransactionID
			} else {
				txnSuffix = fmt.Sprintf("%04d", purchase.ID%10000)
			}
			label := fmt.Sprintf("wavy_%s_%d", txnSuffix, customer.TelegramID)
			_, err := s.subKeyRepo.Create(ctx, &database.SubscriptionKey{
				CustomerID:      customer.ID,
				RemnawaveUUID:   remnawaveUser.UUID,
				Username:        remnawaveUser.Username,
				SubscriptionURL: remnawaveUser.SubscriptionUrl,
				ExpireAt:        &remnawaveUser.ExpireAt,
				Status:          "active",
				Label:           label,
			})
			if err != nil {
				slog.Error("CRITICAL: Failed to save subscription key to DB. Key EXISTS on Remnawave but NOT in local DB.",
					"purchase_id", purchaseId, "username", remnawaveUser.Username, "error", err)
				// Continue — key exists remotely. SyncKeys will recover it later.
			}
		}
		// Update customer
		customerFilesToUpdate := map[string]interface{}{
			"subscription_link": remnawaveUser.SubscriptionUrl,
			"expire_at":         remnawaveUser.ExpireAt,
		}
		if err := s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate); err != nil {
			slog.Error("Failed to update customer fields after create (non-fatal)", "error", err)
		}
	}

	// === MARK AS PAID FIRST ===
	// This MUST happen before Telegram notification and referral processing.
	// The key is already created/extended at this point. If anything below fails,
	// the user still has their key and the purchase is correctly marked as paid.
	err = s.purchaseRepository.MarkAsPaid(ctx, purchase.ID)
	if err != nil {
		slog.Error("CRITICAL: Failed to mark purchase as paid. Key was created but purchase status not updated.",
			"purchase_id", purchaseId, "error", err)
		return err
	}

	slog.Info("Purchase processed successfully", "purchase_id", utils.MaskHalfInt64(purchase.ID), "type", purchase.InvoiceType, "customer_id", utils.MaskHalfInt64(customer.ID))

	// === BELOW THIS LINE: NON-FATAL OPERATIONS ===
	// Telegram notification and referral bonus are best-effort.
	// Failures here should NOT prevent the user from getting their key.

	// Refresh customer
	customer, err = s.customerRepository.FindById(ctx, customer.ID)
	if err != nil {
		slog.Error("Error refreshing customer after purchase (non-fatal)", "error", err)
	}

	if _, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID,
		Text:   s.translation.GetText(customer.Language, "subscription_activated"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(customer),
		},
	}); err != nil {
		slog.Error("Failed to send activation message (non-fatal)", "error", err, "purchase_id", purchaseId)
	}

	// Referral bonus — completely non-fatal
	s.processReferralBonus(ctx, customer)

	return nil
}

// processReferralBonus grants a referral bonus to the referrer if applicable.
// This is intentionally non-fatal — errors are logged but never block the purchase flow.
func (s PaymentService) processReferralBonus(ctx context.Context, customer *database.Customer) {
	ctxReferee := context.Background()
	referee, err := s.referralRepository.FindByReferee(ctxReferee, customer.TelegramID)
	if err != nil {
		slog.Error("Referral lookup failed (non-fatal)", "error", err)
		return
	}
	if referee == nil || referee.BonusGranted {
		return
	}
	refereeCustomer, err := s.customerRepository.FindByTelegramId(ctxReferee, referee.ReferrerID)
	if err != nil {
		slog.Error("Referral customer lookup failed (non-fatal)", "error", err)
		return
	}
	refereeUser, err := s.remnawaveClient.CreateOrUpdateUser(ctxReferee, refereeCustomer.ID, refereeCustomer.TelegramID, 0, config.GetReferralDays(), false)
	if err != nil {
		slog.Error("Referral bonus user creation failed (non-fatal)", "error", err)
		return
	}
	refereeUserFilesToUpdate := map[string]interface{}{
		"subscription_link": refereeUser.GetSubscriptionUrl(),
		"expire_at":         refereeUser.GetExpireAt(),
	}
	if err := s.customerRepository.UpdateFields(ctxReferee, refereeCustomer.ID, refereeUserFilesToUpdate); err != nil {
		slog.Error("Referral customer update failed (non-fatal)", "error", err)
		return
	}
	if err := s.referralRepository.MarkBonusGranted(ctxReferee, referee.ID); err != nil {
		slog.Error("Referral mark granted failed (non-fatal)", "error", err)
		return
	}
	slog.Info("Granted referral bonus", "customer_id", utils.MaskHalfInt64(refereeCustomer.ID))
	if _, err := s.telegramBot.SendMessage(ctxReferee, &bot.SendMessageParams{
		ChatID:    refereeCustomer.TelegramID,
		ParseMode: models.ParseModeHTML,
		Text:      s.translation.GetText(refereeCustomer.Language, "referral_bonus_granted"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(refereeCustomer),
		},
	}); err != nil {
		slog.Error("Referral bonus notification failed (non-fatal)", "error", err)
	}
}

func (s PaymentService) createConnectKeyboard(customer *database.Customer) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton

	if config.GetMiniAppURL() != "" {
		inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
			{Text: s.translation.GetText(customer.Language, "connect_button"), WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			}},
		})
	} else {
		if customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
			inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
				{Text: s.translation.GetText(customer.Language, "connect_button"), WebApp: &models.WebAppInfo{
					URL: *customer.SubscriptionLink,
				}},
			})
			inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
				{Text: s.translation.GetText(customer.Language, "happ_proxy_button"), URL: *customer.SubscriptionLink},
			})
		} else {
			inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
				{Text: s.translation.GetText(customer.Language, "connect_button"), CallbackData: "connect"},
			})
		}
	}

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		{Text: s.translation.GetText(customer.Language, "back_button"), CallbackData: "start"},
	})
	return inlineCustomerKeyboard
}

func (s PaymentService) CreatePurchase(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, invoiceType database.InvoiceType, promoCode string) (url string, purchaseId int64, err error) {
	var promoID *int64
	if promoCode != "" && s.promoCodeRepository != nil {
		promo, err := s.promoCodeRepository.FindByCode(ctx, promoCode)
		if err == nil && promo != nil {
			// Atomically claim a usage slot (checks expiry + limit in one UPDATE)
			claimed, claimErr := s.promoCodeRepository.IncrementUsageAtomic(ctx, promo.ID)
			if claimErr != nil {
				slog.Error("Failed to claim promo slot", "error", claimErr, "code", promoCode)
			} else if claimed {
				discount := float64(promo.DiscountPercent) / 100.0
				amount = amount * (1 - discount)
				amount = math.Round(amount)
				promoID = &promo.ID
			}
		}
	}

	switch invoiceType {
	case database.InvoiceTypeCrypto:
		return s.createCryptoInvoice(ctx, amount, days, trafficLimitGB, customer, promoID)
	case database.InvoiceTypeMobileBanking:
		return s.createMobileBankingPurchase(ctx, amount, days, trafficLimitGB, customer, promoID)
	default:
		return "", 0, fmt.Errorf("unknown invoice type: %s", invoiceType)
	}
}

func (s PaymentService) createCryptoInvoice(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, promoID *int64) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeCrypto,
		Status:         database.PurchaseStatusNew,
		Amount:         amount,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Month:          0,
		Days:           days,
		TrafficLimitGB: trafficLimitGB,
		PromoCodeID:    promoID,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	invoice, err := s.cryptoPayClient.CreateInvoice(&cryptopay.InvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           "RUB",
		Amount:         fmt.Sprintf("%d", int(amount)),
		AcceptedAssets: "USDT",
		Payload:        fmt.Sprintf("purchaseId=%d&username=%s", purchaseId, ctx.Value("username")),
		Description:    fmt.Sprintf("Subscription for %d days", days),
		PaidBtnName:    "callback",
		PaidBtnUrl:     config.BotURL(),
	})
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"crypto_invoice_url": invoice.BotInvoiceUrl,
		"crypto_invoice_id":  invoice.InvoiceID,
		"status":             database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.BotInvoiceUrl, purchaseId, nil
}

func (s PaymentService) ActivateTrial(ctx context.Context, telegramId int64) (string, error) {
	if config.TrialDays() == 0 {
		return "", nil
	}
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return "", err
	}
	if customer == nil {
		return "", fmt.Errorf("customer %d not found", telegramId)
	}
	user, err := s.remnawaveClient.CreateOrUpdateUser(ctx, customer.ID, telegramId, config.TrialTrafficLimit(), config.TrialDays(), true)
	if err != nil {
		slog.Error("Error creating user", "error", err)
		return "", err
	}

	customerFilesToUpdate := map[string]interface{}{
		"subscription_link": user.GetSubscriptionUrl(),
		"expire_at":         user.GetExpireAt(),
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return "", err
	}

	return user.GetSubscriptionUrl(), nil

}

func (s PaymentService) createMobileBankingPurchase(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, promoID *int64) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Status:         database.PurchaseStatusPending,
		Amount:         amount,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Month:          0,
		Days:           days,
		TrafficLimitGB: trafficLimitGB,
		PromoCodeID:    promoID,
	})
	if err != nil {
		slog.Error("Error creating mobile banking purchase", "error", err)
		return "", 0, err
	}

	slog.Info("Mobile banking purchase created", "purchase_id", utils.MaskHalfInt64(purchaseId), "customer_id", utils.MaskHalfInt64(customer.ID))
	// No external URL needed — user sends screenshot directly
	return "", purchaseId, nil
}

// VerificationResult holds the outcome of a mobile payment screenshot check.
type VerificationResult struct {
	Success   bool
	Reason    string
	ReasonKey string // translation key
}

func (s PaymentService) VerifyMobilePayment(ctx context.Context, purchaseID int64, imageBytes []byte, mimeType string) (*VerificationResult, error) {
	if s.geminiClient == nil {
		return &VerificationResult{Success: false, Reason: "Mobile banking not configured", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return nil, fmt.Errorf("error finding purchase %d: %w", purchaseID, err)
	}
	if purchase == nil {
		return nil, fmt.Errorf("purchase %d not found", purchaseID)
	}

	// Guard: prevent re-verification of already-paid purchases
	if purchase.Status == database.PurchaseStatusPaid {
		return &VerificationResult{Success: false, Reason: "Purchase already completed", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	info, err := s.geminiClient.AnalyzePaymentScreenshot(ctx, imageBytes, mimeType)
	if err != nil {
		slog.Error("Gemini analysis failed", "error", err, "purchase_id", purchaseID)
		return &VerificationResult{Success: false, Reason: "Could not analyze screenshot", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	if !info.IsValid {
		slog.Warn("Gemini flagged screenshot as invalid", "purchase_id", purchaseID)
		return &VerificationResult{Success: false, Reason: "Screenshot does not appear to be a valid payment confirmation", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	// Check for image tampering (Photoshop, AI generation, etc.)
	if info.TamperingDetected {
		slog.Warn("Gemini detected image tampering", "purchase_id", purchaseID, "provider", info.Provider)
		return &VerificationResult{Success: false, Reason: "Screenshot appears to be altered or manipulated. Please upload an original, unedited screenshot.", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	// 1. Check transaction ID not empty
	if strings.TrimSpace(info.TransactionID) == "" {
		return &VerificationResult{Success: false, Reason: "No transaction ID found", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	// 2. Check for duplicate transaction ID
	exists, err := s.mobilePaymentRepo.ExistsByTransactionID(ctx, info.TransactionID)
	if err != nil {
		slog.Error("Error checking duplicate txn", "error", err)
		return nil, err
	}
	if exists {
		return &VerificationResult{Success: false, Reason: "Duplicate transaction ID", ReasonKey: "mobile_pay_failed_duplicate"}, nil
	}

	// 3. Check phone number matches configured receiving phone.
	// Some banking apps mask part of the number, so we compare last 4 digits.
	expectedPhone := normalizePhone(config.MobileBankingPhone())
	actualPhone := normalizePhone(info.PhoneNumber)
	if !phoneMatchesSuffix(expectedPhone, actualPhone, 4) {
		slog.Warn("Phone mismatch", "expected", expectedPhone, "got", actualPhone, "purchase_id", purchaseID)
		return &VerificationResult{Success: false, Reason: "Wrong recipient phone number", ReasonKey: "mobile_pay_failed_phone"}, nil
	}

	// 4. Check note for forbidden keywords
	noteLower := strings.ToLower(info.Note)
	if strings.Contains(noteLower, "vpn") || strings.Contains(noteLower, "outline") {
		slog.Warn("Payment note contains forbidden keyword", "note", info.Note, "purchase_id", purchaseID)
		return &VerificationResult{Success: false, Reason: "Payment note contains forbidden keyword", ReasonKey: "mobile_pay_failed_note"}, nil
	}

	// 5. Check amount matches (exact integer match)
	expectedAmount := purchase.Amount
	if math.Abs(info.Amount-expectedAmount) > 0.5 {
		slog.Warn("Amount mismatch", "expected", expectedAmount, "got", info.Amount, "purchase_id", purchaseID)
		return &VerificationResult{
			Success:   false,
			Reason:    fmt.Sprintf("Amount mismatch: expected %.0f, got %.0f", expectedAmount, info.Amount),
			ReasonKey: "mobile_pay_failed_amount",
		}, nil
	}

	// All checks passed — record verification and process
	_, err = s.mobilePaymentRepo.Create(ctx, &database.MobilePaymentVerification{
		PurchaseID:    purchaseID,
		TransactionID: info.TransactionID,
		Provider:      info.Provider,
		PhoneNumber:   info.PhoneNumber,
		Amount:        info.Amount,
		Note:          info.Note,
		Verified:      true,
	})
	if err != nil {
		slog.Error("Error recording mobile payment", "error", err)
		return nil, err
	}

	// Copy payment details to purchase row for revenue tracking
	now := time.Now()
	_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{
		"transaction_id": info.TransactionID,
		"payment_method": info.Provider,
		"payment_phone":  info.PhoneNumber,
		"verified_at":    now,
	})

	err = s.ProcessPurchaseById(ctx, purchaseID)
	if err != nil {
		slog.Error("Error processing verified mobile purchase", "error", err)
		return nil, err
	}

	slog.Info("Mobile payment verified and processed", "purchase_id", purchaseID, "txn_id", info.TransactionID, "provider", info.Provider)
	return &VerificationResult{Success: true, ReasonKey: "mobile_pay_success"}, nil
}

// normalizePhone strips formatting and country code to produce a comparable local number.
func normalizePhone(phone string) string {
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	phone = strings.ReplaceAll(phone, "*", "") // masked digits
	phone = strings.TrimPrefix(phone, "+")
	// Myanmar country code is 95. E.g. +959xxxxxxxx → 959xxxxxxxx.
	// Local format is 09xxxxxxxx.
	// Normalize both to 9xxxxxxxx (strip leading 95 or 0 but keep the 9).
	if strings.HasPrefix(phone, "95") && len(phone) > 9 {
		phone = phone[2:] // +959xxx → 9xxx
	}
	phone = strings.TrimPrefix(phone, "0") // 09xxx → 9xxx
	return phone
}

// phoneMatchesSuffix checks if two phone numbers share the same last N digits.
// This handles cases where banking apps mask/truncate the phone number.
func phoneMatchesSuffix(expected, actual string, n int) bool {
	if actual == "" {
		return false
	}
	// If we have the full number, try exact match first
	if actual == expected {
		return true
	}
	// Fall back to last N digits comparison
	expSuffix := expected
	if len(expected) > n {
		expSuffix = expected[len(expected)-n:]
	}
	actSuffix := actual
	if len(actual) > n {
		actSuffix = actual[len(actual)-n:]
	}
	return expSuffix == actSuffix
}
