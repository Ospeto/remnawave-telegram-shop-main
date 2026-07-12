// Package autorenew runs the per-key auto-renewal cron job.
//
// Design: Auto-renew is per subscription key, NOT per customer.
// Each key has its own auto_renew flag. When the cron fires, it finds
// every key where auto_renew=true AND expire_at is within the catch-up
// lookback window (past) through the 3-day forward window, then EXTENDS
// that specific key (not creates a new one) using the customer's wallet
// balance.
//
// Safety guarantees:
//   - Idempotency: last_auto_renewed_at is stamped as a durable cycle lock
//     before wallet/Remnawave side effects; full finalization clears the
//     claim after success. Transient claims use a TTL lease for pre-side-effect
//     reclaim only; post-side-effect ambiguity prefers hold/manual recovery
//     over a second charge.
//   - Intentional money-safe stuck residual: if the process dies after the
//     pre-extend cycle lock is written but before ExtendKeyWithBalance
//     starts/completes, later runs skip the key (alreadyRenewedThisCycle).
//     That is preferred over risking a double charge; ops may clear
//     last_auto_renewed_at for manual recovery. No automatic recharge of
//     that ambiguous state.
//   - 24h notification throttle: low-balance warnings are sent at most once
//     per day via auto_renew_notified_at on the key.
//   - Ownership: balance and key both belong to the same customer.
package autorenew

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/translation"
)

type keyRepository interface {
	FindExpiringAutoRenewKeys(ctx context.Context, after time.Time, before time.Time) ([]database.SubscriptionKey, error)
	TryClaimAutoRenew(ctx context.Context, keyID int64, expectedLast *time.Time) (*time.Time, bool, error)
	ReleaseAutoRenewClaim(ctx context.Context, keyID int64, claimedAt time.Time) error
	MarkKeyAutoRenewed(ctx context.Context, keyID int64, claimedAt time.Time) error
	// StampKeyLastAutoRenewed records a durable cycle lock (last_auto_renewed_at)
	// while keeping the claim held. Called before side effects so mark/stamp
	// failures after extend cannot open a second charge path after TTL reclaim.
	// Crash after this stamp and before extend is an intentional money-safe
	// stuck residual (skip until manual recovery), not a double-charge path.
	StampKeyLastAutoRenewed(ctx context.Context, keyID int64, claimedAt time.Time) error
	// ClearKeyLastAutoRenewed clears the optimistic pre-extend cycle lock
	// (sets last_auto_renewed_at = NULL) while the worker still owns the claim.
	// It does not restore any prior last_auto_renewed_at history — only undoes
	// the optimistic lock so the same cycle may be retried after extend fails.
	ClearKeyLastAutoRenewed(ctx context.Context, keyID int64, claimedAt time.Time) error
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
	selectPlanFn  func(database.SubscriptionKey) (*config.Plan, error)
}

var (
	errAutoRenewPlanUnknown     = errors.New("auto-renew plan is not configured")
	errAutoRenewPlanUnavailable = errors.New("auto-renew plan is no longer available")
)

// autoRenewLookbackWindow is how far past "now" we still select expired keys
// for catch-up after outages. Must cover multi-hour downtime without relying
// solely on the hourly cron window.
const autoRenewLookbackWindow = 48 * time.Hour

// autoRenewClaimTTL aliases the DB lease duration so service + SQL stay aligned.
const autoRenewClaimTTL = database.AutoRenewClaimTTL

// autoRenewCycleGuardWindow is how far before expire_at a last_auto_renewed_at
// stamp still counts as "already renewed this cycle".
const autoRenewCycleGuardWindow = 4 * 24 * time.Hour

// isAutoRenewClaimExpired reports whether a claim lease is older than ttl.
// Stale pre-side-effect claims may be reclaimed; post-side-effect safety relies
// on a durable last_auto_renewed_at cycle lock (not on infinite claim hold alone).
func isAutoRenewClaimExpired(claimedAt, now time.Time, ttl time.Duration) bool {
	return !claimedAt.After(now.Add(-ttl))
}

