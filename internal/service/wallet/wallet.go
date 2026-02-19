package wallet

import (
	"context"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type WalletService struct {
	paymentService     *payment.PaymentService
	customerRepo       *database.CustomerRepository
	purchaseRepo       *database.PurchaseRepository
	remnawaveClient    *remnawave.Client
	telegramBot        *bot.Bot
	translationManager *translation.Manager
	subKeyRepo         *database.SubscriptionKeyRepository
	walletTxRepo       *database.WalletTransactionRepository
}

func NewWalletService(
	paymentService *payment.PaymentService,
	customerRepo *database.CustomerRepository,
	purchaseRepo *database.PurchaseRepository,
	remnawaveClient *remnawave.Client,
	telegramBot *bot.Bot,
	translationManager *translation.Manager,
	subKeyRepo *database.SubscriptionKeyRepository,
	walletTxRepo *database.WalletTransactionRepository,
) *WalletService {
	return &WalletService{
		paymentService:     paymentService,
		customerRepo:       customerRepo,
		purchaseRepo:       purchaseRepo,
		remnawaveClient:    remnawaveClient,
		telegramBot:        telegramBot,
		translationManager: translationManager,
		subKeyRepo:         subKeyRepo,
		walletTxRepo:       walletTxRepo,
	}
}

// CreateTopUpInvoice creates an invoice for adding funds to the wallet via existing payment methods.
// Under the hood, it creates a Purchase with InvoiceType=wallet_topup.
func (s *WalletService) CreateTopUpInvoice(ctx context.Context, amount float64, customer *database.Customer, method database.InvoiceType) (string, int64, error) {
	// 1. Enforce Minimum Top-Up Amount (Must be >= Minimum Plan Price)
	// We assume config has a min price or we hardcode based on plans.
	// For now, let's assume 6000 MMK is a safe minimum if not dynamic.
	const MinTopUpAmount = 6000.0
	if amount < MinTopUpAmount {
		return "", 0, fmt.Errorf("minimum top-up amount is %.0f MMK", MinTopUpAmount)
	}

	// 2. Create Purchase via PaymentService
	// Note: We use 0 days and 0 traffic because this is just a balance load.
	return s.paymentService.CreatePurchase(ctx, amount, 0, 0, customer, method, "")
}

// PurchaseWithBalance attempts to buy a plan using wallet balance.
func (s *WalletService) PurchaseWithBalance(ctx context.Context, customerID int64, planPrice float64, days int, trafficGB int, promoCode string) error {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer not found")
	}

	// 1. Validate Balance
	if customer.Balance < planPrice {
		return fmt.Errorf("insufficient balance: have %.2f, need %.2f", customer.Balance, planPrice)
	}

	// 2. Create Instant Payment
	// We delegate to PaymentService.CreatePurchase which handles atomic deduction (via DeductBalance)
	// and transaction recording.
	_, _, err = s.paymentService.CreatePurchase(ctx, planPrice, days, trafficGB, customer, database.InvoiceTypeWalletPayment, promoCode)
	if err != nil {
		return err
	}

	return nil
}

