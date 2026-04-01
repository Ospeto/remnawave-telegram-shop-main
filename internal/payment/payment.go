package payment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/gemini"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
	"strings"
	"sync"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v4"
)

// testTransactionID is the magic bypass transaction ID used only in test mode.
// Unexported to prevent misuse; test mode must be enabled by admin command.
const testTransactionID = "01004063070995016447"

// ctxKey is an unexported type to prevent context key collisions across packages.
type ctxKey struct{}

// UsernameCtxKey is the typed context key for passing the Telegram username.
// Callers must use this exact key with context.WithValue.
var UsernameCtxKey = ctxKey{}

// syncCacheEntry stores a snapshot of synced keys with an expiry.
type syncCacheEntry struct {
	keys      []KeyStats
	expiresAt time.Time
}

type walletTopUpTx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type walletTopUpStore interface {
	BeginTx(ctx context.Context) (walletTopUpTx, error)
	AddBalance(ctx context.Context, tx walletTopUpTx, customerID int64, amount float64) error
	LogTopUp(ctx context.Context, tx walletTopUpTx, purchaseID int64, customerID int64, amount float64) error
}

type repositoryWalletTopUpStore struct {
	customerRepo *database.CustomerRepository
	walletTxRepo *database.WalletTransactionRepository
}

func (s repositoryWalletTopUpStore) BeginTx(ctx context.Context) (walletTopUpTx, error) {
	return s.customerRepo.BeginTx(ctx)
}

func (s repositoryWalletTopUpStore) AddBalance(ctx context.Context, tx walletTopUpTx, customerID int64, amount float64) error {
	pgxTx, ok := tx.(pgx.Tx)
	if !ok {
		return fmt.Errorf("wallet top-up transaction has unexpected type %T", tx)
	}
	return s.customerRepo.AddBalanceTx(ctx, pgxTx, customerID, amount)
}

func (s repositoryWalletTopUpStore) LogTopUp(ctx context.Context, tx walletTopUpTx, purchaseID int64, customerID int64, amount float64) error {
	if s.walletTxRepo == nil {
		return nil
	}

	pgxTx, ok := tx.(pgx.Tx)
	if !ok {
		return fmt.Errorf("wallet top-up transaction has unexpected type %T", tx)
	}

	_, err := s.walletTxRepo.CreateTx(ctx, pgxTx, &database.WalletTransaction{
		CustomerID:  customerID,
		Amount:      amount,
		Type:        database.WalletTransactionTypeTopup,
		PurchaseID:  &purchaseID,
		Description: "Wallet Top-up",
	})
	return err
}

type PaymentService struct {
	purchaseRepository  *database.PurchaseRepository
	remnawaveClient     *remnawave.Client
	customerRepository  *database.CustomerRepository
	telegramBot         *bot.Bot
	translation         *translation.Manager
	cryptoPayClient     *cryptopay.Client
	referralRepository  *database.ReferralRepository
	cache               *cache.Cache
	paymentAnalyzer     gemini.Analyzer
	mobilePaymentRepo   *database.MobilePaymentRepository
	subKeyRepo          *database.SubscriptionKeyRepository
	promoCodeRepository *database.PromoCodeRepository
	walletTxRepo        *database.WalletTransactionRepository
	testMode            bool
	testModeMu          sync.RWMutex
	syncCache           sync.Map // key: customerID int64, value: syncCacheEntry
	syncInFlight        sync.Map // key: customerID int64, value: struct{}
}

func NewPaymentService(
	translation *translation.Manager,
	purchaseRepository *database.PurchaseRepository,
	remnawaveClient *remnawave.Client,
	customerRepository *database.CustomerRepository,
	telegramBot *bot.Bot,
	cryptoPayClient *cryptopay.Client,
	referralRepository *database.ReferralRepository,
	cache *cache.Cache,
	paymentAnalyzer gemini.Analyzer,
	mobilePaymentRepo *database.MobilePaymentRepository,
	subKeyRepo *database.SubscriptionKeyRepository,
	promoCodeRepository *database.PromoCodeRepository,
	walletTxRepo *database.WalletTransactionRepository,
) *PaymentService {
	return &PaymentService{
		purchaseRepository:  purchaseRepository,
		remnawaveClient:     remnawaveClient,
		customerRepository:  customerRepository,
		telegramBot:         telegramBot,
		translation:         translation,
		cryptoPayClient:     cryptoPayClient,
		referralRepository:  referralRepository,
		cache:               cache,
		paymentAnalyzer:     paymentAnalyzer,
		mobilePaymentRepo:   mobilePaymentRepo,
		subKeyRepo:          subKeyRepo,
		promoCodeRepository: promoCodeRepository,
		walletTxRepo:        walletTxRepo,
	}
}

// GetPurchaseByID retrieves a purchase record by its ID.
// Prefer this over exposing the raw repository.
func (s *PaymentService) GetPurchaseByID(ctx context.Context, id int64) (*database.Purchase, error) {
	return s.purchaseRepository.FindById(ctx, id)
}

// GetRevenueSummary returns daily revenue aggregated over the last N days.
func (s *PaymentService) GetRevenueSummary(ctx context.Context, days int) ([]database.RevenueSummaryRow, error) {
	return s.purchaseRepository.GetRevenueSummary(ctx, days)
}

// UpdatePurchaseFields updates arbitrary (whitelisted) fields on a purchase record.
func (s *PaymentService) UpdatePurchaseFields(ctx context.Context, id int64, fields map[string]interface{}) error {
	return s.purchaseRepository.UpdateFields(ctx, id, fields)
}

func (s *PaymentService) SetTestMode(enabled bool) {
	s.testModeMu.Lock()
	defer s.testModeMu.Unlock()
	s.testMode = enabled
}

func (s *PaymentService) IsTestMode() bool {
	s.testModeMu.RLock()
	defer s.testModeMu.RUnlock()
	return s.testMode
}

// GetTestTransactionID returns the magic bypass transaction ID for test mode.
// Only display this in admin-only commands — never expose to regular users.
func (s *PaymentService) GetTestTransactionID() string {
	return testTransactionID
}

const syncCacheTTL = 2 * time.Minute

func settleWalletTopUp(
	ctx context.Context,
	store walletTopUpStore,
	purchaseID int64,
	customerID int64,
	amount float64,
	originalStatus database.PurchaseStatus,
	restore func(context.Context, int64, database.PurchaseStatus),
) error {
	dbTx, err := store.BeginTx(ctx)
	if err != nil {
		restore(ctx, purchaseID, originalStatus)
		return fmt.Errorf("failed to begin wallet top-up transaction: %w", err)
	}
	defer func() {
		_ = dbTx.Rollback(ctx)
	}()

	if err := store.AddBalance(ctx, dbTx, customerID, amount); err != nil {
		restore(ctx, purchaseID, originalStatus)
		return err
	}

	if err := store.LogTopUp(ctx, dbTx, purchaseID, customerID, amount); err != nil {
		restore(ctx, purchaseID, originalStatus)
		return err
	}

	if err := dbTx.Commit(ctx); err != nil {
		restore(ctx, purchaseID, originalStatus)
		return fmt.Errorf("failed to commit wallet top-up: %w", err)
	}

	return nil
}

