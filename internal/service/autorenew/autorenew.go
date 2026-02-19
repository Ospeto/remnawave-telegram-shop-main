// Package autorenew contains the scheduled auto-renewal job that checks for
// customers with auto_renew=true whose subscriptions expire within 3 days and
// attempts to renew their plan using their wallet balance.
//
// Safety guarantees:
//   - Idempotency: last_auto_renewed_at is stamped on success; the same expiry
//     cycle cannot result in a double charge even if the cron fires twice.
//   - Correct plan matching: plans are matched by both duration (days) AND
//     traffic type (unlimited vs limited GB) so the user always gets the same
//     category of plan they signed up for.
//   - Notification throttle: low-balance warnings are sent at most once per
//     24-hour window via auto_renew_notified_at.
package autorenew

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/translation"
)

// Job encapsulates the scheduled auto-renew process.
type Job struct {
	customerRepo   *database.CustomerRepository
	walletService  *wallet.WalletService
	paymentService *payment.PaymentService
	tm             *translation.Manager
	telegramBot    *bot.Bot
}

// New creates a new AutoRenew Job.
func New(
	customerRepo *database.CustomerRepository,
	walletService *wallet.WalletService,
	paymentService *payment.PaymentService,
	tm *translation.Manager,
	b *bot.Bot,
) *Job {
	return &Job{
		customerRepo:   customerRepo,
		walletService:  walletService,
		paymentService: paymentService,
		tm:             tm,
		telegramBot:    b,
	}
}

// Run processes auto-renewals for all eligible customers.
// It is intended to be called by a cron scheduler (daily).
func (j *Job) Run(ctx context.Context) {
	slog.Info("Auto-renew: cron job started")

	threeDaysFromNow := time.Now().Add(3 * 24 * time.Hour)
	customers, err := j.customerRepo.FindByAutoRenewExpiring(ctx, threeDaysFromNow)
	if err != nil {
		slog.Error("Auto-renew: error finding candidates", "error", err)
		return
	}

	slog.Info("Auto-renew: found candidates", "count", len(customers))

	for _, customer := range customers {
		j.processCustomer(ctx, customer)
	}

	slog.Info("Auto-renew: cron job finished")
}

// processCustomer handles renewal or pre-expiry warning for one customer.
func (j *Job) processCustomer(ctx context.Context, customer database.Customer) {
	log := slog.With("customer_id", customer.ID)

	// ── Idempotency guard ─────────────────────────────────────────────────────
	// If we already renewed this customer in the current expiry cycle (i.e. the
	// last_auto_renewed_at is AFTER the point where their subscription would have
	// been extended from), skip them. This prevents double-charging if the cron
	// fires twice in one day or the bot restarts mid-run.
	if customer.LastAutoRenewedAt != nil && customer.ExpireAt != nil {
		// They were renewed after their previous expiry → renewal already done.
		if customer.LastAutoRenewedAt.After(customer.ExpireAt.Add(-4 * 24 * time.Hour)) {
			log.Info("Auto-renew: skipping – already renewed in this cycle",
				"last_renewed", customer.LastAutoRenewedAt,
				"expire_at", customer.ExpireAt)
			return
		}
	}

	// ── Find the plan to renew ────────────────────────────────────────────────
	// Match by both duration (days) AND traffic type (unlimited=0 or specific
	// GB cap) so users are never silently switched between plan categories.
	plan := j.findPlan(customer.AutoRenewDuration, customer.AutoRenewTrafficGB)
	if plan == nil {
		log.Warn("Auto-renew: no matching plan found – skipping",
			"wanted_days", customer.AutoRenewDuration,
			"wanted_traffic_gb", customer.AutoRenewTrafficGB)
		// Notify admin / user that config has drifted.
		j.sendMessage(ctx, customer.TelegramID,
			j.tm.GetText(customer.Language, "auto_renew_plan_not_found"))
		return
	}

	log.Info("Auto-renew: matched plan",
		"plan_label", plan.Label,
		"plan_days", plan.Days,
		"plan_price", plan.Price,
		"plan_traffic_gb", plan.TrafficLimitGB)

	// ── Check balance ─────────────────────────────────────────────────────────
	hasBalance, err := j.walletService.HasSufficientBalance(ctx, customer.ID, float64(plan.Price))
	if err != nil {
		log.Error("Auto-renew: error checking balance", "error", err)
		return
	}

	if !hasBalance {
		j.handleInsufficientFunds(ctx, customer, plan)
		return
	}

	// ── Charge and renew ──────────────────────────────────────────────────────
	_, purchaseID, err := j.paymentService.CreatePurchase(
		ctx,
		float64(plan.Price),
		plan.Days,
		plan.TrafficLimitGB,
		&customer,
		database.InvoiceTypeWalletPayment,
		"",
	)
	if err != nil {
		log.Error("Auto-renew: renewal failed", "error", err)
		j.sendMessage(ctx, customer.TelegramID,
			j.tm.GetText(customer.Language, "auto_renew_failed"))
		return
	}

	// Stamp last_auto_renewed_at to prevent double-charge on cron re-run.
	if stampErr := j.customerRepo.MarkAutoRenewed(ctx, customer.ID); stampErr != nil {
		// Non-fatal: renewal succeeded, just log the stamp failure.
		log.Error("Auto-renew: failed to stamp last_auto_renewed_at (non-fatal)", "error", stampErr)
	}

	log.Info("Auto-renew: renewal successful", "purchase_id", purchaseID)
	msg := fmt.Sprintf(j.tm.GetText(customer.Language, "auto_renew_success_detail"),
		plan.Label, plan.Days, plan.Price)
	j.sendMessage(ctx, customer.TelegramID, msg)
}