// RunAutoRenewCheck is the Cron Job function.
// Checks users with auto_renew=true expiring in < 3 days.
func (s *WalletService) RunAutoRenewCheck(ctx context.Context) {
	slog.Info("Running Auto-Renew Cron Job")

	// 1. Find candidates (Active Auto-Renew, Expiring Soon, Has Balance)
	// We need a custom query for this logic.
	// Logic: expire_at BETWEEN now AND now+3days AND auto_renew=true
	now := time.Now()
	threeDaysLater := now.Add(72 * time.Hour)

	candidates, err := s.customerRepo.FindByExpirationRange(ctx, now, threeDaysLater)
	if err != nil {
		slog.Error("AutoRenew: Failed to fetch candidates", "error", err)
		return
	}

	if candidates == nil {
		return
	}

	for _, user := range *candidates {
		// Verify AutoRenew flag explicitly (FindByExpirationRange might not filter by it)
		if !user.AutoRenew {
			continue
		}

		// Verify Balance vs 'AutoRenewPlanPrice'
		// PROBLEM: We need to know HOW MUCH to deduct.
		// Solution: We look at their *last paid purchase* to determine the renewal price/plan.
		lastPurchase, err := s.purchaseRepo.FindSuccessfulPaidPurchaseByCustomer(ctx, user.ID)
		if err != nil || lastPurchase == nil {
			slog.Warn("AutoRenew: No previous purchase found for pricing", "user_id", user.ID)
			continue
		}

		// Price to renew = Last Purchase Amount (assuming price stability)
		// Or utilize `auto_renew_duration` (e.g., 30 days) and fetch current config price.
		// For MVP, let's use the Last Purchase Amount.
		renewPrice := lastPurchase.Amount
		renewDays := lastPurchase.Days
		renewTraffic := lastPurchase.TrafficLimitGB

		if user.Balance >= renewPrice {
			// === RENEW ===
			slog.Info("AutoRenew: Renewing user", "user_id", user.ID, "amount", renewPrice)
			err := s.PurchaseWithBalance(ctx, user.ID, renewPrice, renewDays, renewTraffic, "")
			if err != nil {
				slog.Error("AutoRenew: Failed to renew", "user_id", user.ID, "error", err)
				// Notify User of Failure?
				s.notifyUser(ctx, &user, "auto_renew_failed_generic")
			} else {
				// Allow notification to be handled by CreateWalletPayment -> ProcessPurchase -> Notify
				// But we might want a specific "Auto-Renewed" message.
				s.notifyUser(ctx, &user, "auto_renew_success")
			}
		} else {
			// === INSUFFICIENT FUNDS ===
			// Notify ONLY if we haven't nagged them recently?
			// For now, notify every run (daily) if in window.
			slog.Info("AutoRenew: Insufficient funds", "user_id", user.ID, "balance", user.Balance, "needed", renewPrice)
			s.notifyUser(ctx, &user, "auto_renew_insufficient_funds")
		}
	}
}

func (s *WalletService) notifyUser(ctx context.Context, customer *database.Customer, messageKey string) {
	msg := s.translationManager.GetText(customer.Language, messageKey)
	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID,
		Text:   msg,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: s.translationManager.GetText(customer.Language, "open_wallet_button"), WebApp: &models.WebAppInfo{URL: config.GetMiniAppURL() + "/wallet"}},
				},
			},
		},
	})
	if err != nil {
		slog.Error("AutoRenew: Detailed notification failed", "user_id", customer.ID, "error", err)
	}
}

// === Interface Implementation for API ===

func (s *WalletService) GetBalance(ctx context.Context, customerID int64) (float64, error) {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil {
		return 0, err
	}
	if customer == nil {
		return 0, fmt.Errorf("customer not found")
	}
	return customer.Balance, nil
}

func (s *WalletService) GetTransactionHistory(ctx context.Context, customerID int64, limit int) ([]database.WalletTransaction, error) {
	return s.walletTxRepo.FindByCustomerID(ctx, customerID, limit)
}

func (s *WalletService) HasSufficientBalance(ctx context.Context, customerID int64, amount float64) (bool, error) {
	bal, err := s.GetBalance(ctx, customerID)
	if err != nil {
		return false, err
	}
	return bal >= amount, nil
}

func (s *WalletService) DeductBalance(ctx context.Context, customerID int64, amount float64, purchaseID int64, description string) error {
	// Begin a transaction so the balance deduction and the wallet_transaction log are atomic.
	dbTx, err := s.customerRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin deduct-balance transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = dbTx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := s.customerRepo.DeductBalanceTx(ctx, dbTx, customerID, amount); err != nil {
		_ = dbTx.Rollback(ctx)
		return err
	}

	pid := &purchaseID
	if purchaseID == 0 {
		pid = nil
	}

	if _, err := s.walletTxRepo.CreateTx(ctx, dbTx, &database.WalletTransaction{
		CustomerID:  customerID,
		Amount:      -amount, // negative = outflow
		Type:        database.WalletTransactionTypePurchase,
		PurchaseID:  pid,
		Description: description,
	}); err != nil {
		_ = dbTx.Rollback(ctx)
		return fmt.Errorf("failed to log wallet deduction: %w", err)
	}

	if err := dbTx.Commit(ctx); err != nil {
		_ = dbTx.Rollback(ctx)
		return fmt.Errorf("failed to commit wallet deduction: %w", err)
	}
	return nil
}

func (s *WalletService) SetAutoRenew(ctx context.Context, customerID int64, enabled bool, duration int) error {
	return s.customerRepo.SetAutoRenew(ctx, customerID, enabled, duration)
}

func (s *WalletService) GetAutoRenewStatus(ctx context.Context, customerID int64) (enabled bool, duration int, err error) {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil {
		return false, 0, err
	}
	if customer == nil {
		return false, 0, fmt.Errorf("customer not found")
	}
	return customer.AutoRenew, customer.AutoRenewDuration, nil
}
