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
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/translation"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
)

type contextKey string

const (
	telegramIDKey contextKey = "telegram_id"
	usernameKey   contextKey = "username"
)

func RegisterHandlers(mux *http.ServeMux, customerRepo *database.CustomerRepository, paymentService *payment.PaymentService, telegramBot *bot.Bot, tm *translation.Manager, subKeyRepo *database.SubscriptionKeyRepository) {
	handler := NewAPIHandler(customerRepo, paymentService, telegramBot, tm, subKeyRepo)

	// Middleware chain
	withAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return corsMiddleware(authMiddleware(next))
	}
	public := func(next http.HandlerFunc) http.HandlerFunc {
		return corsMiddleware(next)
	}

	mux.HandleFunc("/api/me", withAuth(handler.GetMe))
	mux.HandleFunc("/api/plans", public(handler.GetPlans))
	mux.HandleFunc("/api/purchase", withAuth(handler.CreatePurchase))
	mux.HandleFunc("/api/upload_screenshot", withAuth(handler.UploadScreenshot))
	mux.HandleFunc("/api/purchase/status", withAuth(handler.GetPurchaseStatus))

	// Deep link redirect — opens in system browser to handle custom URL schemes
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("url")
		if target == "" {
			http.Error(w, "Missing url parameter", http.StatusBadRequest)
			return
		}
		// Only allow happ:// scheme for security
		if !strings.HasPrefix(target, "happ://") {
			http.Error(w, "Unsupported URL scheme", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><head>
<meta http-equiv="refresh" content="0;url=%s">
<title>Redirecting...</title>
<style>body{background:#1a1a2e;color:#fff;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{text-align:center}.spinner{border:3px solid #333;border-top:3px solid #00d4ff;border-radius:50%%;width:40px;height:40px;animation:spin 1s linear infinite;margin:0 auto 16px}
@keyframes spin{to{transform:rotate(360deg)}}</style>
</head><body>
<div class="box"><div class="spinner"></div><p>Opening Happ...</p>
<p style="font-size:13px;opacity:0.6">If nothing happens, <a href="%s" style="color:#00d4ff">tap here</a></p></div>
<script>window.location.href="%s";</script>
</body></html>`, target, target, target)
	})

	// Serve React Frontend (SPA support)
	fs := http.FileServer(http.Dir("./web-app/dist"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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

		// Expected format: "twa <initData>" from Telegram Web App frontend
		initData := strings.TrimPrefix(authHeader, "twa ")

		telegramID, username, err := validateInitData(initData, config.TelegramToken())
		if err != nil {
			http.Error(w, "Invalid initData: "+err.Error(), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), telegramIDKey, telegramID)
		if username != "" {
			ctx = context.WithValue(ctx, usernameKey, username)
		}
		next(w, r.WithContext(ctx))
	}
}

// validateInitData validates the Telegram Web App initData and returns the user ID and Username
func validateInitData(initData string, botToken string) (int64, string, error) {
	// Parse query string
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, "", err
	}

	hash := values.Get("hash")
	if hash == "" {
		return 0, "", fmt.Errorf("hash missing")
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
		return 0, "", fmt.Errorf("hash mismatch")
	}

	// Check auth_date to prevent replay attacks
	authDateStr := values.Get("auth_date")
	if authDateStr == "" {
		return 0, "", fmt.Errorf("auth_date missing")
	}
	authDate, err := strconv.ParseInt(authDateStr, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid auth_date")
	}
	if time.Now().Unix()-authDate > 86400 {
		return 0, "", fmt.Errorf("initData expired")
	}

	// Extract user ID
	userStr := values.Get("user")
	if userStr == "" {
		return 0, "", fmt.Errorf("user data missing")
	}

	// Simple JSON parsing to get ID
	// Telegram sends: {"id":123,"first_name":"...","last_name":"...","username":"...","language_code":"..."}
	var user struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal([]byte(userStr), &user); err != nil {
		return 0, "", err
	}

	return user.ID, user.Username, nil
}
