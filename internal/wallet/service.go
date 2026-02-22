package wallet

import (
	"context"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Service struct {
	pool         *pgxpool.Pool
	customerRepo *database.CustomerRepository
	walletTxRepo *database.WalletTransactionRepository
	purchaseRepo *database.PurchaseRepository
}

func NewService(
	pool *pgxpool.Pool,
	customerRepo *database.CustomerRepository,
	walletTxRepo *database.WalletTransactionRepository,
	purchaseRepo *database.PurchaseRepository,
) *Service {
	return &Service{
		pool:         pool,
		customerRepo: customerRepo,
		walletTxRepo: walletTxRepo,
		purchaseRepo: purchaseRepo,
	}
}

// GetBalance returns the current balance for a customer
func (s *Service) GetBalance(ctx context.Context, customerID int64) (float64, error) {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil {
		return 0, fmt.Errorf("failed to get customer: %w", err)
	}
	if customer == nil {
		return 0, fmt.Errorf("customer not found")
	}
	return customer.Balance, nil
}

// GetTransactionHistory returns the wallet transaction history for a customer
func (s *Service) GetTransactionHistory(ctx context.Context, customerID int64, limit int) ([]database.WalletTransaction, error) {
	return s.walletTxRepo.FindByCustomerID(ctx, customerID, limit)
}

// TopUp adds funds to the customer's wallet
func (s *Service) TopUp(ctx context.Context, customerID int64, amount float64, description string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	// Check minimum amount (lowest plan price)
	minAmount := s.getMinimumTopUpAmount()
	if amount < minAmount {
		return fmt.Errorf("minimum top-up amount is %.0f %s", minAmount, config.Currency())
	}

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update balance atomically
	_, err = tx.Exec(ctx, `
		UPDATE customer 
		SET balance = balance + $1 
		WHERE id = $2
	`, amount, customerID)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Create wallet transaction record
	walletTx := &database.WalletTransaction{
		CustomerID:  customerID,
		Amount:      amount,
		Type:        database.WalletTransactionTypeTopup,
		Description: description,
	}

	buildInsert := sq.Insert("wallet_transaction").
		Columns("customer_id", "amount", "type", "description").
		Values(walletTx.CustomerID, walletTx.Amount, walletTx.Type, walletTx.Description).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build transaction insert: %w", err)
	}

	_, err = tx.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to create transaction record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("Wallet topped up", "customer_id", customerID, "amount", amount)
	return nil
}

// DeductBalance deducts amount from customer's wallet
func (s *Service) DeductBalance(ctx context.Context, customerID int64, amount float64, purchaseID int64, description string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Deduct balance atomically with check
	result, err := tx.Exec(ctx, `
		UPDATE customer 
		SET balance = balance - $1 
		WHERE id = $2 AND balance >= $1
	`, amount, customerID)
	if err != nil {
		return fmt.Errorf("failed to deduct balance: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("insufficient balance")
	}

	// Create wallet transaction record
	walletTx := &database.WalletTransaction{
		CustomerID:  customerID,
		Amount:      -amount,
		Type:        database.WalletTransactionTypePurchase,
		PurchaseID:  &purchaseID,
		Description: description,
	}

	buildInsert := sq.Insert("wallet_transaction").
		Columns("customer_id", "amount", "type", "purchase_id", "description").
		Values(walletTx.CustomerID, walletTx.Amount, walletTx.Type, walletTx.PurchaseID, walletTx.Description).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build transaction insert: %w", err)
	}

	_, err = tx.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to create transaction record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slog.Info("Balance deducted", "customer_id", customerID, "amount", amount, "purchase_id", purchaseID)
	return nil
}

// HasSufficientBalance checks if customer has enough balance
func (s *Service) HasSufficientBalance(ctx context.Context, customerID int64, amount float64) (bool, error) {
	balance, err := s.GetBalance(ctx, customerID)
	if err != nil {
		return false, err
	}
	return balance >= amount, nil
}

// SetAutoRenew enables/disables auto-renewal for a customer
func (s *Service) SetAutoRenew(ctx context.Context, customerID int64, enabled bool, duration int) error {
	if duration <= 0 {
		duration = 30 // default
	}

	updates := map[string]interface{}{
		"auto_renew":          enabled,
		"auto_renew_duration": duration,
	}

	return s.customerRepo.UpdateFields(ctx, customerID, updates)
}

// GetAutoRenewStatus returns auto-renewal settings for a customer
func (s *Service) GetAutoRenewStatus(ctx context.Context, customerID int64) (enabled bool, duration int, err error) {
	customer, err := s.customerRepo.FindById(ctx, customerID)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get customer: %w", err)
	}
	if customer == nil {
		return false, 0, fmt.Errorf("customer not found")
	}
	return customer.AutoRenew, customer.AutoRenewDuration, nil
}

// getMinimumTopUpAmount returns the lowest plan price
func (s *Service) getMinimumTopUpAmount() float64 {
	plans := config.Plans()
	if len(plans) == 0 {
		return float64(config.LowestPlanPrice())
	}

	minPrice := plans[0].Price
	for _, plan := range plans {
		if plan.Price < minPrice {
			minPrice = plan.Price
		}
	}
	return float64(minPrice)
}