// alreadyRenewedThisCycle reports whether last_auto_renewed_at falls inside
// the current expiry cycle (within autoRenewCycleGuardWindow before expire_at).
func alreadyRenewedThisCycle(last, expireAt *time.Time) bool {
	if last == nil || expireAt == nil {
		return false
	}
	return last.After(expireAt.Add(-autoRenewCycleGuardWindow))
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
		selectPlanFn:  findConfiguredRenewalPlan,
	}
}

// Run processes auto-renewals for all eligible subscription keys.
// Called by the cron scheduler.
func (j *Job) Run(ctx context.Context) {
	slog.Info("Auto-renew: per-key cron job started")

	now := j.nowFn()
	threeDaysFromNow := now.Add(3 * 24 * time.Hour)
	lookbackStart := now.Add(-autoRenewLookbackWindow)
	keys, err := j.subKeyRepo.FindExpiringAutoRenewKeys(ctx, lookbackStart, threeDaysFromNow)
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
	// Also covers post-side-effect mark failures that stamped last_auto_renewed_at
	// without clearing the claim (M1 / D2 post-side-effect guard).
	if alreadyRenewedThisCycle(key.LastAutoRenewedAt, key.ExpireAt) {
		log.Info("Auto-renew: skipping — already renewed in this cycle",
			"last_renewed", key.LastAutoRenewedAt, "expire_at", key.ExpireAt)
		return
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
		if err := j.subKeyRepo.ReleaseAutoRenewClaim(restoreCtx, key.ID, *claimedAt); err != nil {
			log.Error("Auto-renew: failed to release claim after error", "error", err)
		}
	}()

	plan, err := j.selectPlanFn(key)
	if err != nil {
		log.Warn("Auto-renew: plan unavailable for exact renewal",
			"reason", err.Error(),
			"key_traffic_gb", key.TrafficLimitGB,
			"renewal_days", key.AutoRenewPlanDays)
		j.handleBlockedRenewal(ctx, customer, &key, j.blockedRenewalMessage(customer.Language, err))
		return
	}
	amount, tier := config.ResolvePlanPrice(*plan, customer.IsReseller)
	charge := float64(amount)
	if customer.Balance < charge {
		log.Warn("Auto-renew: insufficient balance for configured plan",
			"balance", customer.Balance,
			"plan_price", amount,
			"pricing_tier", tier,
			"key_traffic_gb", key.TrafficLimitGB,
			"renewal_days", plan.Days)
		j.handleInsufficientFunds(ctx, customer, &key, amount)
		return
	}

	log.Info("Auto-renew: selected plan",
		"plan_label", plan.Label, "plan_days", plan.Days,
		"plan_price", amount, "pricing_tier", tier, "plan_traffic_gb", plan.TrafficLimitGB)

	// Durable cycle lock BEFORE wallet/Remnawave side effects. If mark/stamp
	// both fail after a successful extend, last_auto_renewed_at still blocks
	// a second charge after TTL reclaim (M1 money-safety).
	if err := j.subKeyRepo.StampKeyLastAutoRenewed(ctx, key.ID, *claimedAt); err != nil {
		log.Error("Auto-renew: failed to stamp cycle lock before extend; aborting without charge",
			"error", err)
		return
	}

	// ── Extend the specific key ───────────────────────────────────────────────
	extendCtx := payment.WithPricingTier(ctx, tier)
	err = j.walletService.ExtendKeyWithBalance(extendCtx, key.ID, customer.ID, charge, plan.Days, plan.TrafficLimitGB)
	if err != nil {
		log.Error("Auto-renew: key extension failed", "error", err)
		// Undo optimistic cycle lock so a later run may retry this cycle.
		if clearErr := j.subKeyRepo.ClearKeyLastAutoRenewed(ctx, key.ID, *claimedAt); clearErr != nil {
			log.Error("Auto-renew: failed to clear cycle lock after extend failure",
				"error", clearErr)
		}
		j.sendMessage(ctx, customer.TelegramID, customer.Language,
			j.tm.GetText(customer.Language, "auto_renew_failed"))
		return
	}

	// Side effect succeeded. Never release the claim on finalization failure —
	// cycle lock is already durable; hold claim as belt-and-suspenders.
	if err := j.subKeyRepo.MarkKeyAutoRenewed(ctx, key.ID, *claimedAt); err != nil {
		log.Error("Auto-renew: failed to mark renewal success after extend; holding claim (cycle lock already set)",
			"error", err)
		// Best-effort refresh of last_auto_renewed_at (already stamped pre-extend).
		if stampErr := j.subKeyRepo.StampKeyLastAutoRenewed(ctx, key.ID, *claimedAt); stampErr != nil {
			log.Error("Auto-renew: failed to refresh cycle lock after mark failure; relying on pre-extend stamp",
				"error", stampErr)
		}
		claimFinalized = true
		return
	}
	claimFinalized = true

	log.Info("Auto-renew: key extended successfully")

	msg := fmt.Sprintf(
		j.tm.GetText(customer.Language, "auto_renew_success_detail"),
		plan.Label, plan.Days, amount,
	)
	j.sendMessage(ctx, customer.TelegramID, customer.Language, msg)
}

