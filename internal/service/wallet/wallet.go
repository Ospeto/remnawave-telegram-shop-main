package wallet

import (
	"context"
	"errors"
	"fmt"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

type WalletService struct {
	paymentService *payment.PaymentService
	customerRepo   *database.CustomerRepository
	subKeyRepo     *database.SubscriptionKeyRepository
	walletTxRepo   *database.WalletTransactionRepository
}

var ErrAutoRenewPlanUnknown = errors.New("auto-renew plan is not configured for this key yet")

func NewWalletService(
	paymentService *payment.PaymentService,
	customerRepo *database.CustomerRepository,
	subKeyRepo *database.SubscriptionKeyRepository,
	walletTxRepo *database.WalletTransactionRepository,
) *WalletService {
	return &WalletService{
		paymentService: paymentService,
		customerRepo:   customerRepo,
		subKeyRepo:     subKeyRepo,
		walletTxRepo:   walletTxRepo,
	}
}

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

// SetKeyAutoRenew toggles auto-renew for a specific subscription key.
// Ownership is validated inside the repository (customerID must match key.customer_id).
func (s *WalletService) SetKeyAutoRenew(ctx context.Context, keyID int64, customerID int64, enabled bool) error {
	if enabled {
		key, err := s.subKeyRepo.FindByID(ctx, keyID)
		if err != nil {
			return err
		}
		if key == nil || key.CustomerID != customerID {
			return fmt.Errorf("key %d not found or not owned by this customer", keyID)
		}
		if key.AutoRenewPlanDays == nil || *key.AutoRenewPlanDays <= 0 || key.AutoRenewPlanTraffic == nil {
			return ErrAutoRenewPlanUnknown
		}
	}
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
	_, _, err = s.paymentService.CreatePurchaseWithExtend(ctx, planPrice, days, trafficGB, customer, keyID, "")
	return err
}
