package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/config"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type FinancialAdjustmentType string

const FinancialAdjustmentTypeRefund FinancialAdjustmentType = "refund"

// Year/custom period labels used by refund aggregation SQL bucketing.
// (Purchase revenue helpers currently normalize day/week/month only.)
const (
	RevenuePeriodYear   RevenueSummaryPeriod = "year"
	RevenuePeriodCustom RevenueSummaryPeriod = "custom"
)

type FinancialAdjustment struct {
	ID             int64                   `json:"id"`
	PurchaseID     *int64                  `json:"purchase_id,omitempty"`
	AdjustmentType FinancialAdjustmentType `json:"adjustment_type"`
	Amount         float64                 `json:"amount"`
	Currency       string                  `json:"currency"`
	EffectiveAt    time.Time               `json:"effective_at"`
	Reason         string                  `json:"reason"`
	ExternalRef    string                  `json:"external_ref"`
	CreatedBy      string                  `json:"created_by"`
	IdempotencyKey string                  `json:"idempotency_key"`
	CreatedAt      time.Time               `json:"created_at"`
}

type CreateFinancialAdjustmentInput struct {
	PurchaseID     *int64
	AdjustmentType FinancialAdjustmentType
	Amount         float64
	Currency       string
	EffectiveAt    time.Time
	Reason         string
	ExternalRef    string
	CreatedBy      string
	IdempotencyKey string
}

type RefundPeriodRow struct {
	PeriodStart string
	Currency    string
	RefundTotal float64
	RefundCount int
}

type FinancialAdjustmentRepository struct {
	pool *pgxpool.Pool
}

func NewFinancialAdjustmentRepository(pool *pgxpool.Pool) *FinancialAdjustmentRepository {
	return &FinancialAdjustmentRepository{pool: pool}
}

func normalizeAdjustmentAmount(amount float64) (float64, error) {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("amount must be positive")
	}
	scaled := amount * 100
	if scaled >= 0 {
		scaled = math.Floor(scaled + 0.5)
	} else {
		scaled = math.Ceil(scaled - 0.5)
	}
	out := scaled / 100
	if out <= 0 {
		return 0, fmt.Errorf("amount must be positive after rounding")
	}
	return out, nil
}

func validateCreateFinancialAdjustmentInput(in CreateFinancialAdjustmentInput) error {
	if in.AdjustmentType != FinancialAdjustmentTypeRefund {
		return fmt.Errorf("unsupported adjustment_type: %s", in.AdjustmentType)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	if strings.TrimSpace(in.CreatedBy) == "" {
		return fmt.Errorf("created_by is required")
	}
	if _, err := normalizeAdjustmentAmount(in.Amount); err != nil {
		return err
	}
	return nil
}

func buildCreateFinancialAdjustmentSQL() string {
	return `
		INSERT INTO financial_adjustment (
			purchase_id, adjustment_type, amount, currency, effective_at,
			reason, external_ref, created_by, idempotency_key
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, purchase_id, adjustment_type, amount, currency, effective_at,
		          reason, external_ref, created_by, idempotency_key, created_at`
}

func buildSumRefundsByPeriodSQL(period RevenueSummaryPeriod) (string, error) {
	var bucket string
	switch period {
	case RevenuePeriodDay, RevenuePeriodCustom:
		bucket = `(fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodWeek:
		bucket = `DATE_TRUNC('week', fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodMonth:
		bucket = `DATE_TRUNC('month', fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodYear:
		bucket = `DATE_TRUNC('year', fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	default:
		return "", fmt.Errorf("unsupported revenue period: %s", period)
	}
	return fmt.Sprintf(`
		SELECT
			%s AS period_start,
			COALESCE(NULLIF(fa.currency, ''), 'MMK') AS currency,
			COALESCE(SUM(fa.amount), 0) AS refund_total,
			COUNT(*) AS refund_count
		FROM financial_adjustment fa
		LEFT JOIN purchase p ON p.id = fa.purchase_id
		LEFT JOIN customer c ON c.id = p.customer_id
		WHERE fa.adjustment_type = 'refund'
		  AND fa.effective_at >= $1
		  AND fa.effective_at < $2
		  AND ($3::bigint = 0 OR c.telegram_id IS NULL OR c.telegram_id <> $3)
		GROUP BY 1, 2
		ORDER BY 1 ASC`, bucket), nil
}

func (r *FinancialAdjustmentRepository) Create(ctx context.Context, in CreateFinancialAdjustmentInput) (*FinancialAdjustment, bool, error) {
	if err := validateCreateFinancialAdjustmentInput(in); err != nil {
		return nil, false, err
	}
	amount, err := normalizeAdjustmentAmount(in.Amount)
	if err != nil {
		return nil, false, err
	}
	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = "MMK"
	}

	row := &FinancialAdjustment{}
	err = r.pool.QueryRow(ctx, buildCreateFinancialAdjustmentSQL(),
		in.PurchaseID, string(in.AdjustmentType), amount, currency, in.EffectiveAt,
		in.Reason, in.ExternalRef, in.CreatedBy, strings.TrimSpace(in.IdempotencyKey),
	).Scan(
		&row.ID, &row.PurchaseID, &row.AdjustmentType, &row.Amount, &row.Currency, &row.EffectiveAt,
		&row.Reason, &row.ExternalRef, &row.CreatedBy, &row.IdempotencyKey, &row.CreatedAt,
	)
	if err == nil {
		return row, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("insert financial_adjustment: %w", err)
	}

	existing := &FinancialAdjustment{}
	err = r.pool.QueryRow(ctx, `
		SELECT id, purchase_id, adjustment_type, amount, currency, effective_at,
		       reason, external_ref, created_by, idempotency_key, created_at
		FROM financial_adjustment WHERE idempotency_key = $1`, strings.TrimSpace(in.IdempotencyKey),
	).Scan(
		&existing.ID, &existing.PurchaseID, &existing.AdjustmentType, &existing.Amount, &existing.Currency, &existing.EffectiveAt,
		&existing.Reason, &existing.ExternalRef, &existing.CreatedBy, &existing.IdempotencyKey, &existing.CreatedAt,
	)
	if err != nil {
		return nil, false, fmt.Errorf("load existing financial_adjustment: %w", err)
	}
	return existing, false, nil
}

func (r *FinancialAdjustmentRepository) SumRefundsByPeriod(ctx context.Context, start, end time.Time, period RevenueSummaryPeriod, adminTelegramID int64) ([]RefundPeriodRow, error) {
	query, err := buildSumRefundsByPeriodSQL(period)
	if err != nil {
		return nil, err
	}
	if adminTelegramID == 0 {
		adminTelegramID = config.GetAdminTelegramId()
	}
	rows, err := r.pool.Query(ctx, query, start, end, adminTelegramID)
	if err != nil {
		return nil, fmt.Errorf("sum refunds by period: %w", err)
	}
	defer rows.Close()
	var out []RefundPeriodRow
	for rows.Next() {
		var rr RefundPeriodRow
		var periodStart time.Time
		if err := rows.Scan(&periodStart, &rr.Currency, &rr.RefundTotal, &rr.RefundCount); err != nil {
			return nil, err
		}
		rr.PeriodStart = periodStart.Format("2006-01-02")
		out = append(out, rr)
	}
	return out, rows.Err()
}
