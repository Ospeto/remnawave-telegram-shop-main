// Package autorenew runs the per-key auto-renewal cron job.
//
// Design: Auto-renew is per subscription key, NOT per customer.
// Each key has its own auto_renew flag. When the cron fires, it finds
// every key where auto_renew=true AND expire_at is within 3 days, then
// EXTENDS that specific key (not creates a new one) using the customer's
// wallet balance.
//
// Safety guarantees:
//   - Idempotency: last_auto_renewed_at is stamped on each key after renewal;
//     the same key cannot be charged twice in one expiry cycle.
//   - 24h notification throttle: low-balance warnings are sent at most once
//     per day via auto_renew_notified_at on the key.
//   - Ownership: balance and key both belong to the same customer.
package autorenew

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/translation"
)

type keyRepository interface {
	FindExpiringAutoRenewKeys(ctx context.Context, before time.Time) ([]database.SubscriptionKey, error)
	TryClaimAutoRenew(ctx context.Context, keyID int64, expectedLast *time.Time) (*time.Time, bool, error)
	RestoreAutoRenewClaim(ctx context.Context, keyID int64, claimedAt time.Time, previous *time.Time) error
	MarkKeyAutoRenewed(ctx context.Context, keyID int64) error
	MarkKeyAutoRenewNotified(ctx context.Context, keyID int64) error
}

type customerRepository interface {
	FindById(ctx context.Context, id int64) (*database.Customer, error)
}

type walletExtender interface {
	ExtendKeyWithBalance(ctx context.Context, keyID int64, customerID int64, planPrice float64, days int, trafficGB int) error
}

type textProvider interface {
	GetText(langCode, key string) string
}

type telegramClient interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
}

// Job encapsulates the scheduled per-key auto-renew process.
type Job struct {
	subKeyRepo    keyRepository
	customerRepo  customerRepository
	walletService walletExtender
	tm            textProvider
	telegramBot   telegramClient
	nowFn         func() time.Time
	selectPlanFn  func(database.SubscriptionKey, float64) (*config.Plan, bool)
}

// New creates a new AutoRenew Job.
func New(
	subKeyRepo *database.SubscriptionKeyRepository,
	customerRepo *database.CustomerRepository,
	walletService *wallet.WalletService,
	tm *translation.Manager,
	b *bot.Bot,
) *Job {
	return &Job{
		subKeyRepo:    subKeyRepo,
		customerRepo:  customerRepo,
		walletService: walletService,
		tm:            tm,
		telegramBot:   b,
		nowFn:         time.Now,
		selectPlanFn:  findAffordablePlan,
	}
}

// Run processes auto-renewals for all eligible subscription keys.
// Called by the cron scheduler (daily at 9 AM).
func (j *Job) Run(ctx context.Context) {
	slog.Info("Auto-renew: per-key cron job started")

	threeDaysFromNow := j.nowFn().Add(3 * 24 * time.Hour)
	keys, err := j.subKeyRepo.FindExpiringAutoRenewKeys(ctx, threeDaysFromNow)
	if err != nil {
		slog.Error("Auto-renew: error finding expiring keys", "error", err)
		return
	}

	slog.Info("Auto-renew: found candidate keys", "count", len(keys))

	for _, key := range keys {
		j.processKey(ctx, key)
	}

	slog.Info("Auto-renew: per-key cron job finished")
}

