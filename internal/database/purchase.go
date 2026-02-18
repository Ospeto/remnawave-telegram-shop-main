package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type InvoiceType string

const (
	InvoiceTypeCrypto        InvoiceType = "crypto"
	InvoiceTypeMobileBanking InvoiceType = "mobile_banking"
	InvoiceTypeWalletTopUp   InvoiceType = "wallet_topup"
	InvoiceTypeWalletPayment InvoiceType = "wallet_payment"
)

type PurchaseStatus string

const (
	PurchaseStatusNew     PurchaseStatus = "new"
	PurchaseStatusPending PurchaseStatus = "pending"
	PurchaseStatusPaid    PurchaseStatus = "paid"
	PurchaseStatusCancel  PurchaseStatus = "cancel"
)

type Purchase struct {
	ID                int64          `db:"id"`
	Amount            float64        `db:"amount"`
	CustomerID        int64          `db:"customer_id"`
	CreatedAt         time.Time      `db:"created_at"`
	Month             int            `db:"month"`
	PaidAt            *time.Time     `db:"paid_at"`
	Currency          string         `db:"currency"`
	ExpireAt          *time.Time     `db:"expire_at"`
	Status            PurchaseStatus `db:"status"`
	InvoiceType       InvoiceType    `db:"invoice_type"`
	CryptoInvoiceID   *int64         `db:"crypto_invoice_id"`
	CryptoInvoiceLink *string        `db:"crypto_invoice_url"`
	YookasaURL        *string        `db:"yookasa_url"`
	YookasaID         *uuid.UUID     `db:"yookasa_id"`
	TrafficLimitGB    int            `db:"traffic_limit_gb"`
	Days              int            `db:"days"`
	ExtendKeyID       *int64         `db:"extend_key_id"`
	PlanLabel         string         `db:"plan_label"`
	PaymentMethod     string         `db:"payment_method"`
	PaymentPhone      string         `db:"payment_phone"`
	VerifiedAt        *time.Time     `db:"verified_at"`
	TransactionID     string         `db:"transaction_id"`
	PromoCodeID       *int64         `db:"promo_code_id"`
}

type PurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewPurchaseRepository(pool *pgxpool.Pool) *PurchaseRepository {
	return &PurchaseRepository{
		pool: pool,
	}
}

func (cr *PurchaseRepository) Create(ctx context.Context, purchase *Purchase) (int64, error) {
	buildInsert := sq.Insert("purchase").
		Columns("amount", "customer_id", "month", "currency", "expire_at", "status", "invoice_type", "crypto_invoice_id", "crypto_invoice_url", "yookasa_url", "yookasa_id", "traffic_limit_gb", "days", "extend_key_id", "promo_code_id").
		Values(purchase.Amount, purchase.CustomerID, purchase.Month, purchase.Currency, purchase.ExpireAt, purchase.Status, purchase.InvoiceType, purchase.CryptoInvoiceID, purchase.CryptoInvoiceLink, purchase.YookasaURL, purchase.YookasaID, purchase.TrafficLimitGB, purchase.Days, purchase.ExtendKeyID, purchase.PromoCodeID).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = cr.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

var purchaseColumns = []string{
	"id", "amount", "customer_id", "created_at", "month",
	"paid_at", "currency", "expire_at", "status", "invoice_type",
	"crypto_invoice_id", "crypto_invoice_url", "yookasa_url", "yookasa_id",
	"traffic_limit_gb", "days", "extend_key_id",
	"plan_label", "payment_method", "payment_phone", "verified_at", "transaction_id", "promo_code_id",
}

func scanPurchase(row pgx.Row) (*Purchase, error) {
	p := &Purchase{}
	err := row.Scan(
		&p.ID, &p.Amount, &p.CustomerID, &p.CreatedAt, &p.Month,
		&p.PaidAt, &p.Currency, &p.ExpireAt, &p.Status, &p.InvoiceType,
		&p.CryptoInvoiceID, &p.CryptoInvoiceLink, &p.YookasaURL, &p.YookasaID,
		&p.TrafficLimitGB, &p.Days, &p.ExtendKeyID,
		&p.PlanLabel, &p.PaymentMethod, &p.PaymentPhone, &p.VerifiedAt, &p.TransactionID, &p.PromoCodeID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan purchase: %w", err)
	}
	return p, nil
}

func (cr *PurchaseRepository) FindByInvoiceTypeAndStatus(ctx context.Context, invoiceType InvoiceType, status PurchaseStatus) (*[]Purchase, error) {
	buildSelect := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": invoiceType},
			sq.Eq{"status": status},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	purchases := []Purchase{}
	for rows.Next() {
		p := Purchase{}
		err = rows.Scan(
			&p.ID, &p.Amount, &p.CustomerID, &p.CreatedAt, &p.Month,
			&p.PaidAt, &p.Currency, &p.ExpireAt, &p.Status, &p.InvoiceType,
			&p.CryptoInvoiceID, &p.CryptoInvoiceLink, &p.YookasaURL, &p.YookasaID,
			&p.TrafficLimitGB, &p.Days, &p.ExtendKeyID,
			&p.PlanLabel, &p.PaymentMethod, &p.PaymentPhone, &p.VerifiedAt, &p.TransactionID, &p.PromoCodeID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &purchases, nil
}

func (cr *PurchaseRepository) FindById(ctx context.Context, id int64) (*Purchase, error) {
	buildSelect := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	return scanPurchase(cr.pool.QueryRow(ctx, sql, args...))
}

// allowedPurchaseFields is a whitelist of columns that can be updated via UpdateFields.
var allowedPurchaseFields = map[string]bool{
	"status": true, "paid_at": true, "crypto_invoice_url": true,
	"crypto_invoice_id": true, "plan_label": true, "payment_phone": true,
	"extend_key_id": true, "transaction_id": true, "payment_method": true,
	"verified_at": true,
}

func (p *PurchaseRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("purchase").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		if !allowedPurchaseFields[field] {
			return fmt.Errorf("disallowed field in purchase update: %s", field)
		}
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update purchase: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no purchase found with id: %d", id)
	}

	return nil
}

func (pr *PurchaseRepository) MarkAsPaid(ctx context.Context, purchaseID int64) error {
	currentTime := time.Now()

	updates := map[string]interface{}{
		"status":  PurchaseStatusPaid,
		"paid_at": currentTime,
	}

	return pr.UpdateFields(ctx, purchaseID, updates)
}

func (pr *PurchaseRepository) FindSuccessfulPaidPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	query := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
			sq.Eq{"invoice_type": InvoiceTypeCrypto},
		}).
		OrderBy("paid_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	return scanPurchase(pr.pool.QueryRow(ctx, sql, args...))
}

