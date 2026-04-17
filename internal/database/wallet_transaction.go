package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type WalletTransactionType string

const (
	WalletTransactionTypeTopup    WalletTransactionType = "topup"
	WalletTransactionTypePurchase WalletTransactionType = "purchase"
	WalletTransactionTypeRefund   WalletTransactionType = "refund"
	WalletTransactionTypeReferral WalletTransactionType = "referral"
)

type WalletTransaction struct {
	ID          int64                 `db:"id" json:"id"`
	CustomerID  int64                 `db:"customer_id" json:"customer_id"`
	Amount      float64               `db:"amount" json:"amount"`
	Type        WalletTransactionType `db:"type" json:"type"`
	PurchaseID  *int64                `db:"purchase_id" json:"purchase_id"`
	Description string                `db:"description" json:"description"`
	CreatedAt   time.Time             `db:"created_at" json:"created_at"`
}

type WalletTransactionRepository struct {
	pool *pgxpool.Pool
}

func NewWalletTransactionRepository(pool *pgxpool.Pool) *WalletTransactionRepository {
	return &WalletTransactionRepository{pool: pool}
}

func (r *WalletTransactionRepository) Create(ctx context.Context, tx *WalletTransaction) (int64, error) {
	buildInsert := sq.Insert("wallet_transaction").
		Columns("customer_id", "amount", "type", "purchase_id", "description").
		Values(tx.CustomerID, tx.Amount, tx.Type, tx.PurchaseID, tx.Description).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build insert query: %w", err)
	}

	var id int64
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create wallet transaction: %w", err)
	}

	return id, nil
}

// CreateTx inserts a wallet_transaction row inside an existing pgx.Tx.
// Use this to keep balance updates and transaction log atomic.
func (r *WalletTransactionRepository) CreateTx(ctx context.Context, pgxTx pgx.Tx, tx *WalletTransaction) (int64, error) {
	buildInsert := sq.Insert("wallet_transaction").
		Columns("customer_id", "amount", "type", "purchase_id", "description").
		Values(tx.CustomerID, tx.Amount, tx.Type, tx.PurchaseID, tx.Description).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildInsert.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build insert query: %w", err)
	}

	var id int64
	err = pgxTx.QueryRow(ctx, sqlStr, args...).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create wallet transaction in tx: %w", err)
	}

	return id, nil
}

func (r *WalletTransactionRepository) FindByCustomerID(ctx context.Context, customerID int64, limit int) ([]WalletTransaction, error) {
	buildSelect := sq.Select("id", "customer_id", "amount", "type", "purchase_id", "description", "created_at").
		From("wallet_transaction").
		Where(sq.Eq{"customer_id": customerID}).
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query wallet transactions: %w", err)
	}
	defer rows.Close()

	var transactions []WalletTransaction
	for rows.Next() {
		var tx WalletTransaction
		err := rows.Scan(
			&tx.ID,
			&tx.CustomerID,
			&tx.Amount,
			&tx.Type,
			&tx.PurchaseID,
			&tx.Description,
			&tx.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan wallet transaction: %w", err)
		}
		transactions = append(transactions, tx)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating wallet transactions: %w", err)
	}

	return transactions, nil
}

func (r *WalletTransactionRepository) SumByCustomerTypeAndDescription(ctx context.Context, customerID int64, txType WalletTransactionType, description string) (float64, error) {
	buildSelect := sq.Select("COALESCE(SUM(amount), 0)").
		From("wallet_transaction").
		Where(sq.Eq{
			"customer_id": customerID,
			"type":        txType,
			"description": description,
		}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build sum query: %w", err)
	}

	var total sql.NullFloat64
	if err := r.pool.QueryRow(ctx, sqlStr, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to sum wallet transactions: %w", err)
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Float64, nil
}
