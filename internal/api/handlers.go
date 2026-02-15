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

type ValidationResponse struct {
	User            *database.Customer `json:"user"`
	SubscriptionUrl *string            `json:"subscription_url"`
	IsActive        bool               `json:"is_active"`
	ExpireAt        *time.Time         `json:"expire_at"`
	DaysRemaining   int                `json:"days_remaining"`
	HappLink        string             `json:"happ_link,omitempty"`
}

type PlanResponse struct {
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	Currency       string `json:"currency"`
}

type APIHandler struct {
	customerRepo   *database.CustomerRepository
	paymentService *payment.PaymentService
	telegramBot    *bot.Bot
	translation    *translation.Manager
}

func NewAPIHandler(customerRepo *database.CustomerRepository, paymentService *payment.PaymentService, telegramBot *bot.Bot, tm *translation.Manager) *APIHandler {
	return &APIHandler{
		customerRepo:   customerRepo,
		paymentService: paymentService,
		telegramBot:    telegramBot,
		translation:    tm,
	}
}

type CreatePurchaseRequest struct {
	PlanIndex int `json:"plan_index"`
}

type CreatePurchaseResponse struct {
	PurchaseID    int64  `json:"purchase_id"`
	PaymentPhone  string `json:"payment_phone"`
	Amount        int    `json:"amount"`
	Currency      string `json:"currency"`
	Instructions  string `json:"instructions"`
	InvoiceType   string `json:"invoice_type"`
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

	// Always use Mobile Banking for Mini App shop
	invoiceType := database.InvoiceTypeMobileBanking

	ctxWithUsername := context.WithValue(r.Context(), "username", username)
	_, purchaseID, err := h.paymentService.CreatePurchase(ctxWithUsername, float64(plan.Price), plan.Days, plan.TrafficLimitGB, customer, invoiceType)
	if err != nil {
		http.Error(w, "Failed to create purchase: "+err.Error(), http.StatusInternalServerError)
		return
	}

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

	happLink := ""
	if customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
		happLink = "happ://add/" + *customer.SubscriptionLink
	}

	resp := ValidationResponse{
		User:            customer,
		SubscriptionUrl: customer.SubscriptionLink,
		IsActive:        isActive,
		ExpireAt:        customer.ExpireAt,
		DaysRemaining:   daysRemaining,
		HappLink:        happLink,
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

type UploadScreenshotResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
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

	// limit to 10MB
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

	resp := UploadScreenshotResponse{
		Status: "failed",
	}

	if result.Success {
		resp.Status = "success"
		resp.Message = "Payment verified successfully!"
	} else {
		resp.Status = "failed"
		resp.Message = result.Reason
		resp.Reason = result.ReasonKey
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type PurchaseStatusResponse struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	PlanLabel string `json:"plan_label"`
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

	// Find plan label
	// We only store plan days/traffic in purchase, not label directly?
	// Actually we just return status. The frontend knows the plan details from previous step.
	// But let's try to infer label or just skip it.
	
	resp := PurchaseStatusResponse{
		ID:     purchase.ID,
		Status: string(purchase.Status),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