// SyncKeys fetches fresh key stats from Remnawave and syncs local DB.
// Results are cached for syncCacheTTL to avoid hammering the external API
// on every GET /api/me request.
func (s *PaymentService) SyncKeys(ctx context.Context, customerID int64, telegramID int64) ([]KeyStats, error) {
	// Return cached result if still fresh
	if v, ok := s.syncCache.Load(customerID); ok {
		entry := v.(syncCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.keys, nil
		}
	}

	if s.subKeyRepo == nil {
		return nil, nil
	}

	// 1. Fetch fresh keys from Remnawave
	users, err := s.remnawaveClient.GetUsersByTelegramId(ctx, telegramID)
	if err != nil {
		slog.Error("Failed to fetch users from Remnawave", "error", err)
		// Fallback: return nothing or let handler use local DB
		return nil, err
	}

	// 2. Fetch local keys
	localKeys, err := s.subKeyRepo.FindByCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}

	// Create maps for lookup/reconciliation.
	remoteMap := make(map[string]remapi.User)
	for _, u := range users {
		remoteMap[u.UUID.String()] = u
	}
	localMap := make(map[string]database.SubscriptionKey, len(localKeys))
	for _, localKey := range localKeys {
		localMap[localKey.RemnawaveUUID.String()] = localKey
	}

	var result []KeyStats

	for _, localKey := range localKeys {
		remoteUser, exists := remoteMap[localKey.RemnawaveUUID.String()]

		// If key exists locally but not remotely -> DELETED
		if !exists {
			if localKey.Status != "deleted" {
				_ = s.subKeyRepo.UpdateStatus(ctx, localKey.ID, "deleted")
			}
			continue
		}

		// Key matches. Update local fields if changed.
		isExpired := remoteUser.ExpireAt.Before(time.Now())
		newStatus := "active"
		if isExpired {
			newStatus = "expired"
		}

		// Sync Expiry
		if localKey.ExpireAt == nil || !localKey.ExpireAt.Equal(remoteUser.ExpireAt) || localKey.Status != newStatus {
			_ = s.subKeyRepo.UpdateExpiry(ctx, localKey.ID, remoteUser.ExpireAt)
			// UpdateExpiry sets status to 'active', so we need to set it correctly if expired
			if newStatus != "active" {
				_ = s.subKeyRepo.UpdateStatus(ctx, localKey.ID, newStatus)
			}
		}

		// Build stats object
		limit := 0
		if remoteUser.TrafficLimitBytes.IsSet() {
			limit = remoteUser.TrafficLimitBytes.Value
		}

		stats := KeyStats{
			ID:                localKey.ID,
			TrafficUsedBytes:  remoteUser.UserTraffic.UsedTrafficBytes,
			TrafficLimitBytes: limit,
			ExpireAt:          remoteUser.ExpireAt,
			Status:            newStatus,
		}
		result = append(result, stats)
	}

	// Reconcile remote-only users into local DB so downstream features
	// (key listing, auto-renew, notifications) do not lose track of paid keys.
	for _, remoteUser := range users {
		if _, exists := localMap[remoteUser.UUID.String()]; exists {
			continue
		}

		newStatus := "active"
		if remoteUser.ExpireAt.Before(time.Now()) {
			newStatus = "expired"
		}

		limitBytes := 0
		if remoteUser.TrafficLimitBytes.IsSet() {
			limitBytes = remoteUser.TrafficLimitBytes.Value
		}
		trafficLimitGB := 0
		if limitBytes > 0 {
			trafficLimitGB = int(math.Ceil(float64(limitBytes) / 1073741824.0))
		}

		expireAt := remoteUser.ExpireAt
		label := remoteUser.Username
		if label == "" {
			label = fmt.Sprintf("key_%s", remoteUser.UUID.String()[:8])
		}

		createdID, createErr := s.subKeyRepo.Create(ctx, &database.SubscriptionKey{
			CustomerID:      customerID,
			RemnawaveUUID:   remoteUser.UUID,
			Username:        remoteUser.Username,
			SubscriptionURL: remoteUser.SubscriptionUrl,
			ExpireAt:        &expireAt,
			Status:          newStatus,
			Label:           label,
			TrafficLimitGB:  trafficLimitGB,
		})
		if createErr != nil {
			// Another worker may have inserted it concurrently.
			existing, findErr := s.subKeyRepo.FindByRemnawaveUUID(ctx, remoteUser.UUID)
			if findErr != nil || existing == nil {
				slog.Error("Failed to reconcile remote-only subscription key",
					"customer_id", customerID,
					"uuid", remoteUser.UUID.String(),
					"create_error", createErr,
					"find_error", findErr,
				)
				continue
			}
			createdID = existing.ID
		}

		result = append(result, KeyStats{
			ID:                createdID,
			TrafficUsedBytes:  remoteUser.UserTraffic.UsedTrafficBytes,
			TrafficLimitBytes: limitBytes,
			ExpireAt:          remoteUser.ExpireAt,
			Status:            newStatus,
		})
	}

	// Cache the result in the sync cache
	s.syncCache.Store(customerID, syncCacheEntry{
		keys:      result,
		expiresAt: time.Now().Add(syncCacheTTL),
	})

	return result, nil
}

// GetCachedSyncKeys returns fresh cached key stats when available.
func (s *PaymentService) GetCachedSyncKeys(customerID int64) ([]KeyStats, bool) {
	v, ok := s.syncCache.Load(customerID)
	if !ok {
		return nil, false
	}

	entry, ok := v.(syncCacheEntry)
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}

	copied := make([]KeyStats, len(entry.keys))
	copy(copied, entry.keys)
	return copied, true
}

// TriggerSyncKeysAsync refreshes key stats in the background (deduplicated per customer).
func (s *PaymentService) TriggerSyncKeysAsync(ctx context.Context, customerID int64, telegramID int64) {
	if _, loaded := s.syncInFlight.LoadOrStore(customerID, struct{}{}); loaded {
		return
	}

	go func() {
		defer s.syncInFlight.Delete(customerID)

		syncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if _, err := s.SyncKeys(syncCtx, customerID, telegramID); err != nil {
			slog.Warn("Background key sync failed", "customer_id", customerID, "error", err)
		}
	}()
}

type KeyStats struct {
	ID                int64
	TrafficUsedBytes  float64
	TrafficLimitBytes int
	ExpireAt          time.Time
	Status            string
}

func (s *PaymentService) restorePurchaseState(ctx context.Context, purchaseID int64, status database.PurchaseStatus) {
	if status == database.PurchaseStatusPaid {
		return
	}

	restoreCtx := context.WithoutCancel(ctx)
	if err := s.purchaseRepository.UpdateFields(restoreCtx, purchaseID, map[string]interface{}{
		"status":  status,
		"paid_at": nil,
	}); err != nil {
		slog.Error("Failed to restore purchase state after error", "purchase_id", purchaseID, "status", status, "error", err)
	}
}

