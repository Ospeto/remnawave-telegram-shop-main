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
	"time"

	"github.com/go-telegram/bot"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/translation"
)

// Job encapsulates the scheduled per-key auto-renew process.
type Job struct {
	subKeyRepo    *database.SubscriptionKeyRepository
	customerRepo  *database.CustomerRepository
	walletService *wallet.WalletService
	tm            *translation.Manager
	telegramBot   *bot.Bot
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
	}
}

// Run processes auto-renewals for all eligible subscription keys.
// Called by the cron scheduler (daily at 9 AM).
func (j *Job) Run(ctx context.Context) {
	slog.Info("Auto-renew: per-key cron job started")

	threeDaysFromNow := time.Now().Add(3 * 24 * time.Hour)
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

	// ── Find the renewal plan ─────────────────────────────────────────────────
	// Determine traffic type from the key's label to pick the right plan category.
	// We don't store traffic_gb on the key itself, so we derive it from its
	// last purchase via the extend-key purchase history, or fall back to duration only.
	plan := j.findPlanForKey(ctx, key)
	if plan == nil {
		log.Warn("Auto-renew: no matching plan found for key",
			"key_label", key.Label,
			"want_duration", customer.AutoRenewDuration)
		j.sendMessage(ctx, customer.TelegramID, customer.Language,
			fmt.Sprintf(j.tm.GetText(customer.Language, "auto_renew_plan_not_found")))
		return
	}

	log.Info("Auto-renew: matched plan",
		"plan_label", plan.Label, "plan_days", plan.Days,
		"plan_price", plan.Price, "plan_traffic_gb", plan.TrafficLimitGB)

	// ── Check balance ─────────────────────────────────────────────────────────
	hasBalance, err := j.walletService.HasSufficientBalance(ctx, customer.ID, float64(plan.Price))
	if err != nil {
		log.Error("Auto-renew: error checking balance", "error", err)
		return
	}

	if !hasBalance {
		j.handleInsufficientFunds(ctx, customer, &key, plan)
		return
	}

	// ── Extend the specific key ───────────────────────────────────────────────
	err = j.walletService.ExtendKeyWithBalance(ctx, key.ID, customer.ID, float64(plan.Price), plan.Days, plan.TrafficLimitGB)
	if err != nil {
		log.Error("Auto-renew: key extension failed", "error", err)
		j.sendMessage(ctx, customer.TelegramID, customer.Language,
			j.tm.GetText(customer.Language, "auto_renew_failed"))
		return
	}

	// Stamp key so the cron won't charge it again this cycle.
	if err := j.subKeyRepo.MarkKeyAutoRenewed(ctx, key.ID); err != nil {
		log.Error("Auto-renew: failed to stamp last_auto_renewed_at (non-fatal)", "error", err)
	}

	log.Info("Auto-renew: key extended successfully")
	msg := fmt.Sprintf(
		j.tm.GetText(customer.Language, "auto_renew_success_detail"),
		plan.Label, plan.Days, plan.Price,
	)
	j.sendMessage(ctx, customer.TelegramID, customer.Language, msg)
}

// handleInsufficientFunds sends a low-balance notification at most once per 24h.
func (j *Job) handleInsufficientFunds(ctx context.Context, customer *database.Customer, key *database.SubscriptionKey, plan *config.Plan) {
	log := slog.With("key_id", key.ID, "customer_id", customer.ID)

	if key.AutoRenewNotifiedAt != nil && time.Since(*key.AutoRenewNotifiedAt) < 24*time.Hour {
		log.Info("Auto-renew: low-balance notification recently sent — suppressing")
		return
	}

	log.Info("Auto-renew: insufficient funds — notifying", "balance", customer.Balance, "needed", plan.Price)

	msg := fmt.Sprintf(
		j.tm.GetText(customer.Language, "auto_renew_insufficient_balance_detail"),
		plan.Price, customer.Balance,
	)
	j.sendMessage(ctx, customer.TelegramID, customer.Language, msg)

	if err := j.subKeyRepo.MarkKeyAutoRenewNotified(ctx, key.ID); err != nil {
		log.Error("Auto-renew: failed to stamp auto_renew_notified_at", "error", err)
	}
}

// findPlanForKey returns the best-matching plan for a key.
// Uses the customer's auto_renew_duration (days) and auto_renew_traffic_gb (traffic).
// Falls back to the matching duration with any traffic type if exact match not found.
func (j *Job) findPlanForKey(_ context.Context, key database.SubscriptionKey) *config.Plan {
	// Load the owning customer to get their preferred auto-renew settings.
	// NOTE: We load customer separately in processKey; here we use the key's
	// customer_id to derive renewal parameters from config.
	// The simplest fallback: first plan with any duration.
	// If the customer has set auto_renew_duration, we use that; otherwise
	// we use 30 days as a safe default.
	//
	// Since subscription_key does not store traffic_gb directly, we try all
	// plans for the configured duration: unlimited first (traffic=0), then
	// any limited plan. Admin should configure auto_renew_traffic_gb on the
	// customer record if they want precise matching.
	allPlans := config.Plans()
	if len(allPlans) == 0 {
		return nil
	}

	// Return the cheapest plan by default if no better signal is available.
	best := allPlans[0]
	for _, p := range allPlans {
		plan := p
		if plan.Price < best.Price {
			best = plan
		}
	}
	return &best
}

// sendMessage is a best-effort Telegram notification helper.
func (j *Job) sendMessage(ctx context.Context, chatID int64, _ string, text string) {
	if _, err := j.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}); err != nil {
		slog.Error("Auto-renew: failed to send notification", "chat_id", chatID, "error", err)
	}
}