// RevenueSummaryRow represents a single row from the revenue_daily view.
type RevenueSummaryRow struct {
	Day             string  `json:"day"`
	PaymentMethod   string  `json:"payment_method"`
	Currency        string  `json:"currency"`
	TotalPurchases  int     `json:"total_purchases"`
	TotalRevenue    float64 `json:"total_revenue"`
	UniqueCustomers int     `json:"unique_customers"`
}

// GetRevenueSummary fetches revenue data for the last N days from the revenue_daily view.
func (pr *PurchaseRepository) GetRevenueSummary(ctx context.Context, days int) ([]RevenueSummaryRow, error) {
	query := `SELECT day, COALESCE(payment_method, '') as payment_method, COALESCE(currency, '') as currency, total_purchases, total_revenue, unique_customers
		FROM revenue_daily
		WHERE day >= CURRENT_DATE - $1::int
		ORDER BY day DESC, total_revenue DESC`

	rows, err := pr.pool.Query(ctx, query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query revenue summary: %w", err)
	}
	defer rows.Close()

	var result []RevenueSummaryRow
	for rows.Next() {
		var r RevenueSummaryRow
		var dayTime time.Time
		if err := rows.Scan(&dayTime, &r.PaymentMethod, &r.Currency, &r.TotalPurchases, &r.TotalRevenue, &r.UniqueCustomers); err != nil {
			return nil, fmt.Errorf("failed to scan revenue row: %w", err)
		}
		r.Day = dayTime.Format("2006-01-02")
		result = append(result, r)
	}
	return result, rows.Err()
}

// RecentPaidRow holds a paid purchase with the customer's Telegram ID.
type RecentPaidRow struct {
	PurchaseID    int64
	TelegramID    int64
	Amount        float64
	Currency      string
	PlanLabel     string
	PaymentMethod string
	PaidAt        time.Time
}

// FindRecentPaid returns the most recently paid purchases, up to limit rows.
func (pr *PurchaseRepository) FindRecentPaid(ctx context.Context, limit int) ([]RecentPaidRow, error) {
	query := `
		SELECT p.id, c.telegram_id, p.amount, p.currency, p.plan_label, p.payment_method, p.paid_at
		FROM purchase p
		JOIN customer c ON c.id = p.customer_id
		WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
		ORDER BY p.paid_at DESC
		LIMIT $1`

	rows, err := pr.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent paid purchases: %w", err)
	}
	defer rows.Close()

	var result []RecentPaidRow
	for rows.Next() {
		var r RecentPaidRow
		if err := rows.Scan(&r.PurchaseID, &r.TelegramID, &r.Amount, &r.Currency, &r.PlanLabel, &r.PaymentMethod, &r.PaidAt); err != nil {
			return nil, fmt.Errorf("failed to scan recent paid row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