func (s *PaymentService) refundWalletCharge(ctx context.Context, customerID int64, amount float64, purchaseID int64, description string) error {
	refundCtx := context.WithoutCancel(ctx)

	dbTx, err := s.customerRepository.BeginTx(refundCtx)
	if err != nil {
		return fmt.Errorf("failed to begin wallet refund transaction: %w", err)
	}
	defer func() {
		_ = dbTx.Rollback(refundCtx)
	}()

	if err := s.customerRepository.AddBalanceTx(refundCtx, dbTx, customerID, amount); err != nil {
		return fmt.Errorf("failed to refund wallet balance: %w", err)
	}

	if s.walletTxRepo != nil {
		if _, err := s.walletTxRepo.CreateTx(refundCtx, dbTx, &database.WalletTransaction{
			CustomerID:  customerID,
			Amount:      amount,
			Type:        database.WalletTransactionTypeRefund,
			PurchaseID:  &purchaseID,
			Description: description,
		}); err != nil {
			return fmt.Errorf("failed to log wallet refund: %w", err)
		}
	}

	if err := dbTx.Commit(refundCtx); err != nil {
		return fmt.Errorf("failed to commit wallet refund: %w", err)
	}

	return nil
}

func (s *PaymentService) releasePromoUsage(ctx context.Context, promoID *int64, reason string) {
	if promoID == nil || s.promoCodeRepository == nil {
		return
	}

	releaseCtx := context.WithoutCancel(ctx)
	released, err := s.promoCodeRepository.ReleaseUsageAtomic(releaseCtx, *promoID)
	if err != nil {
		slog.Error("Failed to release promo usage", "promo_id", *promoID, "reason", reason, "error", err)
		return
	}
	if released {
		slog.Info("Released promo usage after failure", "promo_id", *promoID, "reason", reason)
	}
}

func (s *PaymentService) applyPromoDiscount(ctx context.Context, amount float64, promoCode string, claimUsage bool) (float64, *int64, bool) {
	if promoCode == "" || s.promoCodeRepository == nil {
		return amount, nil, false
	}

	promo, err := s.promoCodeRepository.FindByCode(ctx, promoCode)
	if err != nil {
		slog.Error("Failed to look up promo code", "error", err, "code", promoCode)
		return amount, nil, false
	}
	if promo == nil {
		return amount, nil, false
	}

	if promo.UsedCount >= promo.MaxUses || time.Now().After(promo.ValidUntil) {
		return amount, nil, false
	}

	if claimUsage {
		claimed, err := s.promoCodeRepository.IncrementUsageAtomic(ctx, promo.ID)
		if err != nil {
			slog.Error("Failed to claim promo slot", "error", err, "code", promoCode)
			return amount, nil, false
		}
		if !claimed {
			return amount, nil, false
		}
	}

	discount := float64(promo.DiscountPercent) / 100.0
	amount = amount * (1 - discount)
	amount = math.Round(amount)
	return amount, &promo.ID, claimUsage
}

func (s *PaymentService) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer %s not found", utils.MaskHalfInt64(purchase.CustomerID))
	}

	// Idempotency / Race Condition Check
	if purchase.Status == database.PurchaseStatusPaid {
		slog.Info("Purchase already paid, skipping processing", "purchase_id", purchaseId)
		return nil
	}

	originalStatus := purchase.Status
	claimed, err := s.purchaseRepository.TryMarkAsPaid(ctx, purchase.ID)
	if err != nil {
		return err
	}
	if !claimed {
		slog.Info("Purchase claim lost to another worker, skipping", "purchase_id", purchaseId)
		return nil
	}

	if messageId, b := s.cache.Get(purchase.ID); b {
		_, err = s.telegramBot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    customer.TelegramID,
			MessageID: messageId,
		})
		if err != nil {
			slog.Error("Error deleting message", "error", err)
		}
	}

	const bytesInGB = 1073741824

	if purchase.InvoiceType == database.InvoiceTypeWalletTopUp {
		// WALLET TOP-UP — balance credit and transaction log must be atomic.
		if err := settleWalletTopUp(
			ctx,
			repositoryWalletTopUpStore{customerRepo: s.customerRepository, walletTxRepo: s.walletTxRepo},
			purchase.ID,
			customer.ID,
			purchase.Amount,
			originalStatus,
			s.restorePurchaseState,
		); err != nil {
			slog.Error("CRITICAL: Failed to add balance for wallet top-up", "purchase_id", purchaseId, "error", err)
			return err
		}
		slog.Info("Wallet top-up successful", "purchase_id", purchaseId, "amount", purchase.Amount, "customer_id", customer.ID)
	} else if purchase.ExtendKeyID != nil {
		// EXTEND existing key
		existingKey, err := s.subKeyRepo.FindByID(ctx, *purchase.ExtendKeyID)
		if err != nil || existingKey == nil {
			s.restorePurchaseState(ctx, purchase.ID, originalStatus)
			return fmt.Errorf("subscription key %d not found", *purchase.ExtendKeyID)
		}
		// Build user description
		userDesc := buildUserDescription(purchase.PaymentMethod, purchase.PlanLabel, purchase.Days, purchase.TrafficLimitGB, customer.TelegramID, purchase.TransactionID)
		ctx = context.WithValue(ctx, "description", userDesc)
		// Extend the specific Remnawave user by UUID (adds days and traffic)
		remnawaveUser, err := s.remnawaveClient.ExtendUser(ctx, existingKey.RemnawaveUUID, purchase.TrafficLimitGB*bytesInGB, purchase.Days)
		if err != nil {
			s.restorePurchaseState(ctx, purchase.ID, originalStatus)
			return err
		}
		// Update the subscription_key expiry
		if s.subKeyRepo != nil {
			if err := s.subKeyRepo.UpdateExpiry(ctx, existingKey.ID, remnawaveUser.ExpireAt); err != nil {
				slog.Error("Failed to update key expiry (non-fatal)", "key_id", existingKey.ID, "error", err)
			}
			if err := s.subKeyRepo.UpdateSubscriptionURL(ctx, existingKey.ID, remnawaveUser.SubscriptionUrl); err != nil {
				slog.Error("Failed to update subscription URL (non-fatal)", "key_id", existingKey.ID, "error", err)
			}
		}
		// Update customer expire_at to the latest
		customerFilesToUpdate := map[string]interface{}{
			"subscription_link": remnawaveUser.SubscriptionUrl,
			"expire_at":         remnawaveUser.ExpireAt,
		}
		if err := s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate); err != nil {
			slog.Error("Failed to update customer fields after extend (non-fatal)", "error", err)
		}
	} else {
		// CREATE new key — always creates a fresh Remnawave user
		usernameSeed := purchase.TransactionID
		if usernameSeed != "" {
			usernameSeed = fmt.Sprintf("%s_%d", usernameSeed, purchase.ID)
		} else {
			usernameSeed = fmt.Sprintf("PURCHASE_%d", purchase.ID)
		}

		// Build user description
		userDesc := buildUserDescription(purchase.PaymentMethod, purchase.PlanLabel, purchase.Days, purchase.TrafficLimitGB, customer.TelegramID, purchase.TransactionID)
		ctx = context.WithValue(ctx, "description", userDesc)

		remnawaveUser, err := s.remnawaveClient.ForceCreateNewUser(ctx, customer.ID, customer.TelegramID, purchase.TrafficLimitGB*bytesInGB, purchase.Days, 0, usernameSeed)
		if err != nil {
			s.restorePurchaseState(ctx, purchase.ID, originalStatus)
			return err
		}
		// Insert into subscription_key table
		if s.subKeyRepo != nil {
			txnSuffix := ""
			if len(purchase.TransactionID) >= 4 {
				txnSuffix = purchase.TransactionID[len(purchase.TransactionID)-4:]
			} else if len(purchase.TransactionID) > 0 {
				txnSuffix = purchase.TransactionID
			} else {
				txnSuffix = fmt.Sprintf("%04d", purchase.ID%10000)
			}
			label := fmt.Sprintf("wavy_%s_%d", txnSuffix, customer.TelegramID)
			_, err := s.subKeyRepo.Create(ctx, &database.SubscriptionKey{
				CustomerID:      customer.ID,
				RemnawaveUUID:   remnawaveUser.UUID,
				Username:        remnawaveUser.Username,
				SubscriptionURL: remnawaveUser.SubscriptionUrl,
				ExpireAt:        &remnawaveUser.ExpireAt,
				Status:          "active",
				Label:           label,
				TrafficLimitGB:  purchase.TrafficLimitGB,
			})
			if err != nil {
				slog.Error("CRITICAL: Failed to save subscription key to DB. Key EXISTS on Remnawave but NOT in local DB.",
					"purchase_id", purchaseId, "username", remnawaveUser.Username, "error", err)
				// Continue — key exists remotely. SyncKeys now reconciles missing
				// local rows from Remnawave and will recover this mismatch.
			}
		}
		// Update customer
		customerFilesToUpdate := map[string]interface{}{
			"subscription_link": remnawaveUser.SubscriptionUrl,
			"expire_at":         remnawaveUser.ExpireAt,
		}
		if err := s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate); err != nil {
			slog.Error("Failed to update customer fields after create (non-fatal)", "error", err)
		}
	}

	if purchase.PromoCodeID != nil &&
		purchase.InvoiceType != database.InvoiceTypeWalletPayment &&
		purchase.InvoiceType != database.InvoiceTypeWalletTopUp &&
		s.promoCodeRepository != nil {
		promoCtx := context.WithoutCancel(ctx)
		if err := s.promoCodeRepository.IncrementUsage(promoCtx, *purchase.PromoCodeID); err != nil {
			slog.Error("Failed to finalize promo usage after successful fulfillment", "purchase_id", purchaseId, "promo_id", *purchase.PromoCodeID, "error", err)
		}
	}

	slog.Info("Purchase processed successfully", "purchase_id", utils.MaskHalfInt64(purchase.ID), "type", purchase.InvoiceType, "customer_id", utils.MaskHalfInt64(customer.ID))

	// === BELOW THIS LINE: NON-FATAL OPERATIONS ===
	// Telegram notification and referral bonus are best-effort.
	// Failures here should NOT prevent the user from getting their key.

	// Refresh customer
	customer, err = s.customerRepository.FindById(ctx, customer.ID)
	if err != nil {
		slog.Error("Error refreshing customer after purchase (non-fatal)", "error", err)
	}

	if _, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID,
		Text:   s.translation.GetText(customer.Language, "subscription_activated"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(customer),
		},
	}); err != nil {
		slog.Error("Failed to send activation message (non-fatal)", "error", err, "purchase_id", purchaseId)
	}

	// Referral bonus — completely non-fatal
	s.processReferralBonus(ctx, customer)

	return nil
}

