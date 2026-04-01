package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/translation"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
)

// --- Response types ---

type KeyResponse struct {
	ID              int64      `json:"id"`
	Label           string     `json:"label"`
	Username        string     `json:"username"`
	SubscriptionURL string     `json:"subscription_url"`
	HappLink        string     `json:"happ_link"`
	ExpireAt        *time.Time `json:"expire_at"`
	DaysRemaining   int        `json:"days_remaining"`
	Status          string     `json:"status"`
	TrafficUsedGB   float64    `json:"traffic_used_gb"`
	TrafficLimitGB  float64    `json:"traffic_limit_gb"`
	AutoRenew       bool       `json:"auto_renew"`
}

type ValidationResponse struct {
	User                *database.Customer `json:"user"`
	Keys                []KeyResponse      `json:"keys"`
	IsActive            bool               `json:"is_active"`
	ExpireAt            *time.Time         `json:"expire_at"`
	DaysRemaining       int                `json:"days_remaining"`
	TrialEligible       bool               `json:"trial_eligible"`
	TrialDays           int                `json:"trial_days"`
	ReferralCount       int                `json:"referral_count"`
	ReferralEarned      float64            `json:"referral_earned"`
	ReferralBonusAmount float64            `json:"referral_bonus_amount"`
	BotURL              string             `json:"bot_url"`
}

type PlanResponse struct {
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	Currency       string `json:"currency"`
}