// handleInsufficientFunds sends a low-balance notification at most once per 24h.
func (j *Job) handleInsufficientFunds(ctx context.Context, customer database.Customer, plan *config.Plan) {
	log := slog.With("customer_id", customer.ID)

	// Throttle: only notify if we haven't notified in the last 24 hours.
	if customer.AutoRenewNotifiedAt != nil {
		if time.Since(*customer.AutoRenewNotifiedAt) < 24*time.Hour {
			log.Info("Auto-renew: low-balance notification already sent recently – suppressing")
			return
		}
	}

	log.Info("Auto-renew: insufficient funds – notifying user",
		"balance", customer.Balance, "needed", plan.Price)

	msg := fmt.Sprintf(j.tm.GetText(customer.Language, "auto_renew_insufficient_balance_detail"),
		plan.Price, customer.Balance)
	j.sendMessage(ctx, customer.TelegramID, msg)

	// Stamp auto_renew_notified_at so we don't spam them.
	if err := j.customerRepo.MarkAutoRenewNotified(ctx, customer.ID); err != nil {
		log.Error("Auto-renew: failed to stamp auto_renew_notified_at", "error", err)
	}
}

// findPlan returns the config plan matching the given days AND traffic type.
// trafficGB == 0 means "unlimited"; a positive value matches the exact GB cap
// or any unlimited plan if no limited plan matches (graceful fallback).
func (j *Job) findPlan(days int, trafficGB int) *config.Plan {
	// First pass: exact match (days + traffic type).
	for _, plan := range config.Plans() {
		p := plan
		if p.Days == days && p.TrafficLimitGB == trafficGB {
			return &p
		}
	}

	// Second pass: same days, same traffic category (unlimited=0 or any limited).
	// This handles the case where the exact GB cap was changed in config.
	isUnlimited := trafficGB == 0
	for _, plan := range config.Plans() {
		p := plan
		if p.Days == days {
			if isUnlimited && p.TrafficLimitGB == 0 {
				return &p
			}
			if !isUnlimited && p.TrafficLimitGB > 0 {
				return &p
			}
		}
	}

	return nil
}

// sendMessage is a best-effort Telegram notification helper.
func (j *Job) sendMessage(ctx context.Context, chatID int64, text string) {
	if _, err := j.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}); err != nil {
		slog.Error("Auto-renew: failed to send notification", "chat_id", chatID, "error", err)
	}
}
