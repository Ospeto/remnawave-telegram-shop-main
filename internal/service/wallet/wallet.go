package wallet

import (
	"context"
	"fmt"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"

	"github.com/go-telegram/bot"
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

// SetKeyAutoRenew toggles auto-renew for a specific subscription key.
// Ownership is validated inside the repository (customerID must match key.customer_id).
func (s *WalletService) SetKeyAutoRenew(ctx context.Context, keyID int64, customerID int64, enabled bool) error {
	return s.subKeyRepo.SetAutoRenew(ctx, keyID, customerID, enabled)
}

// ExtendKeyWithBalance extends a specific subscription key using the customer's
// wallet balance. It validates balance first, then delegates to the payment
// service which charges the wallet and calls Remnawave's extend API.
func (s *WalletService) ExtendKeyWithBalance(ctx context.Context, keyID int64, customerID int64, planPrice float64, days int, trafficGB int) error {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer not found")
	}
	if customer.Balance < planPrice {
		return fmt.Errorf("insufficient balance: have %.2f, need %.2f", customer.Balance, planPrice)
	}

	// Ownership check: make sure the key belongs to this customer.
	key, err := s.subKeyRepo.FindByID(ctx, keyID)
	if err != nil {
		return err
	}
	if key == nil || key.CustomerID != customerID {
		return fmt.Errorf("key %d not found or not owned by this customer", keyID)
	}

	// Delegate to CreatePurchase with ExtendKeyID set — this charges the wallet
	// and calls Remnawave ExtendUser on the specific key UUID.
	_, _, err = s.paymentService.CreatePurchaseWithExtend(ctx, planPrice, days, trafficGB, customer, keyID)
	return err
}