// ReferralBonusAmount is the wallet credit (in MMK) granted to each party when a referral converts.
// This is now a variable that can be updated via the admin /setreferralbonus command,
// and it persists in the app_config database table.
var ReferralBonusAmount float64 = 1000.0

// processReferralBonus grants a 1,000 MMK wallet bonus to both the referrer and
// the referee (new buyer) when the referee completes their first purchase.
// This is intentionally non-fatal — errors are logged but never block the purchase flow.
func (s *PaymentService) processReferralBonus(ctx context.Context, customer *database.Customer) {
	// context.WithoutCancel inherits values (tracing, etc.) from the parent context
	// but is not cancelled when the parent is cancelled, making this operation
	// safe to run after the purchase flow completes.
	ctxRef := context.WithoutCancel(ctx)

	slog.Info("[REFERRAL-DEBUG] processReferralBonus called",
		"customer_id", customer.ID,
		"telegram_id", customer.TelegramID,
	)

	referral, err := s.referralRepository.FindByReferee(ctxRef, customer.TelegramID)
	if err != nil {
		slog.Error("[REFERRAL-DEBUG] Referral lookup failed (non-fatal)", "error", err)
		return
	}
	if referral == nil {
		slog.Info("[REFERRAL-DEBUG] No referral record found for this customer — not referred by anyone")
		return // customer was not referred by anyone
	}

	slog.Info("[REFERRAL-DEBUG] Referral record found!",
		"referral_id", referral.ID,
		"referrer_telegram_id", referral.ReferrerID,
		"referee_telegram_id", referral.RefereeID,
		"bonus_granted", referral.BonusGranted,
		"referee_bonus_granted", referral.RefereeBonusGranted,
	)

	// --- Credit REFERRER (person who shared the link) ---
	if !referral.BonusGranted {
		slog.Info("[REFERRAL-DEBUG] Crediting REFERRER...")
		referrerCustomer, err := s.customerRepository.FindByTelegramId(ctxRef, referral.ReferrerID)
		if err != nil || referrerCustomer == nil {
			slog.Error("[REFERRAL-DEBUG] FAILED: referrer customer lookup failed (non-fatal)", "error", err, "referrer_id", referral.ReferrerID)
			return
		}

		slog.Info("[REFERRAL-DEBUG] Referrer found, adding balance",
			"referrer_customer_id", referrerCustomer.ID,
			"current_balance", referrerCustomer.Balance,
			"bonus_amount", ReferralBonusAmount,
		)

		dbTx, err := s.customerRepository.BeginTx(ctxRef)
		if err != nil {
			slog.Error("[REFERRAL-DEBUG] FAILED: failed to begin referrer bonus transaction (non-fatal)", "error", err)
			return
		}
		referrerTxDone := false
		defer func() {
			if !referrerTxDone {
				_ = dbTx.Rollback(ctxRef)
			}
		}()

		claimed, err := s.referralRepository.TryMarkBonusGrantedTx(ctxRef, dbTx, referral.ID)
		if err != nil {
			slog.Error("[REFERRAL-DEBUG] FAILED: failed to claim referrer bonus (non-fatal)", "error", err)
			return
		}
		if !claimed {
			_ = dbTx.Rollback(ctxRef)
			referrerTxDone = true
			slog.Info("[REFERRAL-DEBUG] Referrer bonus already claimed by another worker, skipping")
		} else {
			if err := s.customerRepository.AddBalanceTx(ctxRef, dbTx, referrerCustomer.ID, ReferralBonusAmount); err != nil {
				slog.Error("[REFERRAL-DEBUG] FAILED: failed to credit referrer balance (non-fatal)", "error", err)
				return
			}
			slog.Info("[REFERRAL-DEBUG] Referrer balance credited successfully")

			if s.walletTxRepo != nil {
				if _, err := s.walletTxRepo.CreateTx(ctxRef, dbTx, &database.WalletTransaction{
					CustomerID:  referrerCustomer.ID,
					Amount:      ReferralBonusAmount,
					Type:        database.WalletTransactionTypeReferral,
					Description: "Referral bonus — friend made their first purchase",
				}); err != nil {
					slog.Error("[REFERRAL-DEBUG] FAILED: failed to log referrer wallet transaction (non-fatal)", "error", err)
					return
				}
				slog.Info("[REFERRAL-DEBUG] Referrer wallet transaction logged")
			}

			if err := dbTx.Commit(ctxRef); err != nil {
				slog.Error("[REFERRAL-DEBUG] FAILED: failed to commit referrer bonus transaction (non-fatal)", "error", err)
				return
			}
			referrerTxDone = true

			slog.Info("[REFERRAL-DEBUG] SUCCESS: Granted referral bonus to referrer", "referrer_id", referrerCustomer.ID, "amount", ReferralBonusAmount)

			if _, err := s.telegramBot.SendMessage(ctxRef, &bot.SendMessageParams{
				ChatID:    referrerCustomer.TelegramID,
				ParseMode: models.ParseModeHTML,
				Text:      s.translation.GetText(referrerCustomer.Language, "referral_bonus_granted"),
			}); err != nil {
				slog.Error("[REFERRAL-DEBUG] Referrer notification failed (non-fatal)", "error", err)
			}
		}
	} else {
		slog.Info("[REFERRAL-DEBUG] Referrer bonus already granted, skipping")
	}

	// --- Credit REFEREE (the new buyer who clicked the link) ---
	if !referral.RefereeBonusGranted {
		slog.Info("[REFERRAL-DEBUG] Crediting REFEREE...")

		dbTx, err := s.customerRepository.BeginTx(ctxRef)
		if err != nil {
			slog.Error("[REFERRAL-DEBUG] FAILED: failed to begin referee bonus transaction (non-fatal)", "error", err)
			return
		}
		refereeTxDone := false
		defer func() {
			if !refereeTxDone {
				_ = dbTx.Rollback(ctxRef)
			}
		}()

		claimed, err := s.referralRepository.TryMarkRefereeBonusGrantedTx(ctxRef, dbTx, referral.ID)
		if err != nil {
			slog.Error("[REFERRAL-DEBUG] FAILED: failed to claim referee bonus (non-fatal)", "error", err)
			return
		}
		if !claimed {
			_ = dbTx.Rollback(ctxRef)
			refereeTxDone = true
			slog.Info("[REFERRAL-DEBUG] Referee bonus already claimed by another worker, skipping")
		} else {
			if err := s.customerRepository.AddBalanceTx(ctxRef, dbTx, customer.ID, ReferralBonusAmount); err != nil {
				slog.Error("[REFERRAL-DEBUG] FAILED: failed to credit referee balance (non-fatal)", "error", err)
				return
			}
			slog.Info("[REFERRAL-DEBUG] Referee balance credited successfully")

			if s.walletTxRepo != nil {
				if _, err := s.walletTxRepo.CreateTx(ctxRef, dbTx, &database.WalletTransaction{
					CustomerID:  customer.ID,
					Amount:      ReferralBonusAmount,
					Type:        database.WalletTransactionTypeReferral,
					Description: "Welcome bonus — joined via referral link",
				}); err != nil {
					slog.Error("[REFERRAL-DEBUG] FAILED: failed to log referee wallet transaction (non-fatal)", "error", err)
					return
				}
				slog.Info("[REFERRAL-DEBUG] Referee wallet transaction logged")
			}

			if err := dbTx.Commit(ctxRef); err != nil {
				slog.Error("[REFERRAL-DEBUG] FAILED: failed to commit referee bonus transaction (non-fatal)", "error", err)
				return
			}
			refereeTxDone = true

			slog.Info("[REFERRAL-DEBUG] SUCCESS: Granted welcome bonus to referee", "referee_id", customer.ID, "amount", ReferralBonusAmount)

			if _, err := s.telegramBot.SendMessage(ctxRef, &bot.SendMessageParams{
				ChatID:    customer.TelegramID,
				ParseMode: models.ParseModeHTML,
				Text:      s.translation.GetText(customer.Language, "referee_bonus_granted"),
			}); err != nil {
				slog.Error("[REFERRAL-DEBUG] Referee notification failed (non-fatal)", "error", err)
			}
		}
	} else {
		slog.Info("[REFERRAL-DEBUG] Referee bonus already granted, skipping")
	}

	slog.Info("[REFERRAL-DEBUG] processReferralBonus completed")
}

