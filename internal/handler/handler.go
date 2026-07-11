package handler

import (
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/notification"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/reporting"
	"remnawave-tg-shop-bot/internal/service/healthcheck"
	appSync "remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Handler struct {
	customerRepository  *database.CustomerRepository
	purchaseRepository  *database.PurchaseRepository
	translation         *translation.Manager
	paymentService      *payment.PaymentService
	syncService         *appSync.SyncService
	subscriptionService *notification.SubscriptionService
	subKeyRepo          *database.SubscriptionKeyRepository
	referralRepository  *database.ReferralRepository
	promoCodeRepository *database.PromoCodeRepository
	appConfigRepository *database.AppConfigRepository
	healthcheckService  *healthcheck.Service
	cache               *cache.Cache
	mobilePayCache      *cache.Cache // telegramID → purchaseID for pending mobile screenshots
	financeService      *reporting.FinanceService

	// Rate Limiting
	limitersMu *sync.Mutex
	limiters   map[int64]*rate.Limiter

	adminFlowsMu *sync.Mutex
	adminFlows   map[int64]adminFlowState
}

func NewHandler(
	syncService *appSync.SyncService,
	paymentService *payment.PaymentService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	subscriptionService *notification.SubscriptionService,
	subKeyRepo *database.SubscriptionKeyRepository,
	referralRepository *database.ReferralRepository,
	promoCodeRepository *database.PromoCodeRepository,
	appConfigRepository *database.AppConfigRepository,
	healthcheckService *healthcheck.Service,
	cache *cache.Cache,
	mobilePayCache *cache.Cache,
	financeService *reporting.FinanceService,
) *Handler {
	h := &Handler{
		syncService:         syncService,
		paymentService:      paymentService,
		customerRepository:  customerRepository,
		purchaseRepository:  purchaseRepository,
		translation:         translation,
		subscriptionService: subscriptionService,
		subKeyRepo:          subKeyRepo,
		referralRepository:  referralRepository,
		promoCodeRepository: promoCodeRepository,
		appConfigRepository: appConfigRepository,
		healthcheckService:  healthcheckService,
		cache:               cache,
		mobilePayCache:      mobilePayCache,
		financeService:      financeService,
		limiters:            make(map[int64]*rate.Limiter),
		limitersMu:          &sync.Mutex{},
		adminFlows:          make(map[int64]adminFlowState),
		adminFlowsMu:        &sync.Mutex{},
	}

	// Start cleanup loop for rate limiters
	go h.cleanupLimiters()
	return h
}

// cleanupLimiters periodically clears the map to prevent unbounded growth.
// Simplified approach: just wipe the map every hour to release memory.
// Active users will just get a new limiter.
func (h *Handler) cleanupLimiters() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		h.limitersMu.Lock()
		h.limiters = make(map[int64]*rate.Limiter)
		h.limitersMu.Unlock()
	}
}
