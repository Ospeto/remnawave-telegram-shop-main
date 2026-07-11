package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/config"

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
	PurchaseStatusNew        PurchaseStatus = "new"
	PurchaseStatusPending    PurchaseStatus = "pending"
	PurchaseStatusProcessing PurchaseStatus = "processing"
	PurchaseStatusPaid       PurchaseStatus = "paid"
	PurchaseStatusCancel     PurchaseStatus = "cancel"
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
	IdempotencyKey    *uuid.UUID     `db:"idempotency_key"`
}

type PurchaseRepository struct {
	pool *pgxpool.Pool
}

const promoReservationHoldWindow = 24 * time.Hour

func NewPurchaseRepository(pool *pgxpool.Pool) *PurchaseRepository {
	return &PurchaseRepository{
		pool: pool,
	}
}

func buildPurchaseInsert(purchase *Purchase) sq.InsertBuilder {
	return sq.Insert("purchase").
		Columns("amount", "customer_id", "month", "currency", "expire_at", "status", "invoice_type", "crypto_invoice_id", "crypto_invoice_url", "yookasa_url", "yookasa_id", "traffic_limit_gb", "days", "extend_key_id", "promo_code_id", "idempotency_key").
		Values(purchase.Amount, purchase.CustomerID, purchase.Month, purchase.Currency, purchase.ExpireAt, purchase.Status, purchase.InvoiceType, purchase.CryptoInvoiceID, purchase.CryptoInvoiceLink, purchase.YookasaURL, purchase.YookasaID, purchase.TrafficLimitGB, purchase.Days, purchase.ExtendKeyID, purchase.PromoCodeID, purchase.IdempotencyKey).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)
}