// processKey handles renewal or low-balance warning for one subscription key.
func (j *Job) processKey(ctx context.Context, key database.SubscriptionKey) {
	log := slog.With("key_id", key.ID, "customer_id", key.CustomerID)

	// ── Load customer ─────────────────────────────────────────────────────────
	customer, err := j.customerRepo.FindById(ctx, key.CustomerID)
	if err != nil || customer == nil {
		log.Error("Auto-renew: customer not found, skipping key")
		return
	}

	// ── Idempotency guard ─────────────────────────────────────────────────────
	// If this key was already renewed in the current expiry cycle, skip it.
	if key.LastAutoRenewedAt != nil && key.ExpireAt != nil {
		if key.LastAutoRenewedAt.After(key.ExpireAt.Add(-4 * 24 * time.Hour)) {
			log.Info("Auto-renew: skipping — already renewed in this cycle",
				"last_renewed", key.LastAutoRenewedAt, "expire_at", key.ExpireAt)
			return
		}
	}

	// ── Cross-replica claim guard ─────────────────────────────────────────────
	// Claim this key atomically before charging wallet balance so two replicas
	// cannot renew the same key at the same time.
	claimedAt, claimed, err := j.subKeyRepo.TryClaimAutoRenew(ctx, key.ID, key.LastAutoRenewedAt)
	if err != nil {
		log.Error("Auto-renew: failed to claim key", "error", err)
		return
	}
	if !claimed {
		log.Info("Auto-renew: skipping — key already claimed by another worker")
		return
	}

	claimFinalized := false
	defer func() {
		if claimFinalized || claimedAt == nil {
			return
		}
		restoreCtx := context.WithoutCancel(ctx)
		if err := j.subKeyRepo.RestoreAutoRenewClaim(restoreCtx, key.ID, *claimedAt, key.LastAutoRenewedAt); err != nil {
			log.Error("Auto-renew: failed to release claim after error", "error", err)
		}
	}()

	// ── Find the best affordable plan ─────────────────────────────────────────
	// findAffordablePlan prefers the most expensive plan the user can afford
	// (best value), falling back to cheaper options of the same traffic type.
	// Returns nil if no plan at all is affordable.
	plan, isFallback := j.selectPlanFn(key, customer.Balance)
	if plan == nil {
		log.Warn("Auto-renew: insufficient balance for any plan",
			"balance", customer.Balance, "key_traffic_gb", key.TrafficLimitGB)
		j.handleInsufficientFunds(ctx, customer, &key, nil)
		return
	}

	log.Info("Auto-renew: selected plan",
		"plan_label", plan.Label, "plan_days", plan.Days,
		"plan_price", plan.Price, "plan_traffic_gb", plan.TrafficLimitGB,
		"is_fallback", isFallback)

	// ── Extend the specific key ───────────────────────────────────────────────
	err = j.walletService.ExtendKeyWithBalance(ctx, key.ID, customer.ID, float64(plan.Price), plan.Days, plan.TrafficLimitGB)
	if err != nil {
		log.Error("Auto-renew: key extension failed", "error", err)
		j.sendMessage(ctx, customer.TelegramID, customer.Language,
			j.tm.GetText(customer.Language, "auto_renew_failed"))
		return
	}
	claimFinalized = true

	log.Info("Auto-renew: key extended successfully", "is_fallback", isFallback)

	if isFallback {
		msg := fmt.Sprintf(
			j.tm.GetText(customer.Language, "auto_renew_fallback_detail"),
			plan.Days, plan.Price,
		)
		j.sendMessage(ctx, customer.TelegramID, customer.Language, msg)
	} else {
		msg := fmt.Sprintf(
			j.tm.GetText(customer.Language, "auto_renew_success_detail"),
			plan.Label, plan.Days, plan.Price,
		)
		j.sendMessage(ctx, customer.TelegramID, customer.Language, msg)
	}
}

// handleInsufficientFunds sends a low-balance notification at most once per 24h.
// plan may be nil when no plan of any price is affordable.
func (j *Job) handleInsufficientFunds(ctx context.Context, customer *database.Customer, key *database.SubscriptionKey, plan *config.Plan) {
	log := slog.With("key_id", key.ID, "customer_id", customer.ID)

	if key.AutoRenewNotifiedAt != nil && j.nowFn().Sub(*key.AutoRenewNotifiedAt) < 24*time.Hour {
		log.Info("Auto-renew: low-balance notification recently sent — suppressing")
		return
	}

	var neededPrice int
	if plan != nil {
		neededPrice = plan.Price
	} else {
		neededPrice = minimumPlanPriceForKey(*key)
	}
	shortfall := 0
	if deficit := float64(neededPrice) - customer.Balance; deficit > 0 {
		shortfall = int(math.Ceil(deficit))
	}
	log.Info("Auto-renew: insufficient funds — notifying", "balance", customer.Balance, "needed", neededPrice)

	msg := fmt.Sprintf(
		j.tm.GetText(customer.Language, "auto_renew_insufficient_balance_detail"),
		shortfall,
	)
	j.sendMessage(ctx, customer.TelegramID, customer.Language, msg)

	if err := j.subKeyRepo.MarkKeyAutoRenewNotified(ctx, key.ID); err != nil {
		log.Error("Auto-renew: failed to stamp auto_renew_notified_at", "error", err)
	}
}

