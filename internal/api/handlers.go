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
	User            *database.Customer `json:"user"`
	Keys            []KeyResponse      `json:"keys"`
	IsActive        bool               `json:"is_active"`
	ExpireAt        *time.Time         `json:"expire_at"`
	DaysRemaining   int                `json:"days_remaining"`
}

// ... PlanResponse struct ...

// ... APIHandler struct ...

// ... NewAPIHandler func ...

// ... CreatePurchaseRequest/Response ...

// ... CreatePurchase func ...

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

	// Sync keys with Remnawave to get fresh stats and status
	syncedKeys, err := h.paymentService.SyncKeys(r.Context(), customer.ID, customer.TelegramID)
	// If sync fails, fall back to local DB? Alternatively just log error and return empty or old?
	// For "real-time" requirement, we want the synced data. If API down, use local DB as backup.
	var keys []KeyResponse

	const bytesInGB = 1073741824.0

	// Use synced keys if available
	if err == nil && syncedKeys != nil {
		if h.subKeyRepo != nil {
			// Reload local keys to get correct static fields (Label, URL), but use stats from sync
			localKeys, _ := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
			
			// Map for quick lookup
			statsMap := make(map[int64]payment.KeyStats)
			for _, sk := range syncedKeys {
				statsMap[sk.ID] = sk
			}

			for _, k := range localKeys {
				stat, exists := statsMap[k.ID]
				// If not in sync map, it might be deleted or sync failed just for this one.
				// Based on SyncKeys logic, if it's not returned it's effectively gone or sync logic handled it.
				// But SyncKeys returns the list of ACTIVE/EXPIRED keys only (deleted ones are skipped in loop possibly? No, SyncKeys returns result struct).
				// We should iterate over result from SyncKeys primarily OR filter localKeys by status != deleted.
				
				// Better approach: Iterate localKeys. If status is deleted, skip.
				// If status is active/expired, match with stats.
				
				// Re-fetch to be safe after sync updates
				if k.Status == "deleted" { 
					continue 
				}
				
				// If we have fresh stats, use them
				validStat := stat
				if !exists {
					// Should ideally check DB status again if SyncKeys updated it to deleted
					// But for now let's skip if no stats found (implies deleted externally)
					// Actually SyncKeys updates DB. So if we re-fetch `localKeys` AFTER `SyncKeys`, we are good.
					// Let's rely on the returned `syncedKeys` which contains the ID.
					// We need to merge Synced Data (Stats) + DB Data (Label/URL).
				}
			}
			
			// Re-query DB to get the latest status updates applied by SyncKeys
			freshLocalKeys, _ := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
			for _, k := range freshLocalKeys {
				if k.Status == "deleted" {
					continue
				}
				
				kDays := 0
				if k.ExpireAt != nil && k.ExpireAt.After(time.Now()) {
					kDays = int(time.Until(*k.ExpireAt).Hours() / 24)
				}

				// Find traffic stats
				usedGB := 0.0
				limitGB := 0.0
				if stat, ok := statsMap[k.ID]; ok {
					usedGB = float64(stat.TrafficUsedBytes) / bytesInGB
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
		}
	} else {
		// Fallback to local DB if sync failed
		if h.subKeyRepo != nil {
			subKeys, _ := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
			for _, k := range subKeys {
				if k.Status == "deleted" { continue }
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
					// No traffic stats in fallback
				})
			}
		}
	}

	// Fallback: if no keys in table but customer has a subscription link, show it (Legacy)
	if len(keys) == 0 && customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
		keys = append(keys, KeyResponse{
			ID:              0,
			Label:           "Key 1",
			SubscriptionURL: *customer.SubscriptionLink,
			HappLink:        "happ://add/" + *customer.SubscriptionLink,
			ExpireAt:        customer.ExpireAt,
			DaysRemaining:   daysRemaining,
			Status:          func() string { if isActive { return "active" }; return "expired" }(),
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

	
	resp := PurchaseStatusResponse{
		ID:     purchase.ID,
		Status: string(purchase.Status),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