func (cr *PurchaseRepository) Create(ctx context.Context, purchase *Purchase) (int64, error) {
	sql, args, err := buildPurchaseInsert(purchase).ToSql()
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

func (cr *PurchaseRepository) CreateTx(ctx context.Context, tx pgx.Tx, purchase *Purchase) (int64, error) {
	sql, args, err := buildPurchaseInsert(purchase).ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = tx.QueryRow(ctx, sql, args...).Scan(&id)
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
	"plan_label", "payment_method", "payment_phone", "verified_at", "transaction_id", "promo_code_id", "idempotency_key",
}

// scanPurchaseRow scans a single pgx.Row into a Purchase.
func scanPurchase(row pgx.Row) (*Purchase, error) {
	p := &Purchase{}
	err := row.Scan(
		&p.ID, &p.Amount, &p.CustomerID, &p.CreatedAt, &p.Month,
		&p.PaidAt, &p.Currency, &p.ExpireAt, &p.Status, &p.InvoiceType,
		&p.CryptoInvoiceID, &p.CryptoInvoiceLink, &p.YookasaURL, &p.YookasaID,
		&p.TrafficLimitGB, &p.Days, &p.ExtendKeyID,
		&p.PlanLabel, &p.PaymentMethod, &p.PaymentPhone, &p.VerifiedAt, &p.TransactionID, &p.PromoCodeID, &p.IdempotencyKey,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan purchase: %w", err)
	}
	return p, nil
}

// scanPurchaseRow scans the current row from a pgx.Rows iterator into a Purchase.
// Use this in multi-row loops to avoid duplicating field-order across callers.
func scanPurchaseRow(rows pgx.Rows) (*Purchase, error) {
	p := &Purchase{}
	err := rows.Scan(
		&p.ID, &p.Amount, &p.CustomerID, &p.CreatedAt, &p.Month,
		&p.PaidAt, &p.Currency, &p.ExpireAt, &p.Status, &p.InvoiceType,
		&p.CryptoInvoiceID, &p.CryptoInvoiceLink, &p.YookasaURL, &p.YookasaID,
		&p.TrafficLimitGB, &p.Days, &p.ExtendKeyID,
		&p.PlanLabel, &p.PaymentMethod, &p.PaymentPhone, &p.VerifiedAt, &p.TransactionID, &p.PromoCodeID, &p.IdempotencyKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan purchase row: %w", err)
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
		p, err := scanPurchaseRow(rows)
		if err != nil {
			return nil, err
		}
		purchases = append(purchases, *p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &purchases, nil
}

func (cr *PurchaseRepository) FindLatestAwaitingVerificationByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	buildSelect := buildLatestAwaitingVerificationByCustomerSelect(customerID, time.Now())

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	return scanPurchase(cr.pool.QueryRow(ctx, sql, args...))
}

func (cr *PurchaseRepository) FindLatestAwaitingVerificationByCustomerTx(ctx context.Context, tx pgx.Tx, customerID int64) (*Purchase, error) {
	buildSelect := buildLatestAwaitingVerificationByCustomerSelect(customerID, time.Now())

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	return scanPurchase(tx.QueryRow(ctx, sql, args...))
}

func buildLatestAwaitingVerificationByCustomerSelect(customerID int64, now time.Time) sq.SelectBuilder {
	return sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"invoice_type": []InvoiceType{InvoiceTypeMobileBanking, InvoiceTypeWalletTopUp}},
			sq.Eq{"status": []PurchaseStatus{PurchaseStatusPending, PurchaseStatusNew}},
			sq.GtOrEq{"created_at": now.Add(-promoReservationHoldWindow)},
		}).
		OrderBy("created_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)
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

func (cr *PurchaseRepository) FindByIdempotencyKey(ctx context.Context, key uuid.UUID) (*Purchase, error) {
	buildSelect := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.Eq{"idempotency_key": key}).
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

// CancelAwaitingVerification marks a customer's screenshot-verification purchase as cancelled.
// The status predicate keeps a late cancel from overwriting a purchase that is already processing or paid.
func (pr *PurchaseRepository) CancelAwaitingVerification(ctx context.Context, purchaseID, customerID int64) (bool, error) {
	query := `
		UPDATE purchase
		SET status = $1, paid_at = NULL
		WHERE id = $2
		  AND customer_id = $3
		  AND invoice_type IN ($4, $5)
		  AND status IN ($6, $7)
	`

	tag, err := pr.pool.Exec(
		ctx,
		query,
		PurchaseStatusCancel,
		purchaseID,
		customerID,
		InvoiceTypeMobileBanking,
		InvoiceTypeWalletTopUp,
		PurchaseStatusNew,
		PurchaseStatusPending,
	)
	if err != nil {
		return false, fmt.Errorf("failed to cancel purchase %d: %w", purchaseID, err)
	}

	return tag.RowsAffected() > 0, nil
}

// TryMarkAsProcessing atomically claims a purchase for fulfillment.
// It returns false when another worker already won the race or the purchase
// is already in a terminal or in-flight state.
func (pr *PurchaseRepository) TryMarkAsProcessing(ctx context.Context, purchaseID int64) (bool, error) {
	query := `
		UPDATE purchase
		SET status = $1
		WHERE id = $2 AND status IN ($3, $4)
	`

	tag, err := pr.pool.Exec(ctx, query, PurchaseStatusProcessing, purchaseID, PurchaseStatusNew, PurchaseStatusPending)
	if err != nil {
		return false, fmt.Errorf("failed to claim purchase %d: %w", purchaseID, err)
	}

	return tag.RowsAffected() > 0, nil
}

func (pr *PurchaseRepository) MarkAsPaid(ctx context.Context, purchaseID int64) error {
	now := time.Now()

	query := `
		UPDATE purchase
		SET status = $1, paid_at = $2
		WHERE id = $3 AND status = $4
	`

	tag, err := pr.pool.Exec(ctx, query, PurchaseStatusPaid, now, purchaseID, PurchaseStatusProcessing)
	if err != nil {
		return fmt.Errorf("failed to mark purchase %d as paid: %w", purchaseID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("purchase %d was not in processing state", purchaseID)
	}

	return nil
}

// FindSuccessfulPaidPurchaseByCustomer returns the most recent paid purchase for
// the customer regardless of invoice type, used to determine referral eligibility.
// Previously filtered to crypto-only which silently excluded wallet/mobile users.
func (cr *PurchaseRepository) FindSuccessfulPaidPurchaseByCustomer(ctx context.Context, customerID int64) (*Purchase, error) {
	query := sq.Select(purchaseColumns...).
		From("purchase").
		Where(sq.And{
			sq.Eq{"customer_id": customerID},
			sq.Eq{"status": PurchaseStatusPaid},
		}).
		OrderBy("paid_at DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	return scanPurchase(cr.pool.QueryRow(ctx, sql, args...))
}

type RevenueSummaryPeriod string

const (
	RevenuePeriodDay    RevenueSummaryPeriod = "day"
	RevenuePeriodWeek   RevenueSummaryPeriod = "week"
	RevenuePeriodMonth  RevenueSummaryPeriod = "month"
	RevenuePeriodYear   RevenueSummaryPeriod = "year"
	RevenuePeriodCustom RevenueSummaryPeriod = "custom"
)

func NormalizeRevenueSummaryPeriod(period string) (RevenueSummaryPeriod, error) {
	switch RevenueSummaryPeriod(period) {
	case "", RevenuePeriodDay:
		return RevenuePeriodDay, nil
	case RevenuePeriodWeek:
		return RevenuePeriodWeek, nil
	case RevenuePeriodMonth:
		return RevenuePeriodMonth, nil
	case RevenuePeriodYear:
		return RevenuePeriodYear, nil
	case RevenuePeriodCustom:
		return RevenuePeriodCustom, nil
	default:
		return "", fmt.Errorf("unsupported revenue period: %s", period)
	}
}

// RevenueSummaryRow represents a single breakdown row from the revenue summary query.
// TotalRevenue intentionally means service revenue, not wallet cash-in. Wallet top-ups
// are cash collected and wallet liability movement, then wallet spends are counted when
// the service is delivered.
type RevenueSummaryRow struct {
	Day                        string  `json:"day"`
	Period                     string  `json:"period"`
	PeriodStart                string  `json:"period_start"`
	PaymentMethod              string  `json:"payment_method"`
	InvoiceType                string  `json:"invoice_type"`
	RevenueCategory            string  `json:"revenue_category"`
	Currency                   string  `json:"currency"`
	TotalPurchases             int     `json:"total_purchases"`
	TotalRevenue               float64 `json:"total_revenue"`
	UniqueCustomers            int     `json:"unique_customers"`
	CashCollected              float64 `json:"cash_collected"`
	WalletTopUps               float64 `json:"wallet_topups"`
	WalletSpend                float64 `json:"wallet_spend"`
	ServiceRevenue             float64 `json:"service_revenue"`
	NewKeyPurchases            int     `json:"new_key_purchases"`
	ExtensionPurchases         int     `json:"extension_purchases"`
	WalletTopUpPurchases       int     `json:"wallet_topup_purchases"`
	PeriodTotalPurchases       int     `json:"period_total_purchases"`
	PeriodServicePurchases     int     `json:"period_service_purchases"`
	PeriodUniqueCustomers      int     `json:"period_unique_customers"`
	PeriodCashCollected        float64 `json:"period_cash_collected"`
	PeriodWalletTopUps         float64 `json:"period_wallet_topups"`
	PeriodWalletSpend          float64 `json:"period_wallet_spend"`
	PeriodServiceRevenue       float64 `json:"period_service_revenue"`
	PeriodNewKeyPurchases      int     `json:"period_new_key_purchases"`
	PeriodExtensionPurchases   int     `json:"period_extension_purchases"`
	PeriodWalletTopUpPurchases int     `json:"period_wallet_topup_purchases"`
}

const revenueSummaryTimezone = "Asia/Yangon"

func revenuePeriodExpression(period RevenueSummaryPeriod) (string, error) {
	switch period {
	case RevenuePeriodDay, RevenuePeriodCustom:
		return fmt.Sprintf("(p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	case RevenuePeriodWeek:
		return fmt.Sprintf("DATE_TRUNC('week', p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	case RevenuePeriodMonth:
		return fmt.Sprintf("DATE_TRUNC('month', p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	case RevenuePeriodYear:
		return fmt.Sprintf("DATE_TRUNC('year', p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	default:
		return "", fmt.Errorf("unsupported revenue period: %s", period)
	}
}

func InclusiveYangonDateRangeToHalfOpen(from, to time.Time) (time.Time, time.Time, error) {
	loc := revenueSummaryLocation()
	start := time.Date(from.In(loc).Year(), from.In(loc).Month(), from.In(loc).Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(to.In(loc).Year(), to.In(loc).Month(), to.In(loc).Day(), 0, 0, 0, 0, loc)
	if endDay.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("to must be on or after from")
	}
	return start, endDay.AddDate(0, 0, 1), nil
}

func buildRevenueSummaryQuery(period RevenueSummaryPeriod) (string, error) {
	periodExpr, err := revenuePeriodExpression(period)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`
		WITH paid AS (
			SELECT
				p.id,
				p.customer_id,
				p.amount,
				COALESCE(NULLIF(p.currency, ''), 'MMK') AS currency,
				COALESCE(
					NULLIF(p.payment_method, ''),
					CASE
						WHEN p.invoice_type = 'wallet_payment' THEN 'wallet'
						ELSE 'unknown'
					END
				) AS payment_method,
				p.invoice_type,
				p.extend_key_id,
				%s AS period_start,
				CASE
					WHEN p.invoice_type = 'wallet_topup' THEN 'wallet_topup'
					WHEN p.extend_key_id IS NOT NULL THEN 'extension'
					ELSE 'new_key'
				END AS revenue_category
			FROM purchase p
			JOIN customer c ON c.id = p.customer_id
			WHERE p.status = 'paid'
			  AND p.paid_at IS NOT NULL
			  AND p.paid_at >= $1
			  AND p.paid_at < $2
			  AND ($3::bigint = 0 OR c.telegram_id <> $3)
		),
		period_totals AS (
			SELECT
				period_start,
				currency,
				COUNT(*) AS period_total_purchases,
				COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup') AS period_service_purchases,
				COUNT(DISTINCT customer_id) AS period_unique_customers,
				COALESCE(SUM(CASE WHEN invoice_type IN ('crypto', 'mobile_banking', 'wallet_topup') THEN amount ELSE 0 END), 0) AS period_cash_collected,
				COALESCE(SUM(CASE WHEN invoice_type = 'wallet_topup' THEN amount ELSE 0 END), 0) AS period_wallet_topups,
				COALESCE(SUM(CASE WHEN invoice_type = 'wallet_payment' THEN amount ELSE 0 END), 0) AS period_wallet_spend,
				COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS period_service_revenue,
				COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NULL) AS period_new_key_purchases,
				COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NOT NULL) AS period_extension_purchases,
				COUNT(*) FILTER (WHERE invoice_type = 'wallet_topup') AS period_wallet_topup_purchases
			FROM paid
			GROUP BY 1, 2
		),
		breakdown AS (
			SELECT
				period_start,
				payment_method,
				invoice_type,
				revenue_category,
				currency,
				COUNT(*) AS total_purchases,
				COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS total_revenue,
				COUNT(DISTINCT customer_id) AS unique_customers,
				COALESCE(SUM(CASE WHEN invoice_type IN ('crypto', 'mobile_banking', 'wallet_topup') THEN amount ELSE 0 END), 0) AS cash_collected,
				COALESCE(SUM(CASE WHEN invoice_type = 'wallet_topup' THEN amount ELSE 0 END), 0) AS wallet_topups,
				COALESCE(SUM(CASE WHEN invoice_type = 'wallet_payment' THEN amount ELSE 0 END), 0) AS wallet_spend,
				COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS service_revenue,
				COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NULL) AS new_key_purchases,
				COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NOT NULL) AS extension_purchases,
				COUNT(*) FILTER (WHERE invoice_type = 'wallet_topup') AS wallet_topup_purchases
			FROM paid
			GROUP BY 1, 2, 3, 4, 5
		)
		SELECT
			b.period_start,
			b.payment_method,
			b.invoice_type,
			b.revenue_category,
			b.currency,
			b.total_purchases,
			b.total_revenue,
			b.unique_customers,
			b.cash_collected,
			b.wallet_topups,
			b.wallet_spend,
			b.service_revenue,
			b.new_key_purchases,
			b.extension_purchases,
			b.wallet_topup_purchases,
			pt.period_total_purchases,
			pt.period_service_purchases,
			pt.period_unique_customers,
			pt.period_cash_collected,
			pt.period_wallet_topups,
			pt.period_wallet_spend,
			pt.period_service_revenue,
			pt.period_new_key_purchases,
			pt.period_extension_purchases,
			pt.period_wallet_topup_purchases
		FROM breakdown b
		JOIN period_totals pt ON pt.period_start = b.period_start AND pt.currency = b.currency
		ORDER BY b.period_start DESC, b.service_revenue DESC, b.cash_collected DESC`, periodExpr), nil
}

// GetRevenueSummary fetches revenue data for the last N days, excluding admin-account purchases.
func (pr *PurchaseRepository) GetRevenueSummary(ctx context.Context, days int) ([]RevenueSummaryRow, error) {
	if days <= 0 {
		days = 1
	}
	location := revenueSummaryLocation()
	end := startOfRevenueDay(time.Now().In(location)).AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -days)

	return pr.GetRevenueSummaryRange(ctx, start, end, RevenuePeriodDay)
}

func revenueSummaryLocation() *time.Location {
	location, err := time.LoadLocation(revenueSummaryTimezone)
	if err != nil {
		return time.FixedZone("MMT", 6*3600+30*60)
	}
	return location
}

func startOfRevenueDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (pr *PurchaseRepository) GetRevenueSummaryForPeriods(ctx context.Context, period RevenueSummaryPeriod, periods int) ([]RevenueSummaryRow, error) {
	if periods <= 0 {
		periods = 1
	}
	location := revenueSummaryLocation()
	now := time.Now().In(location)
	var start, end time.Time

	switch period {
	case RevenuePeriodDay:
		end = startOfRevenueDay(now).AddDate(0, 0, 1)
		start = end.AddDate(0, 0, -periods)
	case RevenuePeriodWeek:
		end = startOfRevenueWeek(now).AddDate(0, 0, 7)
		start = end.AddDate(0, 0, -7*periods)
	case RevenuePeriodMonth:
		end = startOfRevenueMonth(now).AddDate(0, 1, 0)
		start = end.AddDate(0, -periods, 0)
	case RevenuePeriodYear:
		end = startOfRevenueYear(now).AddDate(1, 0, 0)
		start = end.AddDate(-periods, 0, 0)
	default:
		return nil, fmt.Errorf("unsupported revenue period: %s", period)
	}

	return pr.GetRevenueSummaryRange(ctx, start, end, period)
}

func startOfRevenueWeek(t time.Time) time.Time {
	day := startOfRevenueDay(t)
	daysSinceMonday := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -daysSinceMonday)
}

func startOfRevenueMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func startOfRevenueYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}

func (pr *PurchaseRepository) GetRevenueSummaryRange(ctx context.Context, start, end time.Time, period RevenueSummaryPeriod) ([]RevenueSummaryRow, error) {
	query, err := buildRevenueSummaryQuery(period)
	if err != nil {
		return nil, err
	}
	adminTelegramID := config.GetAdminTelegramId()

	rows, err := pr.pool.Query(ctx, query, start, end, adminTelegramID)
	if err != nil {
		return nil, fmt.Errorf("failed to query revenue summary: %w", err)
	}
	defer rows.Close()

	var result []RevenueSummaryRow
	for rows.Next() {
		var r RevenueSummaryRow
		var periodStart time.Time
		if err := rows.Scan(
			&periodStart,
			&r.PaymentMethod,
			&r.InvoiceType,
			&r.RevenueCategory,
			&r.Currency,
			&r.TotalPurchases,
			&r.TotalRevenue,
			&r.UniqueCustomers,
			&r.CashCollected,
			&r.WalletTopUps,
			&r.WalletSpend,
			&r.ServiceRevenue,
			&r.NewKeyPurchases,
			&r.ExtensionPurchases,
			&r.WalletTopUpPurchases,
			&r.PeriodTotalPurchases,
			&r.PeriodServicePurchases,
			&r.PeriodUniqueCustomers,
			&r.PeriodCashCollected,
			&r.PeriodWalletTopUps,
			&r.PeriodWalletSpend,
			&r.PeriodServiceRevenue,
			&r.PeriodNewKeyPurchases,
			&r.PeriodExtensionPurchases,
			&r.PeriodWalletTopUpPurchases,
		); err != nil {
			return nil, fmt.Errorf("failed to scan revenue row: %w", err)
		}
		r.Period = string(period)
		r.PeriodStart = periodStart.Format("2006-01-02")
		r.Day = r.PeriodStart
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
