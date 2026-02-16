package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/translation"
	"strconv"
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
}

type ValidationResponse struct {
	User          *database.Customer `json:"user"`
	Keys          []KeyResponse      `json:"keys"`
	IsActive      bool               `json:"is_active"`
	ExpireAt      *time.Time         `json:"expire_at"`
	DaysRemaining int                `json:"days_remaining"`
}

type PlanResponse struct {
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	Currency       string `json:"currency"`
}

type CreatePurchaseRequest struct {
	PlanIndex   int    `json:"plan_index"`
	ExtendKeyID *int64 `json:"extend_key_id,omitempty"`
}

type CreatePurchaseResponse struct {
	PurchaseID   int64  `json:"purchase_id"`
	PaymentPhone string `json:"payment_phone"`
	Amount       int    `json:"amount"`
	Currency     string `json:"currency"`
	Instructions string `json:"instructions"`
	InvoiceType  string `json:"invoice_type"`
}

type UploadScreenshotResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

type PurchaseStatusResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

// --- Handler ---

type APIHandler struct {
	customerRepo   *database.CustomerRepository
	paymentService *payment.PaymentService
	telegramBot    *bot.Bot
	translation    *translation.Manager
	subKeyRepo     *database.SubscriptionKeyRepository
}

func NewAPIHandler(
	customerRepo *database.CustomerRepository,
	paymentService *payment.PaymentService,
	telegramBot *bot.Bot,
	tm *translation.Manager,
	subKeyRepo *database.SubscriptionKeyRepository,
) *APIHandler {
	return &APIHandler{
		customerRepo:   customerRepo,
		paymentService: paymentService,
		telegramBot:    telegramBot,
		translation:    tm,
		subKeyRepo:     subKeyRepo,
	}
}

// --- Handlers ---

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
	username, _ := r.Context().Value(usernameKey).(string)

	var req CreatePurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	plan := config.PlanByIndex(req.PlanIndex)
	if plan == nil {
		http.Error(w, "Invalid plan index", http.StatusBadRequest)
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

	invoiceType := database.InvoiceTypeMobileBanking
	ctxWithUsername := context.WithValue(r.Context(), "username", username)

	_, purchaseID, err := h.paymentService.CreatePurchase(ctxWithUsername, float64(plan.Price), plan.Days, plan.TrafficLimitGB, customer, invoiceType)
	if err != nil {
		http.Error(w, "Failed to create purchase: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Store plan label and payment phone for revenue tracking
	updateFields := map[string]interface{}{
		"plan_label":    plan.Label,
		"payment_phone": config.MobileBankingPhone(),
	}
	if req.ExtendKeyID != nil {
		updateFields["extend_key_id"] = *req.ExtendKeyID
	}
	purchaseRepo := h.paymentService.GetPurchaseRepository()
	_ = purchaseRepo.UpdateFields(r.Context(), purchaseID, updateFields)

	instructions := fmt.Sprintf(
		h.translation.GetText(customer.Language, "mobile_pay_instructions"),
		plan.Price,
		config.MobileBankingPhone(),
	)

	resp := CreatePurchaseResponse{
		PurchaseID:   purchaseID,
		PaymentPhone: config.MobileBankingPhone(),
		Amount:       plan.Price,
		Currency:     config.Currency(),
		Instructions: instructions,
		InvoiceType:  string(invoiceType),
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

	// Sync keys with Remnawave to get fresh stats and filter deleted keys.
	// SyncKeys updates the local DB (marks deleted, updates expiry/status).
	syncedKeys, syncErr := h.paymentService.SyncKeys(r.Context(), customer.ID, customer.TelegramID)

	if syncErr == nil && syncedKeys != nil && h.subKeyRepo != nil {
		// Build stats lookup from synced data
		statsMap := make(map[int64]payment.KeyStats, len(syncedKeys))
		for _, sk := range syncedKeys {
			statsMap[sk.ID] = sk
		}

		// Single DB query after sync to get labels/URLs with updated statuses
		localKeys, _ := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
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
			})
		}
	} else if h.subKeyRepo != nil {
		// Fallback: sync unavailable, use local DB only
		subKeys, _ := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
		for _, k := range subKeys {
			if k.Status == "deleted" {
				continue
			}
			kDays := 0
			if k.ExpireAt != nil && k.ExpireAt.After(time.Now()) {
				kDays = int(time.Until(*k.ExpireAt).Hours() / 24)
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
			})
		}
	}

	// Legacy fallback: customer has subscription_link but no subscription_key rows
	if len(keys) == 0 && customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
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

	resp := ValidationResponse{
		User:          customer,
		Keys:          keys,
		IsActive:      isActive,
		ExpireAt:      customer.ExpireAt,
		DaysRemaining: daysRemaining,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "File too big or invalid form", http.StatusBadRequest)
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

	purchase, err := h.paymentService.GetPurchaseRepository().FindById(r.Context(), int64(purchaseID))
	if err != nil || purchase == nil {
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil || customer == nil || purchase.CustomerID != customer.ID {
		http.Error(w, "Purchase not allowed", http.StatusForbidden)
		return
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

	purchase, err := h.paymentService.GetPurchaseRepository().FindById(r.Context(), int64(purchaseID))
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

	purchaseRepo := h.paymentService.GetPurchaseRepository()
	summary, err := purchaseRepo.GetRevenueSummary(r.Context(), days)
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