// findAffordablePlan picks the best plan for renewal given the customer's balance.
//
// Strategy:
//  1. Gather all plans with the same traffic type as the key
//     (unlimited = traffic_limit_gb 0, limited = traffic_limit_gb > 0).
//  2. Try to find the plan whose duration best matches the key's last purchase
//     (longest duration the balance can afford that is >= remaining days, or
//     simply the longest duration available if balance covers it).
//  3. If balance cannot cover *any* same-type plan, return nil (caller notifies user).
//  4. On tie, always prefer longer duration (better value for user).
//
// Returns:  (planToUse, isFallback)
// isFallback=true means the original/preferred plan was too expensive and we
// fell back to the cheapest affordable option.
func findAffordablePlan(key database.SubscriptionKey, balance float64) (plan *config.Plan, isFallback bool) {
	allPlans := config.Plans()
	if len(allPlans) == 0 {
		return nil, false
	}

	isUnlimited := key.TrafficLimitGB == 0

	// Filter to same traffic type.
	var sametype []config.Plan
	for _, p := range allPlans {
		if isUnlimited && p.TrafficLimitGB == 0 {
			sametype = append(sametype, p)
		} else if !isUnlimited && p.TrafficLimitGB > 0 {
			sametype = append(sametype, p)
		}
	}
	if len(sametype) == 0 {
		// Fallback: if no matching traffic type plans exist, try any plan.
		sametype = allPlans
	}

	// Sort by price descending so we try the most expensive affordable plan first.
	// (Gives the user the best value — longest duration they can afford.)
	sort.Slice(sametype, func(i, j int) bool { return sametype[i].Price > sametype[j].Price })

	// Find the cheapest plan (for the hard fallback).
	cheapest := sametype[len(sametype)-1]
	for _, p := range sametype {
		if p.Price < cheapest.Price {
			cheapest = p
		}
	}

	// Walk from most expensive to cheapest and return the first one the user can afford.
	for i, p := range sametype {
		plan := p
		if float64(plan.Price) <= balance {
			isFallback = i == len(sametype)-1 && plan.Price == cheapest.Price && len(sametype) > 1
			return &plan, isFallback
		}
	}

	// Balance doesn't even cover the cheapest same-type plan.
	return nil, false
}

func minimumPlanPriceForKey(key database.SubscriptionKey) int {
	allPlans := config.Plans()
	if len(allPlans) == 0 {
		return config.LowestPlanPrice()
	}

	isUnlimited := key.TrafficLimitGB == 0
	minPrice := 0
	found := false

	for _, plan := range allPlans {
		if isUnlimited && plan.TrafficLimitGB != 0 {
			continue
		}
		if !isUnlimited && plan.TrafficLimitGB == 0 {
			continue
		}
		if !found || plan.Price < minPrice {
			minPrice = plan.Price
			found = true
		}
	}

	if found {
		return minPrice
	}
	return config.LowestPlanPrice()
}

// sendMessage is a best-effort Telegram notification helper.
func (j *Job) sendMessage(ctx context.Context, chatID int64, _ string, text string) {
	if j.telegramBot == nil {
		return
	}
	if _, err := j.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}); err != nil {
		slog.Error("Auto-renew: failed to send notification", "chat_id", chatID, "error", err)
	}
}