type CreatePurchaseRequest struct {
	PlanIndex     int    `json:"plan_index"`
	ExtendKeyID   *int64 `json:"extend_key_id,omitempty"`
	PromoCode     string `json:"promo_code,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
	Amount        int    `json:"amount,omitempty"` // Explicit amount for wallet top-up
}

type CreatePurchaseResponse struct {
	PurchaseID       int64                     `json:"purchase_id"`
	PaymentPhone     string                    `json:"payment_phone,omitempty"`
	PaymentPhones    map[string]string         `json:"payment_phones,omitempty"`
	PaymentProviders []payment.PaymentProvider `json:"payment_providers,omitempty"`
	Amount           int                       `json:"amount"`
	Currency         string                    `json:"currency"`
	Instructions     string                    `json:"instructions,omitempty"`
	InvoiceType      string                    `json:"invoice_type"`
	BotURL           string                    `json:"bot_url"`
	HappLink         string                    `json:"happ_link,omitempty"`
}

// WalletServiceInterface defines the interface for wallet operations
type WalletServiceInterface interface {
	GetBalance(ctx context.Context, customerID int64) (float64, error)
	GetTransactionHistory(ctx context.Context, customerID int64, limit int) ([]database.WalletTransaction, error)
	HasSufficientBalance(ctx context.Context, customerID int64, amount float64) (bool, error)
	DeductBalance(ctx context.Context, customerID int64, amount float64, purchaseID int64, description string) error
	SetAutoRenew(ctx context.Context, customerID int64, enabled bool, duration int) error
	GetAutoRenewStatus(ctx context.Context, customerID int64) (enabled bool, duration int, err error)
	SetKeyAutoRenew(ctx context.Context, keyID int64, customerID int64, enabled bool) error
}

type UploadScreenshotResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Reason   string `json:"reason,omitempty"`
	HappLink string `json:"happ_link,omitempty"`
}

type PurchaseStatusResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

func parsePaymentMethod(method string) (database.InvoiceType, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", "mobile_banking":
		return database.InvoiceTypeMobileBanking, nil
	case "crypto":
		return database.InvoiceTypeCrypto, nil
	case "wallet":
		return database.InvoiceTypeWalletPayment, nil
	case "wallet_topup":
		return database.InvoiceTypeWalletTopUp, nil
	default:
		return "", fmt.Errorf("unsupported payment_method %q", method)
	}
}

// --- Handler ---

type APIHandler struct {
	customerRepo        *database.CustomerRepository
	paymentService      *payment.PaymentService
	telegramBot         *bot.Bot
	translation         *translation.Manager
	subKeyRepo          *database.SubscriptionKeyRepository
	promoCodeRepository *database.PromoCodeRepository
	walletService       WalletServiceInterface
	referralRepo        *database.ReferralRepository
}

func NewAPIHandler(
	customerRepo *database.CustomerRepository,
	paymentService *payment.PaymentService,
	telegramBot *bot.Bot,
	tm *translation.Manager,
	subKeyRepo *database.SubscriptionKeyRepository,
	promoCodeRepository *database.PromoCodeRepository,
	walletService WalletServiceInterface,
	referralRepo *database.ReferralRepository,
) *APIHandler {
	return &APIHandler{
		customerRepo:        customerRepo,
		paymentService:      paymentService,
		telegramBot:         telegramBot,
		translation:         tm,
		subKeyRepo:          subKeyRepo,
		promoCodeRepository: promoCodeRepository,
		walletService:       walletService,
		referralRepo:        referralRepo,
	}
}

// --- Handlers ---

func (h *APIHandler) ValidatePromo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code", http.StatusBadRequest)
		return
	}

	promo, err := h.promoCodeRepository.FindByCode(r.Context(), code)
	// Return the same 404 for all invalid/exhausted/expired cases to prevent
	// oracle attacks that distinguish between "code never existed" vs "code exhausted".
	if err != nil || promo == nil {
		http.Error(w, "Invalid or expired code", http.StatusNotFound)
		return
	}
	if promo.UsedCount >= promo.MaxUses {
		http.Error(w, "Invalid or expired code", http.StatusNotFound)
		return
	}
	if time.Now().After(promo.ValidUntil) {
		http.Error(w, "Invalid or expired code", http.StatusNotFound)
		return
	}

	resp := map[string]interface{}{
		"valid":            true,
		"code":             promo.Code,
		"discount_percent": promo.DiscountPercent,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) CreatePurchase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreatePurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	invoiceType, err := parsePaymentMethod(req.PaymentMethod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var price float64
	var days int
	var trafficLimit int
	var label string

	if invoiceType == database.InvoiceTypeWalletTopUp {
		if req.Amount <= 0 {
			http.Error(w, "Invalid amount for top-up", http.StatusBadRequest)
			return
		}
		if req.PromoCode != "" {
			http.Error(w, "promo_code is not valid for wallet top-up", http.StatusBadRequest)
			return
		}
		// For top-up, we use the explicit amount
		price = float64(req.Amount)
		days = 0
		trafficLimit = 0
		label = fmt.Sprintf("Wallet Top-up: %d %s", req.Amount, config.Currency())
	} else {
		plan := config.PlanByIndex(req.PlanIndex)
		if plan == nil {
			http.Error(w, "Invalid plan index", http.StatusBadRequest)
			return
		}
		price = float64(plan.Price)
		days = plan.Days
		trafficLimit = plan.TrafficLimitGB
		label = plan.Label
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if req.ExtendKeyID != nil {
		if invoiceType == database.InvoiceTypeWalletTopUp {
			http.Error(w, "extend_key_id is not valid for wallet top-up", http.StatusBadRequest)
			return
		}
		if h.subKeyRepo == nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		extendKey, err := h.subKeyRepo.FindByID(r.Context(), *req.ExtendKeyID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if extendKey == nil {
			http.Error(w, "Subscription key not found", http.StatusNotFound)
			return
		}
		if extendKey.CustomerID != customer.ID {
			http.Error(w, "Purchase not allowed", http.StatusForbidden)
			return
		}
	}

	var purchaseID int64
	if req.ExtendKeyID != nil && invoiceType == database.InvoiceTypeWalletPayment {
		_, purchaseID, err = h.paymentService.CreatePurchaseWithExtend(r.Context(), price, days, trafficLimit, customer, *req.ExtendKeyID, req.PromoCode)
	} else {
		// Delegate to PaymentService with Promo Code
		_, purchaseID, err = h.paymentService.CreatePurchase(r.Context(), price, days, trafficLimit, customer, invoiceType, req.PromoCode)
	}
	if err != nil {
		http.Error(w, "Failed to create purchase: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch the actual purchase to get the final amount (which might be discounted)
	purchase, err := h.paymentService.GetPurchaseByID(r.Context(), purchaseID)
	if err != nil || purchase == nil {
		slog.Error("Failed to retrieve purchase after creation", "purchase_id", purchaseID, "error", err)
		http.Error(w, "Failed to retrieve purchase details", http.StatusInternalServerError)
		return
	}

	updateFields := map[string]interface{}{
		"plan_label":    label,
		"payment_phone": payment.GetFirstPaymentPhone(),
	}
	if req.ExtendKeyID != nil && invoiceType != database.InvoiceTypeWalletPayment {
		updateFields["extend_key_id"] = *req.ExtendKeyID
	}
	if err := h.paymentService.UpdatePurchaseFields(r.Context(), purchaseID, updateFields); err != nil {
		slog.Warn("Failed to update purchase fields", "purchase_id", purchaseID, "error", err)
	}

	var instructions string
	var mobileNumber string
	var paymentPhones map[string]string
	var paymentProviders []payment.PaymentProvider
	if purchase.InvoiceType == database.InvoiceTypeMobileBanking || purchase.InvoiceType == database.InvoiceTypeWalletTopUp {
		instructions = fmt.Sprintf(
			h.translation.GetText(customer.Language, "mobile_pay_instructions"),
			int(purchase.Amount),
			payment.BuildPaymentReceiversHTML(),
		)
		mobileNumber = payment.GetFirstPaymentPhone()
		paymentPhones = payment.GetAllPaymentPhones()
		paymentProviders = payment.GetEnabledPaymentProviders()
	}

	happLink := ""
	if purchase.InvoiceType == database.InvoiceTypeWalletPayment {
		if h.subKeyRepo != nil {
			if req.ExtendKeyID != nil {
				if extendKey, kErr := h.subKeyRepo.FindByID(r.Context(), *req.ExtendKeyID); kErr == nil && extendKey != nil && extendKey.SubscriptionURL != "" {
					happLink = "happ://add/" + extendKey.SubscriptionURL
				}
			} else {
				keys, kErr := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
				if kErr == nil && len(keys) > 0 {
					latestKey := keys[0]
					happLink = "happ://add/" + latestKey.SubscriptionURL
				}
			}
		}
	}

	resp := CreatePurchaseResponse{
		PurchaseID:       purchase.ID,
		PaymentPhone:     mobileNumber,
		PaymentPhones:    paymentPhones,
		PaymentProviders: paymentProviders,
		Amount:           int(purchase.Amount),
		Currency:         config.Currency(),
		Instructions:     instructions,
		InvoiceType:      string(purchase.InvoiceType),
		BotURL:           config.BotURL(),
		HappLink:         happLink,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	isActive := false
	daysRemaining := 0
	if customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()) {
		isActive = true
		daysRemaining = int(time.Until(*customer.ExpireAt).Hours() / 24)
	}

	var keys []KeyResponse
	const bytesInGB = 1073741824.0

	// Track if user has any keys in the new system (even deleted ones).
	hasMigratedKeys := false

	cachedStats, hasCachedStats := h.paymentService.GetCachedSyncKeys(customer.ID)
	if !hasCachedStats {
		h.paymentService.TriggerSyncKeysAsync(r.Context(), customer.ID, customer.TelegramID)
	}
	statsMap := make(map[int64]payment.KeyStats, len(cachedStats))
	for _, sk := range cachedStats {
		statsMap[sk.ID] = sk
	}

	if h.subKeyRepo != nil {
		localKeys, _ := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
		if len(localKeys) > 0 {
			hasMigratedKeys = true
		}
		for _, k := range localKeys {
			if k.Status == "deleted" {
				continue
			}

			kDays := 0
			if k.ExpireAt != nil && k.ExpireAt.After(time.Now()) {
				kDays = int(time.Until(*k.ExpireAt).Hours() / 24)
			}

			usedGB, limitGB := 0.0, 0.0
			if stat, ok := statsMap[k.ID]; ok {
				usedGB = stat.TrafficUsedBytes / bytesInGB
				limitGB = float64(stat.TrafficLimitBytes) / bytesInGB
			} else if k.TrafficLimitGB > 0 {
				limitGB = float64(k.TrafficLimitGB)
			}

			keys = append(keys, KeyResponse{
				ID:              k.ID,
				Label:           k.Label,
				Username:        k.Username,
				SubscriptionURL: k.SubscriptionURL,
				HappLink:        "happ://add/" + k.SubscriptionURL,
				ExpireAt:        k.ExpireAt,
				DaysRemaining:   kDays,
				Status:          k.Status,
				TrafficUsedGB:   usedGB,
				TrafficLimitGB:  limitGB,
				AutoRenew:       k.AutoRenew,
			})
		}
	}

	// Legacy fallback: customer has subscription_link but no subscription_key rows (not migrated yet)
	if len(keys) == 0 && !hasMigratedKeys && customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
		status := "expired"
		if isActive {
			status = "active"
		}
		keys = append(keys, KeyResponse{
			ID:              0,
			Label:           "Key 1",
			SubscriptionURL: *customer.SubscriptionLink,
			HappLink:        "happ://add/" + *customer.SubscriptionLink,
			ExpireAt:        customer.ExpireAt,
			DaysRemaining:   daysRemaining,
			Status:          status,
		})
	}

	// Determine trial eligibility: trial enabled + no subscription history.
	trialEligible := config.TrialDays() > 0 && customer.SubscriptionLink == nil && len(keys) == 0 && !hasMigratedKeys

	// Fetch referral summary for the home chip (non-fatal)
	referralCount := 0
	var referralEarned float64
	if h.referralRepo != nil {
		if refs, err := h.referralRepo.FindByReferrer(r.Context(), customer.TelegramID); err == nil {
			referralCount = len(refs)
			for _, ref := range refs {
				if ref.BonusGranted {
					referralEarned += payment.ReferralBonusAmount
				}
			}
		}
	}

	resp := ValidationResponse{
		User:                customer,
		Keys:                keys,
		IsActive:            isActive,
		ExpireAt:            customer.ExpireAt,
		DaysRemaining:       daysRemaining,
		TrialEligible:       trialEligible,
		TrialDays:           config.TrialDays(),
		ReferralCount:       referralCount,
		ReferralEarned:      referralEarned,
		ReferralBonusAmount: payment.ReferralBonusAmount,
		BotURL:              config.BotURL(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) ActivateTrial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if config.TrialDays() == 0 {
		http.Error(w, "Trial is not available", http.StatusBadRequest)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// One trial per user: check if they ever had a subscription
	if customer.SubscriptionLink != nil {
		http.Error(w, "Trial already used", http.StatusConflict)
		return
	}
	if h.subKeyRepo != nil {
		keys, keyErr := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
		if keyErr != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if len(keys) > 0 {
			http.Error(w, "Trial already used", http.StatusConflict)
			return
		}
	}

	// Use actual username if available, otherwise stringified ID. Ideally this logic belongs in service layer.
	username, _ := r.Context().Value(payment.UsernameCtxKey).(string)
	if username == "" {
		username = fmt.Sprintf("%d", telegramID)
	}
	ctxWithUsername := context.WithValue(r.Context(), payment.UsernameCtxKey, username)
	subURL, err := h.paymentService.ActivateTrial(ctxWithUsername, telegramID)
	if err != nil {
		http.Error(w, "Failed to activate trial", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "activated",
		"subscription_url": subURL,
		"trial_days":       config.TrialDays(),
	})
}

func (h *APIHandler) GetPlans(w http.ResponseWriter, r *http.Request) {
	plans := config.Plans()
	var response []PlanResponse
	currency := config.Currency()

	for _, p := range plans {
		response = append(response, PlanResponse{
			Label:          p.Label,
			Days:           p.Days,
			Price:          p.Price,
			TrafficLimitGB: p.TrafficLimitGB,
			Currency:       currency,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *APIHandler) UploadScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	purchaseIDStr := r.URL.Query().Get("id")
	if purchaseIDStr == "" {
		http.Error(w, "Missing purchase id", http.StatusBadRequest)
		return
	}
	purchaseID, err := strconv.Atoi(purchaseIDStr)
	if err != nil {
		http.Error(w, "Invalid purchase id", http.StatusBadRequest)
		return
	}

	// === OWNERSHIP CHECK BEFORE READING FILE BODY ===
	// Do this first to avoid loading up to 10MB into RAM for unauthorized requests.
	purchase, err := h.paymentService.GetPurchaseByID(r.Context(), int64(purchaseID))
	if err != nil || purchase == nil {
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil || customer == nil || purchase.CustomerID != customer.ID {
		http.Error(w, "Purchase not allowed", http.StatusForbidden)
		return
	}

	// Now safe to read the file. Limit body to 10 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "File too big or invalid form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusBadRequest)
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(fileBytes)
	}

	result, err := h.paymentService.VerifyMobilePayment(r.Context(), int64(purchaseID), fileBytes, mimeType)
	if err != nil {
		http.Error(w, "Verification error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := UploadScreenshotResponse{Status: "failed"}
	if result.Success {
		resp.Status = "success"
		resp.Message = "Payment verified successfully!"
		// Look up the latest subscription key for this customer to build Happ deep link
		if h.subKeyRepo != nil {
			if purchase.ExtendKeyID != nil {
				if extendKey, kErr := h.subKeyRepo.FindByID(r.Context(), *purchase.ExtendKeyID); kErr == nil && extendKey != nil && extendKey.SubscriptionURL != "" {
					resp.HappLink = "happ://add/" + extendKey.SubscriptionURL
				}
			} else {
				keys, kErr := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
				if kErr == nil && len(keys) > 0 {
					latestKey := keys[0]
					resp.HappLink = "happ://add/" + latestKey.SubscriptionURL
				}
			}
		}
	} else {
		resp.Message = result.Reason
		resp.Reason = result.ReasonKey
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) GetPurchaseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	purchaseIDStr := r.URL.Query().Get("id")
	if purchaseIDStr == "" {
		http.Error(w, "Missing purchase id", http.StatusBadRequest)
		return
	}
	purchaseID, err := strconv.Atoi(purchaseIDStr)
	if err != nil {
		http.Error(w, "Invalid purchase id", http.StatusBadRequest)
		return
	}

	purchase, err := h.paymentService.GetPurchaseByID(r.Context(), int64(purchaseID))
	if err != nil || purchase == nil {
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil || customer == nil || purchase.CustomerID != customer.ID {
		http.Error(w, "Purchase not allowed", http.StatusForbidden)
		return
	}

	resp := PurchaseStatusResponse{
		ID:     purchase.ID,
		Status: string(purchase.Status),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) GetRevenueSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
			days = d
		}
	}

	summary, err := h.paymentService.GetRevenueSummary(r.Context(), days)
	if err != nil {
		http.Error(w, "Failed to fetch revenue: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure we return an empty array instead of null for consistency
	if summary == nil {
		summary = []database.RevenueSummaryRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// --- Wallet Handlers ---

func maskHalfInt64(id int64) string {
	s := strconv.FormatInt(id, 10)
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	maskLen := len(s) / 2
	visibleLen := len(s) - maskLen
	return strings.Repeat("*", maskLen) + s[visibleLen:]
}

func (h *APIHandler) GetReferrals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	type ReferralItem struct {
		ID        int64     `json:"id"`
		MaskedID  string    `json:"masked_id"`
		CreatedAt time.Time `json:"created_at"`
		Status    string    `json:"status"` // "bonus_received" or "pending"
	}

	var items []ReferralItem
	if h.referralRepo != nil {
		if refs, err := h.referralRepo.FindByReferrer(r.Context(), customer.TelegramID); err == nil {
			for _, ref := range refs {
				status := "pending"
				if ref.BonusGranted {
					status = "bonus_received"
				}
				items = append(items, ReferralItem{
					ID:        ref.ID,
					MaskedID:  maskHalfInt64(ref.RefereeID),
					CreatedAt: ref.UsedAt,
					Status:    status,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *APIHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	balance, err := h.walletService.GetBalance(r.Context(), customer.ID)
	if err != nil {
		http.Error(w, "Failed to get balance: "+err.Error(), http.StatusInternalServerError)
		return
	}

	autoRenew, autoRenewDuration, err := h.walletService.GetAutoRenewStatus(r.Context(), customer.ID)
	if err != nil {
		http.Error(w, "Failed to get auto-renew status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"balance":               balance,
		"currency":              config.Currency(),
		"auto_renew":            autoRenew,
		"auto_renew_duration":   autoRenewDuration,
		"bot_url":               config.BotURL(),
		"referral_bonus_amount": payment.ReferralBonusAmount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) GetWalletHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	history, err := h.walletService.GetTransactionHistory(r.Context(), customer.ID, limit)
	if err != nil {
		http.Error(w, "Failed to get transaction history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func (h *APIHandler) UpdateAutoRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Enabled  bool `json:"enabled"`
		Duration int  `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Validate duration if enabled
	if req.Enabled {
		isValidDuration := false
		for _, plan := range config.Plans() {
			if plan.Days == req.Duration {
				isValidDuration = true
				break
			}
		}
		if !isValidDuration {
			http.Error(w, "Invalid auto-renew duration", http.StatusBadRequest)
			return
		}
	}

	if err := h.walletService.SetAutoRenew(r.Context(), customer.ID, req.Enabled, req.Duration); err != nil {
		http.Error(w, "Failed to update auto-renew: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// UpdateKeyAutoRenew toggles the auto_renew flag on a specific subscription key.
// POST /api/keys/autorenew
// Body: { "key_id": 42, "enabled": true }
func (h *APIHandler) UpdateKeyAutoRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		KeyID   int64 `json:"key_id"`
		Enabled bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.KeyID == 0 {
		http.Error(w, "Missing key_id", http.StatusBadRequest)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// SetKeyAutoRenew validates that key.customer_id == customer.ID internally.
	if err := h.walletService.SetKeyAutoRenew(r.Context(), req.KeyID, customer.ID, req.Enabled); err != nil {
		http.Error(w, "Failed to update key auto-renew: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
