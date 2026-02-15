package api

import (
	"encoding/json"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"time"
)

type ValidationResponse struct {
	User            *database.Customer `json:"user"`
	SubscriptionUrl *string            `json:"subscription_url"`
	IsActive        bool               `json:"is_active"`
	ExpireAt        *time.Time         `json:"expire_at"`
}

type PlanResponse struct {
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	Currency       string `json:"currency"`
}

type APIHandler struct {
	customerRepo *database.CustomerRepository
}

func NewAPIHandler(customerRepo *database.CustomerRepository) *APIHandler {
	return &APIHandler{customerRepo: customerRepo}
}

func (h *APIHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	// User is already validated by middleware and stored in context?
	// For simplicity, we'll parse it again or expect middleware to set it.
	// Actually, let's keep it simple: Middleware validates initData, verifies hash,
	// and extracts the user ID. We'll simulate that for now or implement it fully.
	
	// Assuming middleware sets "telegram_id" in context
	telegramID, ok := r.Context().Value("telegram_id").(int64)
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
	if customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()) {
		isActive = true
	}

	resp := ValidationResponse{
		User:            customer,
		SubscriptionUrl: customer.SubscriptionLink,
		IsActive:        isActive,
		ExpireAt:        customer.ExpireAt,
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