func (s *PaymentService) createConnectKeyboard(customer *database.Customer) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton

	if config.GetMiniAppURL() != "" {
		inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
			{Text: s.translation.GetText(customer.Language, "connect_button"), WebApp: &models.WebAppInfo{
				URL: config.GetMiniAppURL(),
			}},
		})
	} else {
		if customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
			inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
				{Text: s.translation.GetText(customer.Language, "connect_button"), WebApp: &models.WebAppInfo{
					URL: *customer.SubscriptionLink,
				}},
			})
			inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
				{Text: s.translation.GetText(customer.Language, "happ_proxy_button"), URL: *customer.SubscriptionLink},
			})
		} else {
			inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
				{Text: s.translation.GetText(customer.Language, "connect_button"), CallbackData: "connect"},
			})
		}
	}

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		{Text: s.translation.GetText(customer.Language, "back_button"), CallbackData: "start"},
	})
	return inlineCustomerKeyboard
}

func (s *PaymentService) CreatePurchase(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, invoiceType database.InvoiceType, promoCode string) (url string, purchaseId int64, err error) {
	var promoID *int64
	var promoClaimed bool
	if invoiceType != database.InvoiceTypeWalletTopUp {
		amount, promoID, promoClaimed = s.applyPromoDiscount(ctx, amount, promoCode, invoiceType == database.InvoiceTypeWalletPayment)
	}
	defer func() {
		if err != nil && promoClaimed {
			s.releasePromoUsage(ctx, promoID, "purchase creation failed")
		}
	}()

	if amount <= 0 {
		if promoID != nil && !promoClaimed && s.promoCodeRepository != nil {
			claimed, claimErr := s.promoCodeRepository.IncrementUsageAtomic(ctx, *promoID)
			if claimErr != nil {
				return "", 0, fmt.Errorf("failed to claim promo usage for free purchase: %w", claimErr)
			}
			if !claimed {
				return "", 0, fmt.Errorf("promo code is no longer available")
			}
			promoClaimed = true
		}
		return s.createFreePurchase(ctx, days, trafficLimitGB, customer, promoID)
	}

	switch invoiceType {
	case database.InvoiceTypeCrypto:
		return s.createCryptoInvoice(ctx, amount, days, trafficLimitGB, customer, promoID)
	case database.InvoiceTypeMobileBanking:
		return s.createMobileBankingPurchase(ctx, amount, days, trafficLimitGB, customer, promoID)
	case database.InvoiceTypeWalletTopUp:
		return s.createWalletTopUpInvoice(ctx, amount, customer, promoID)
	case database.InvoiceTypeWalletPayment:
		return s.createWalletPurchase(ctx, amount, days, trafficLimitGB, customer, promoID)
	default:
		s.releasePromoUsage(ctx, promoID, "unknown invoice type")
		return "", 0, fmt.Errorf("unknown invoice type: %s", invoiceType)
	}
}

