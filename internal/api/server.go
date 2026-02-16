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

func RegisterHandlers(mux *http.ServeMux, customerRepo *database.CustomerRepository, paymentService *payment.PaymentService, telegramBot *bot.Bot, tm *translation.Manager, subKeyRepo *database.SubscriptionKeyRepository, promoCodeRepository *database.PromoCodeRepository) {
	handler := NewAPIHandler(customerRepo, paymentService, telegramBot, tm, subKeyRepo, promoCodeRepository)

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
	mux.HandleFunc("/api/revenue", withAuth(handler.GetRevenueSummary))
	mux.HandleFunc("/api/promo/validate", withAuth(handler.ValidatePromo))

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
		// Extract the subscription URL from the deep link for copy fallback
		subURL := strings.TrimPrefix(target, "happ://add/")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><head>
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Open in Happ</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0f0f1a;color:#e0e0e0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px}
.card{background:#1a1a2e;border-radius:16px;padding:32px 24px;max-width:380px;width:100%%;text-align:center;box-shadow:0 8px 32px rgba(0,0,0,0.4)}
h1{font-size:20px;margin-bottom:8px;color:#fff}
.sub{font-size:14px;color:#888;margin-bottom:24px}
.spinner{border:3px solid #333;border-top:3px solid #00d4ff;border-radius:50%%;width:44px;height:44px;animation:spin .8s linear infinite;margin:0 auto 20px}
@keyframes spin{to{transform:rotate(360deg)}}
.phase-open .fallback{display:none}
.phase-fallback .loading{display:none}
.btn{display:block;width:100%%;padding:14px;border-radius:10px;font-size:15px;font-weight:600;text-decoration:none;margin-bottom:10px;border:none;cursor:pointer;transition:transform .15s}
.btn:active{transform:scale(0.97)}
.btn-primary{background:linear-gradient(135deg,#00d4ff,#0099cc);color:#fff}
.btn-android{background:linear-gradient(135deg,#34a853,#1e8e3e);color:#fff}
.btn-ios{background:linear-gradient(135deg,#007aff,#005bb5);color:#fff}
.btn-copy{background:#2a2a3e;color:#00d4ff;border:1px solid #333}
.btn-retry{background:#2a2a3e;color:#fff;border:1px solid #444}
.divider{color:#555;font-size:13px;margin:16px 0 12px}
.copied{color:#34a853;font-size:13px;margin-top:6px;opacity:0;transition:opacity .3s}
.copied.show{opacity:1}
.icon{font-size:48px;margin-bottom:16px}
</style>
</head><body>
<div class="card" id="card">
  <div class="loading">
    <div class="spinner"></div>
    <h1>Opening Happ...</h1>
    <p class="sub">Please wait</p>
  </div>
  <div class="fallback" style="display:none">
    <div class="icon">📱</div>
    <h1>Happ Not Found</h1>
    <p class="sub">Install Happ to add your VPN config automatically</p>
    <a id="btn-retry" href="%s" class="btn btn-retry">🔄 Try Again</a>
    <div class="divider">— install Happ —</div>
    <a id="btn-android" href="https://play.google.com/store/search?q=happ+vpn&c=apps" target="_blank" class="btn btn-android" style="display:none">▶️ Google Play Store</a>
    <a id="btn-ios" href="https://apps.apple.com/search?term=happ+vpn" target="_blank" class="btn btn-ios" style="display:none">🍎 App Store</a>
    <div class="divider">— or add manually —</div>
    <button onclick="copyUrl()" class="btn btn-copy">📋 Copy Subscription URL</button>
    <p class="copied" id="copied">✅ Copied to clipboard!</p>
  </div>
</div>
<script>
var deepLink = %q;
var subUrl = %q;

// Detect platform
var ua = navigator.userAgent || '';
var isAndroid = /android/i.test(ua);
var isIOS = /iphone|ipad|ipod/i.test(ua);

// Show the right store button
if (isAndroid) document.getElementById('btn-android').style.display = 'block';
else if (isIOS) document.getElementById('btn-ios').style.display = 'block';
else {
  document.getElementById('btn-android').style.display = 'block';
  document.getElementById('btn-ios').style.display = 'block';
}

// Try opening the deep link
var launched = false;
document.addEventListener('visibilitychange', function() {
  if (document.hidden) launched = true;
});

window.location.href = deepLink;

// If still here after 2.5s, app probably not installed
setTimeout(function() {
  if (!launched) {
    document.querySelector('.loading').style.display = 'none';
    document.querySelector('.fallback').style.display = 'block';
  }
}, 2500);

function copyUrl() {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(subUrl).then(function() { showCopied(); });
  } else {
    var t = document.createElement('textarea');
    t.value = subUrl;
    document.body.appendChild(t);
    t.select();
    document.execCommand('copy');
    document.body.removeChild(t);
    showCopied();
  }
}
function showCopied() {
  var el = document.getElementById('copied');
  el.classList.add('show');
  setTimeout(function() { el.classList.remove('show'); }, 2000);
}
</script>
</body></html>`, target, target, subURL)
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
