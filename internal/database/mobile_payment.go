package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type MobilePaymentVerification struct {
	ID              int64     `db:"id"`
	PurchaseID      int64     `db:"purchase_id"`
	TransactionID   string    `db:"transaction_id"`
	Provider        string    `db:"provider"`
	PhoneNumber     string    `db:"phone_number"`
	Amount          float64   `db:"amount"`
	Note            string    `db:"note"`
	Verified        bool      `db:"verified"`
	RejectionReason string    `db:"rejection_reason"`
	CreatedAt       time.Time `db:"created_at"`
}

type MobilePaymentRepository struct {
	pool *pgxpool.Pool
}

func NewMobilePaymentRepository(pool *pgxpool.Pool) *MobilePaymentRepository {
	return &MobilePaymentRepository{pool: pool}
}

func (r *MobilePaymentRepository) Create(ctx context.Context, record *MobilePaymentVerification) (int64, error) {
	builder := sq.Insert("mobile_payment_verification").
		Columns("purchase_id", "transaction_id", "provider", "phone_number", "amount", "note", "verified", "rejection_reason").
		Values(record.PurchaseID, record.TransactionID, record.Provider, record.PhoneNumber, record.Amount, record.Note, record.Verified, record.RejectionReason).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return 0, fmt.Errorf("build insert: %w", err)
	}

	var id int64
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert mobile_payment_verification: %w", err)
	}

	return id, nil
}

func (r *MobilePaymentRepository) ExistsByTransactionID(ctx context.Context, txnID string) (bool, error) {
	builder := sq.Select("1").
		From("mobile_payment_verification").
		Where(sq.Eq{"transaction_id": txnID}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	var dummy int
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query mobile_payment_verification: %w", err)
	}

	return true, nil
}

func (r *MobilePaymentRepository) DeleteByTransactionID(ctx context.Context, txnID string) error {
	builder := sq.Delete("mobile_payment_verification").
		Where(sq.Eq{"transaction_id": txnID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build delete query: %w", err)
	}

	if _, err := r.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("delete mobile_payment_verification: %w", err)
	}

	return nil
}

func (r *MobilePaymentRepository) FindByPurchaseID(ctx context.Context, purchaseID int64) (*MobilePaymentVerification, error) {
	builder := sq.Select("id", "purchase_id", "transaction_id", "provider", "phone_number", "amount", "note", "verified", "rejection_reason", "created_at").
		From("mobile_payment_verification").
		Where(sq.Eq{"purchase_id": purchaseID}).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	record := &MobilePaymentVerification{}
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&record.ID, &record.PurchaseID, &record.TransactionID, &record.Provider,
		&record.PhoneNumber, &record.Amount, &record.Note, &record.Verified,
		&record.RejectionReason, &record.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query mobile_payment_verification: %w", err)
	}

	return record, nil
}