func (s *PaymentService) createFreePurchase(ctx context.Context, days int, trafficLimitGB int, customer *database.Customer, promoID *int64) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeWalletPayment, // Treat free purchases like wallet payments
		Status:         database.PurchaseStatusNew,
		Amount:         0,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Month:          0,
		Days:           days,
		TrafficLimitGB: trafficLimitGB,
		PromoCodeID:    promoID,
	})
	if err != nil {
		slog.Error("Error creating free purchase", "error", err)
		return "", 0, err
	}

	if err := s.ProcessPurchaseById(ctx, purchaseId); err != nil {
		slog.Error("Error processing free purchase", "error", err, "purchase_id", purchaseId)
		return "", 0, err
	}

	slog.Info("Free purchase processed successfully", "purchase_id", utils.MaskHalfInt64(purchaseId), "customer_id", utils.MaskHalfInt64(customer.ID), "promo_id", promoID)
	return "", purchaseId, nil
}

func (s *PaymentService) createCryptoInvoice(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, promoID *int64) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeCrypto,
		Status:         database.PurchaseStatusNew,
		Amount:         amount,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Month:          0,
		Days:           days,
		TrafficLimitGB: trafficLimitGB,
		PromoCodeID:    promoID,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	invoice, err := s.cryptoPayClient.CreateInvoice(ctx, &cryptopay.InvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           config.Currency(),
		Amount:         fmt.Sprintf("%d", int(amount)),
		AcceptedAssets: "USDT",
		Payload:        fmt.Sprintf("purchaseId=%d&username=%s", purchaseId, ctx.Value(UsernameCtxKey)),
		Description:    fmt.Sprintf("Subscription for %d days", days),
		PaidBtnName:    "callback",
		PaidBtnUrl:     config.BotURL(),
	})
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		_ = s.purchaseRepository.UpdateFields(context.WithoutCancel(ctx), purchaseId, map[string]interface{}{
			"status": database.PurchaseStatusCancel,
		})
		return "", 0, err
	}

	updates := map[string]interface{}{
		"crypto_invoice_url": invoice.BotInvoiceUrl,
		"crypto_invoice_id":  invoice.InvoiceID,
		"status":             database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		_ = s.purchaseRepository.UpdateFields(context.WithoutCancel(ctx), purchaseId, map[string]interface{}{
			"status": database.PurchaseStatusCancel,
		})
		return "", 0, err
	}

	return invoice.BotInvoiceUrl, purchaseId, nil
}

func (s *PaymentService) ActivateTrial(ctx context.Context, telegramId int64) (string, error) {
	if config.TrialDays() == 0 {
		return "", nil
	}
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return "", err
	}
	if customer == nil {
		return "", fmt.Errorf("customer %d not found", telegramId)
	}
	user, err := s.remnawaveClient.CreateOrUpdateUser(ctx, customer.ID, telegramId, config.TrialTrafficLimit(), config.TrialDays(), true)
	if err != nil {
		slog.Error("Error creating user", "error", err)
		return "", err
	}

	customerFilesToUpdate := map[string]interface{}{
		"subscription_link": user.GetSubscriptionUrl(),
		"expire_at":         user.GetExpireAt(),
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return "", err
	}

	return user.GetSubscriptionUrl(), nil

}

func (s *PaymentService) createMobileBankingPurchase(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, promoID *int64) (url string, purchaseId int64, err error) {
	if len(GetEnabledPaymentProviders()) == 0 {
		return "", 0, fmt.Errorf("no mobile banking accounts configured")
	}

	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeMobileBanking,
		Status:         database.PurchaseStatusPending,
		Amount:         amount,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Month:          0,
		Days:           days,
		TrafficLimitGB: trafficLimitGB,
		PromoCodeID:    promoID,
	})
	if err != nil {
		slog.Error("Error creating mobile banking purchase", "error", err)
		return "", 0, err
	}

	slog.Info("Mobile banking purchase created", "purchase_id", utils.MaskHalfInt64(purchaseId), "customer_id", utils.MaskHalfInt64(customer.ID))
	// No external URL needed — user sends screenshot directly
	return "", purchaseId, nil
}

func (s *PaymentService) createWalletPurchase(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, promoID *int64) (url string, purchaseId int64, err error) {
	// Check if customer has sufficient balance logic is handled by DeductBalance (atomic)
	// But we check first to fail fast before creating purchase
	if customer.Balance < amount {
		slog.Error("Insufficient wallet balance before purchase creation", "customer_id", utils.MaskHalfInt64(customer.ID), "amount", amount)
		return "", 0, fmt.Errorf("insufficient wallet balance")
	}

	// Create purchase record
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeWalletPayment,
		Status:         database.PurchaseStatusNew,
		Amount:         amount,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Month:          0,
		Days:           days,
		TrafficLimitGB: trafficLimitGB,
		PromoCodeID:    promoID,
	})
	if err != nil {
		slog.Error("Error creating wallet purchase", "error", err)
		return "", 0, err
	}

	// Deduct balance immediately
	if err := s.customerRepository.DeductBalance(ctx, customer.ID, amount); err != nil {
		slog.Error("Error deducting wallet balance", "error", err, "purchase_id", purchaseId)
		// Mark purchase as cancelled
		s.purchaseRepository.UpdateFields(ctx, purchaseId, map[string]interface{}{"status": database.PurchaseStatusCancel})
		return "", 0, fmt.Errorf("failed to deduct balance: %w", err)
	}

	// Log transaction if repo exists
	if s.walletTxRepo != nil {
		_, err := s.walletTxRepo.Create(ctx, &database.WalletTransaction{
			CustomerID:  customer.ID,
			Amount:      -amount,
			Type:        database.WalletTransactionTypePurchase,
			PurchaseID:  &purchaseId,
			Description: fmt.Sprintf("Purchase plan %d days", days),
		})
		if err != nil {
			slog.Error("Failed to log wallet transaction (non-fatal)", "error", err)
		}
	}

	// Process the purchase immediately (no waiting for payment confirmation)
	if err := s.ProcessPurchaseById(ctx, purchaseId); err != nil {
		slog.Error("Error processing wallet purchase", "error", err, "purchase_id", purchaseId)
		if refundErr := s.refundWalletCharge(ctx, customer.ID, amount, purchaseId, fmt.Sprintf("Refund wallet purchase #%d", purchaseId)); refundErr != nil {
			slog.Error("Failed to refund wallet charge after processing error", "error", refundErr, "purchase_id", purchaseId)
			return "", 0, fmt.Errorf("failed to process wallet purchase: %w (refund failed: %v)", err, refundErr)
		}
		return "", 0, err
	}

	slog.Info("Wallet purchase completed", "purchase_id", utils.MaskHalfInt64(purchaseId), "customer_id", utils.MaskHalfInt64(customer.ID))
	return "", purchaseId, nil
}

