package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"sort"
	"strconv"
	"strings"
)

func RegisterHandlers(mux *http.ServeMux, customerRepo *database.CustomerRepository) {
	handler := NewAPIHandler(customerRepo)

	// Middleware chain
	withAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return corsMiddleware(authMiddleware(next))
	}
	public := func(next http.HandlerFunc) http.HandlerFunc {
		return corsMiddleware(next)
	}

	mux.HandleFunc("/api/me", withAuth(handler.GetMe))
	mux.HandleFunc("/api/plans", public(handler.GetPlans))

	// Serve React Frontend (SPA support)
	fs := http.FileServer(http.Dir("./web-app/dist"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If path exists in static, serve it
		// If not (and not /api), serve index.html for SPA routing
		// Simple approach: http.FileServer
		// Better approach for SPA: check if file exists, else serve index.html
		// For MVP: FileServer is fine if we don't use pushState deeply or if we just serve the root
		fs.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// Try query param for dev convenience? No, stick to header
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Expected format: "tma <initData>" directory from Telegram
		// But usually we just send the initData string. Let's assume the raw initData string is passed.
		initData := strings.TrimPrefix(authHeader, "tma ")

		telegramID, err := validateInitData(initData, config.TelegramToken())
		if err != nil {
			http.Error(w, "Invalid initData: "+err.Error(), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "telegram_id", telegramID)
		next(w, r.WithContext(ctx))
	}
}

// validateInitData validates the Telegram Web App initData and returns the user ID
func validateInitData(initData string, botToken string) (int64, error) {
	// Parse query string
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, err
	}

	hash := values.Get("hash")
	if hash == "" {
		return 0, fmt.Errorf("hash missing")
	}

	// Remove hash from values to compute data-check-string
	values.Del("hash")

	// Sort keys alphabetically
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build data-check-string
	var dataCheckArr []string
	for _, k := range keys {
		dataCheckArr = append(dataCheckArr, fmt.Sprintf("%s=%s", k, values.Get(k)))
	}
	dataCheckString := strings.Join(dataCheckArr, "\n")

	// Compute HMAC-SHA256 signature
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))
	secret := secretKey.Sum(nil)

	h := hmac.New(sha256.New, secret)
	h.Write([]byte(dataCheckString))
	computedHash := hex.EncodeToString(h.Sum(nil))

	if computedHash != hash {
		return 0, fmt.Errorf("hash mismatch")
	}

	// Check auth_date to prevent replay attacks
	authDateStr := values.Get("auth_date")
	if authDateStr == "" {
		return 0, fmt.Errorf("auth_date missing")
	}
	authDate, err := strconv.ParseInt(authDateStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid auth_date")
	}
	if time.Now().Unix()-authDate > 86400 {
		return 0, fmt.Errorf("initData expired")
	}

	// Extract user ID
	userStr := values.Get("user")
	if userStr == "" {
		return 0, fmt.Errorf("user data missing")
	}

	// Simple JSON parsing to get ID
	// Telegram sends: {"id":123,"first_name":"...","last_name":"...","username":"...","language_code":"..."}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(userStr), &user); err != nil {
		return 0, err
	}

	return user.ID, nil
}
