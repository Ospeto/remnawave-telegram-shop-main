package handler

import (
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/notification"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
)

type Handler struct {
	customerRepository  *database.CustomerRepository
	purchaseRepository  *database.PurchaseRepository
	cryptoPayClient     *cryptopay.Client
	translation         *translation.Manager
	paymentService      *payment.PaymentService
	syncService         *sync.SyncService
	subscriptionService *notification.SubscriptionService
	referralRepository  *database.ReferralRepository
	promoCodeRepository *database.PromoCodeRepository
	cache               *cache.Cache
	mobilePayCache      *cache.Cache // telegramID → purchaseID for pending mobile screenshots
	TestMode            bool
}

func NewHandler(
	syncService *sync.SyncService,
	paymentService *payment.PaymentService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	subscriptionService *notification.SubscriptionService,
	referralRepository *database.ReferralRepository,
	promoCodeRepository *database.PromoCodeRepository,
	cache *cache.Cache,
	mobilePayCache *cache.Cache,
) *Handler {
	return &Handler{
		syncService:         syncService,
		paymentService:      paymentService,
		customerRepository:  customerRepository,
		purchaseRepository:  purchaseRepository,
		cryptoPayClient:     cryptoPayClient,
		translation:         translation,
		subscriptionService: subscriptionService,
		referralRepository:  referralRepository,
		promoCodeRepository: promoCodeRepository,
		cache:               cache,
		mobilePayCache:      mobilePayCache,
		TestMode:            false,
	}
}