// CreatePurchaseWithExtend is like createWalletPurchase but sets ExtendKeyID so
// ProcessPurchaseById will call Remnawave's ExtendUser on the specific key UUID
// rather than creating a brand-new user. Used by the per-key auto-renew cron.
func (s *PaymentService) CreatePurchaseWithExtend(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, keyID int64, promoCode string) (url string, purchaseId int64, err error) {
	amount, promoID, promoClaimed := s.applyPromoDiscount(ctx, amount, promoCode, true)
	defer func() {
		if err != nil && promoClaimed {
			s.releasePromoUsage(ctx, promoID, "extend-key wallet purchase failed")
		}
	}()

	if customer.Balance < amount {
		slog.Error("Insufficient wallet balance for extend-key purchase", "customer_id", utils.MaskHalfInt64(customer.ID), "amount", amount, "key_id", keyID)
		return "", 0, fmt.Errorf("insufficient wallet balance")
	}

	// Create the purchase record with ExtendKeyID set.
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeWalletPayment,
		Status:         database.PurchaseStatusNew,
		Amount:         amount,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Days:           days,
		TrafficLimitGB: trafficLimitGB,
		ExtendKeyID:    &keyID,
		PromoCodeID:    promoID,
	})
	if err != nil {
		slog.Error("Error creating extend-key wallet purchase", "error", err)
		return "", 0, err
	}

	// Deduct balance atomically.
	if err := s.customerRepository.DeductBalance(ctx, customer.ID, amount); err != nil {
		slog.Error("Error deducting wallet balance for extend-key", "error", err, "purchase_id", purchaseId)
		s.purchaseRepository.UpdateFields(ctx, purchaseId, map[string]interface{}{"status": database.PurchaseStatusCancel})
		return "", 0, fmt.Errorf("failed to deduct balance: %w", err)
	}

	// Log the transaction.
	if s.walletTxRepo != nil {
		if _, err := s.walletTxRepo.Create(ctx, &database.WalletTransaction{
			CustomerID:  customer.ID,
			Amount:      -amount,
			Type:        database.WalletTransactionTypePurchase,
			PurchaseID:  &purchaseId,
			Description: fmt.Sprintf("Auto-renew key #%d (%d days)", keyID, days),
		}); err != nil {
			slog.Error("Failed to log extend-key wallet transaction (non-fatal)", "error", err)
		}
	}

	// Process immediately: follows the ExtendKeyID branch in ProcessPurchaseById.
	if err := s.ProcessPurchaseById(ctx, purchaseId); err != nil {
		slog.Error("Error processing extend-key wallet purchase", "error", err, "purchase_id", purchaseId)
		if refundErr := s.refundWalletCharge(ctx, customer.ID, amount, purchaseId, fmt.Sprintf("Refund extend-key purchase #%d", purchaseId)); refundErr != nil {
			slog.Error("Failed to refund wallet charge after extend-key processing error", "error", refundErr, "purchase_id", purchaseId)
			return "", 0, fmt.Errorf("failed to process extend-key wallet purchase: %w (refund failed: %v)", err, refundErr)
		}
		return "", 0, err
	}

	slog.Info("Extend-key wallet purchase completed", "purchase_id", utils.MaskHalfInt64(purchaseId), "key_id", keyID, "customer_id", utils.MaskHalfInt64(customer.ID))
	return "", purchaseId, nil
}

// VerificationResult holds the outcome of a mobile payment screenshot check.
type VerificationResult struct {
	Success   bool
	Reason    string
	ReasonKey string // translation key
}