// handleInsufficientFunds sends a low-balance notification at most once per 24h.
// neededPrice is the resolved charge amount (retail or wholesale).
func (j *Job) handleInsufficientFunds(ctx context.Context, customer *database.Customer, key *database.SubscriptionKey, neededPrice int) {
	log := slog.With("key_id", key.ID, "customer_id", customer.ID)

	if key.AutoRenewNotifiedAt != nil && j.nowFn().Sub(*key.AutoRenewNotifiedAt) < 24*time.Hour {
		log.Info("Auto-renew: low-balance notification recently sent — suppressing")
		return
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
	j.handleBlockedRenewal(ctx, customer, key, msg)
}

func (j *Job) handleBlockedRenewal(ctx context.Context, customer *database.Customer, key *database.SubscriptionKey, message string) {
	log := slog.With("key_id", key.ID, "customer_id", customer.ID)

	if key.AutoRenewNotifiedAt != nil && j.nowFn().Sub(*key.AutoRenewNotifiedAt) < 24*time.Hour {
		log.Info("Auto-renew: renewal warning recently sent — suppressing")
		return
	}

	if err := j.sendMessage(ctx, customer.TelegramID, customer.Language, message); err != nil {
		log.Error("Auto-renew: failed to send renewal warning", "error", err)
		return
	}

	if err := j.subKeyRepo.MarkKeyAutoRenewNotified(ctx, key.ID); err != nil {
		log.Error("Auto-renew: failed to stamp auto_renew_notified_at", "error", err)
	}
}

func findConfiguredRenewalPlan(key database.SubscriptionKey) (*config.Plan, error) {
	if key.AutoRenewPlanDays == nil || *key.AutoRenewPlanDays <= 0 || key.AutoRenewPlanTraffic == nil {
		return nil, errAutoRenewPlanUnknown
	}

	trafficGB := *key.AutoRenewPlanTraffic

	for _, plan := range config.AllPlans() {
		if plan.Days == *key.AutoRenewPlanDays && plan.TrafficLimitGB == trafficGB {
			planCopy := plan
			return &planCopy, nil
		}
	}

	return nil, errAutoRenewPlanUnavailable
}

func (j *Job) blockedRenewalMessage(lang string, err error) string {
	switch {
	case errors.Is(err, errAutoRenewPlanUnknown):
		return j.tm.GetText(lang, "auto_renew_plan_unconfigured_detail")
	case errors.Is(err, errAutoRenewPlanUnavailable):
		return j.tm.GetText(lang, "auto_renew_plan_unavailable_detail")
	default:
		return j.tm.GetText(lang, "auto_renew_failed")
	}
}

// sendMessage is a best-effort Telegram notification helper.
func (j *Job) sendMessage(ctx context.Context, chatID int64, _ string, text string) error {
	if j.telegramBot == nil {
		return nil
	}
	if _, err := j.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}); err != nil {
		slog.Error("Auto-renew: failed to send notification", "chat_id", chatID, "error", err)
		return err
	}
	return nil
}
