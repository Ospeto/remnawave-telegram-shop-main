// Package autorenew contains the scheduled auto-renewal job that checks for
// customers with auto_renew=true whose subscriptions expire within 3 days and
// attempts to renew their plan using their wallet balance.
package autorenew

import (
	"context"
	"log/slog"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/translation"
	"time"

	"github.com/go-telegram/bot"
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
// It is intended to be called by a cron scheduler.
func (j *Job) Run(ctx context.Context) {
	threeDaysFromNow := time.Now().Add(3 * 24 * time.Hour)
	customers, err := j.customerRepo.FindByAutoRenewExpiring(ctx, threeDaysFromNow)
	if err != nil {
		slog.Error("Auto-renew: error finding candidates", "error", err)
		return
	}

	for _, customer := range customers {
		plan := j.findPlanByDuration(customer.AutoRenewDuration)
		if plan == nil {
			slog.Warn("Auto-renew: no matching plan for customer", "customer_id", customer.ID, "duration", customer.AutoRenewDuration)
			continue
		}

		hasBalance, err := j.walletService.HasSufficientBalance(ctx, customer.ID, float64(plan.Price))
		if err != nil {
			slog.Error("Auto-renew: error checking balance", "customer_id", customer.ID, "error", err)
			continue
		}

		if !hasBalance {
			j.sendMessage(ctx, customer.TelegramID, j.tm.GetText(customer.Language, "auto_renew_insufficient_balance"))
			continue
		}

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
			slog.Error("Auto-renew: renewal failed", "customer_id", customer.ID, "error", err)
			j.sendMessage(ctx, customer.TelegramID, j.tm.GetText(customer.Language, "auto_renew_failed"))
			continue
		}

		slog.Info("Auto-renew: renewal successful", "customer_id", customer.ID, "purchase_id", purchaseID)
		j.sendMessage(ctx, customer.TelegramID, j.tm.GetText(customer.Language, "auto_renew_success"))
	}
}

// findPlanByDuration returns the first plan matching the given number of days, or nil.
func (j *Job) findPlanByDuration(days int) *config.Plan {
	for _, plan := range config.Plans() {
		if plan.Days == days {
			return &plan
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