func (s *PaymentService) VerifyMobilePayment(ctx context.Context, purchaseID int64, imageBytes []byte, mimeType string) (*VerificationResult, error) {
	if s.paymentAnalyzer == nil {
		return &VerificationResult{Success: false, Reason: "Mobile banking not configured", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	enabledProviders := GetEnabledPaymentProviders()
	if len(enabledProviders) == 0 {
		return &VerificationResult{Success: false, Reason: "No receiving accounts configured", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	purchase, err := s.purchaseRepository.FindById(ctx, purchaseID)
	if err != nil {
		return nil, fmt.Errorf("error finding purchase %d: %w", purchaseID, err)
	}
	if purchase == nil {
		return nil, fmt.Errorf("purchase %d not found", purchaseID)
	}

	// Guard: prevent re-verification of already-paid purchases
	if purchase.Status == database.PurchaseStatusPaid {
		return &VerificationResult{Success: false, Reason: "Purchase already completed", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	geminiProviders := make([]gemini.ConfiguredProvider, 0, len(enabledProviders))
	for _, provider := range enabledProviders {
		geminiProviders = append(geminiProviders, gemini.ConfiguredProvider{
			Key:         provider.Key,
			Label:       provider.Label,
			Phone:       provider.Phone,
			AccountName: provider.AccountName,
		})
	}

	info, err := s.paymentAnalyzer.AnalyzePaymentScreenshot(ctx, imageBytes, mimeType, geminiProviders)
	if err != nil {
		logAttrs := []any{
			"purchase_id", purchaseID,
			"error", err,
		}
		if providerErr, ok := gemini.AsProviderError(err); ok {
			logAttrs = append(logAttrs,
				"provider", providerErr.Provider,
				"error_class", providerErr.Class,
				"status_code", providerErr.StatusCode,
			)
		}
		slog.Error("Mobile payment analysis failed", logAttrs...)
		return &VerificationResult{Success: false, Reason: "Could not analyze screenshot", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	slog.Info("Mobile payment analyzer outcome",
		"purchase_id", purchaseID,
		"provider", info.Provider,
		"is_valid", info.IsValid,
		"has_transaction_id", strings.TrimSpace(info.TransactionID) != "",
	)

	if !info.IsValid {
		slog.Warn("Mobile payment screenshot rejected by analyzer",
			"purchase_id", purchaseID,
			"provider", info.Provider,
			"outcome", "semantic_negative",
		)
		return &VerificationResult{Success: false, Reason: "Screenshot does not appear to be a valid payment confirmation", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	// 1. Check transaction ID not empty
	if strings.TrimSpace(info.TransactionID) == "" {
		return &VerificationResult{Success: false, Reason: "No transaction ID found", ReasonKey: "mobile_pay_failed_generic"}, nil
	}

	// === TEST MODE BYPASS ===
	// In Test Mode, we accept:
	// 1. "Magic" Transaction ID (bypasses everything)
	// 2. Any valid-looking receipt (bypasses duplicate/amount checks)
	if s.IsTestMode() {
		isMagic := strings.TrimSpace(info.TransactionID) == testTransactionID
		slog.Info("Test Mode processing", "purchase_id", purchaseID, "is_magic", isMagic)

		// Record verification
		// Append random suffix to transaction ID to avoid duplicate key violations on multiple tests
		storedTxID := fmt.Sprintf("%s_%d", info.TransactionID, time.Now().UnixNano())

		_, err = s.mobilePaymentRepo.Create(ctx, &database.MobilePaymentVerification{
			PurchaseID:    purchaseID,
			TransactionID: storedTxID,
			Provider:      info.Provider,
			PhoneNumber:   info.PhoneNumber,
			Amount:        info.Amount,
			Note:          info.Note,
			Verified:      true,
		})
		if err != nil {
			slog.Error("Error recording mobile payment (test mode)", "error", err)
			return nil, err
		}

		err = s.ProcessPurchaseById(ctx, purchaseID)
		if err != nil {
			slog.Error("Error processing verified mobile purchase (test mode)", "error", err)
			if delErr := s.mobilePaymentRepo.DeleteByTransactionID(context.WithoutCancel(ctx), storedTxID); delErr != nil {
				slog.Error("Error deleting test-mode mobile payment record after failure", "error", delErr)
			}
			return nil, err
		}

		now := time.Now()
		_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{
			"transaction_id": storedTxID,
			"payment_method": info.Provider,
			"payment_phone":  info.PhoneNumber,
			"verified_at":    now,
		})

		return &VerificationResult{Success: true, ReasonKey: "mobile_pay_success"}, nil
	}
	// ========================

	// 2. Check for duplicate transaction ID
	exists, err := s.mobilePaymentRepo.ExistsByTransactionID(ctx, info.TransactionID)
	if err != nil {
		slog.Error("Error checking duplicate txn", "error", err)
		return nil, err
	}
	if exists {
		return &VerificationResult{Success: false, Reason: "Duplicate transaction ID", ReasonKey: "mobile_pay_failed_duplicate"}, nil
	}

	// 3. Check the receipt matches one of the enabled providers.
	// Phone suffix is preferred, but account name can also verify the receiver when
	// different apps use different names/number formats.
	matchedProvider, matchedBy, matched := MatchPaymentRecipient(NormalizeProviderKey(info.Provider), info.PhoneNumber, info.RecipientName, 4)
	if !matched {
		slog.Warn("Recipient mismatch", "provider", info.Provider, "phone", info.PhoneNumber, "recipient_name", info.RecipientName, "purchase_id", purchaseID)
		return &VerificationResult{Success: false, Reason: "Wrong recipient details", ReasonKey: "mobile_pay_failed_phone"}, nil
	}

	// 4. Check note for forbidden keywords
	noteLower := strings.ToLower(info.Note)
	if strings.Contains(noteLower, "vpn") || strings.Contains(noteLower, "outline") {
		slog.Warn("Payment note contains forbidden keyword", "note", info.Note, "purchase_id", purchaseID)
		return &VerificationResult{Success: false, Reason: "Payment note contains forbidden keyword", ReasonKey: "mobile_pay_failed_note"}, nil
	}

	// 5. Check amount matches (exact integer match)
	expectedAmount := purchase.Amount
	if math.Abs(info.Amount-expectedAmount) > 0.5 {
		slog.Warn("Amount mismatch", "expected", expectedAmount, "got", info.Amount, "purchase_id", purchaseID)
		return &VerificationResult{
			Success:   false,
			Reason:    fmt.Sprintf("Amount mismatch: expected %.0f, got %.0f", expectedAmount, info.Amount),
			ReasonKey: "mobile_pay_failed_amount",
		}, nil
	}

	// All checks passed — record verification and process
	_, err = s.mobilePaymentRepo.Create(ctx, &database.MobilePaymentVerification{
		PurchaseID:    purchaseID,
		TransactionID: info.TransactionID,
		Provider:      matchedProvider.Key,
		PhoneNumber:   info.PhoneNumber,
		Amount:        info.Amount,
		Note:          info.Note,
		Verified:      true,
	})
	if err != nil {
		slog.Error("Error recording mobile payment", "error", err)
		return nil, err
	}

	err = s.ProcessPurchaseById(ctx, purchaseID)
	if err != nil {
		slog.Error("Error processing verified mobile purchase", "error", err)
		if delErr := s.mobilePaymentRepo.DeleteByTransactionID(context.WithoutCancel(ctx), info.TransactionID); delErr != nil {
			slog.Error("Error deleting mobile payment record after failure", "error", delErr)
		}
		return nil, err
	}

	// Copy payment details to purchase row for revenue tracking after the
	// purchase is actually fulfilled so failed attempts can be retried.
	now := time.Now()
	_ = s.purchaseRepository.UpdateFields(ctx, purchaseID, map[string]interface{}{
		"transaction_id": info.TransactionID,
		"payment_method": matchedProvider.Key,
		"payment_phone":  info.PhoneNumber,
		"verified_at":    now,
	})

	slog.Info("Mobile payment verified and processed", "purchase_id", purchaseID, "txn_id", info.TransactionID, "provider", matchedProvider.Key, "matched_by", matchedBy)
	return &VerificationResult{Success: true, ReasonKey: "mobile_pay_success"}, nil
}

// normalizePhone strips formatting and country code to produce a comparable local number.
// buildUserDescription creates a description for the Remnawave user.
// Format: "Wavy: 1 Month 100GB | TG: 5075836448 | Tx: 01004066..."
func buildUserDescription(paymentMethod, planLabel string, days int, trafficGB int, telegramID int64, txnID string) string {
	provider := paymentMethod
	if provider == "" {
		provider = "Wavy"
	}

	plan := planLabel
	if plan == "" {
		plan = fmt.Sprintf("%d Days %dGB", days, trafficGB)
	}

	desc := fmt.Sprintf("%s: %s | TG: %d", provider, plan, telegramID)
	if txnID != "" {
		desc += " | Tx: " + txnID
	}
	return desc
}

func normalizePhone(phone string) string {
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	phone = strings.ReplaceAll(phone, "*", "") // masked digits
	phone = strings.TrimPrefix(phone, "+")
	// Myanmar country code is 95. E.g. +959xxxxxxxx → 959xxxxxxxx.
	// Local format is 09xxxxxxxx.
	// Normalize both to 9xxxxxxxx (strip leading 95 or 0 but keep the 9).
	if strings.HasPrefix(phone, "95") && len(phone) > 9 {
		phone = phone[2:] // +959xxx → 9xxx
	}
	phone = strings.TrimPrefix(phone, "0") // 09xxx → 9xxx
	return phone
}

// phoneMatchesSuffix checks if two phone numbers share the same last N digits.
// This handles cases where banking apps mask/truncate the phone number.
func phoneMatchesSuffix(expected, actual string, n int) bool {
	if actual == "" {
		return false
	}
	// If we have the full number, try exact match first
	if actual == expected {
		return true
	}
	// Fall back to last N digits comparison
	expSuffix := expected
	if len(expected) > n {
		expSuffix = expected[len(expected)-n:]
	}
	actSuffix := actual
	if len(actual) > n {
		actSuffix = actual[len(actual)-n:]
	}
	return expSuffix == actSuffix
}

func (s *PaymentService) createWalletTopUpInvoice(ctx context.Context, amount float64, customer *database.Customer, promoID *int64) (url string, purchaseId int64, err error) {
	if len(GetEnabledPaymentProviders()) == 0 {
		return "", 0, fmt.Errorf("no mobile banking accounts configured")
	}

	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType:    database.InvoiceTypeWalletTopUp,
		Status:         database.PurchaseStatusPending,
		Amount:         amount,
		Currency:       config.Currency(),
		CustomerID:     customer.ID,
		Month:          0, // Not applicable for top-up
		Days:           0, // Not applicable
		TrafficLimitGB: 0, // Not applicable
	})
	if err != nil {
		slog.Error("Error creating wallet top-up", "error", err)
		return "", 0, err
	}
	slog.Info("Wallet top-up invoice created", "purchase_id", purchaseId, "customer_id", customer.ID)
	return "", purchaseId, nil
}

// CreateWalletPayment was a duplicate of createWalletPurchase with diverging behavior
// (it set paid_at before delivery succeeded). Removed — all wallet payments now go
// through CreatePurchase → createWalletPurchase which is the single, correct code path.
