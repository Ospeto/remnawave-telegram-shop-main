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

type PromoCode struct {
	ID              int64     `db:"id"`
	Code            string    `db:"code"`
	DiscountPercent int       `db:"discount_percent"`
	MaxUses         int       `db:"max_uses"`
	UsedCount       int       `db:"used_count"`
	ValidUntil      time.Time `db:"valid_until"`
	CreatedAt       time.Time `db:"created_at"`
}

type PromoCodeRepository struct {
	pool *pgxpool.Pool
}

func NewPromoCodeRepository(pool *pgxpool.Pool) *PromoCodeRepository {
	return &PromoCodeRepository{
		pool: pool,
	}
}

func (r *PromoCodeRepository) Create(ctx context.Context, code string, discount, maxUses int, validUntil time.Time) error {
	buildInsert := sq.Insert("promo_codes").
		Columns("code", "discount_percent", "max_uses", "valid_until").
		Values(code, discount, maxUses, validUntil).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert query: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute insert query: %w", err)
	}

	return nil
}

func (r *PromoCodeRepository) FindByCode(ctx context.Context, code string) (*PromoCode, error) {
	buildSelect := buildPromoCodeSelect(time.Now()).
		Where(sq.Eq{"p.code": code})

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	promo, err := scanPromoCode(r.pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find promo code: %w", err)
	}

	return promo, nil
}

func (r *PromoCodeRepository) FindByCodeForUpdateTx(ctx context.Context, tx pgx.Tx, code string) (*PromoCode, error) {
	buildSelect := buildPromoCodeSelect(time.Now()).
		Where(sq.Eq{"p.code": code}).
		Suffix("FOR UPDATE")

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select-for-update query: %w", err)
	}

	promo, err := scanPromoCode(tx.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lock promo code: %w", err)
	}

	return promo, nil
}

func (r *PromoCodeRepository) FindByID(ctx context.Context, id int64) (*PromoCode, error) {
	buildSelect := buildPromoCodeSelect(time.Now()).
		Where(sq.Eq{"p.id": id})

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	promo, err := scanPromoCode(r.pool.QueryRow(ctx, sql, args...))
	if err != nil {
		return nil, err
	}

	return promo, nil
}

func (r *PromoCodeRepository) IncrementUsage(ctx context.Context, id int64) error {
	buildUpdate := sq.Update("promo_codes").
		Set("used_count", sq.Expr("used_count + 1")).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update query: %w", err)
	}
	return nil
}

// IncrementUsageAtomic atomically increments usage only if within limits and not expired.
// Returns true if a slot was claimed, false if the code is exhausted or expired.
func (r *PromoCodeRepository) IncrementUsageAtomic(ctx context.Context, id int64) (bool, error) {
	query := `UPDATE promo_codes SET used_count = used_count + 1
		WHERE id = $1 AND used_count < max_uses AND valid_until > NOW()`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("failed to increment promo usage: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

// ReleaseUsageAtomic rolls back one claimed promo usage slot.
// It is used when downstream purchase creation fails after a promo was already claimed.
func (r *PromoCodeRepository) ReleaseUsageAtomic(ctx context.Context, id int64) (bool, error) {
	query := `UPDATE promo_codes SET used_count = used_count - 1
		WHERE id = $1 AND used_count > 0`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("failed to release promo usage: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

// ListAll returns all promo codes ordered by creation date descending.
func (r *PromoCodeRepository) ListAll(ctx context.Context) ([]PromoCode, error) {
	buildSelect := buildPromoCodeSelect(time.Now()).
		OrderBy("p.created_at DESC")

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query promo codes: %w", err)
	}
	defer rows.Close()

	var codes []PromoCode
	for rows.Next() {
		promo, err := scanPromoCodeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan promo code: %w", err)
		}
		codes = append(codes, *promo)
	}
	return codes, rows.Err()
}

func buildPromoCodeSelect(now time.Time) sq.SelectBuilder {
	return sq.Select(
		"p.id",
		"p.code",
		"p.discount_percent",
		"p.max_uses",
	).Column(
		sq.Expr(
			`COALESCE((
				SELECT COUNT(*)
				FROM purchase pu
				WHERE pu.promo_code_id = p.id
				  AND (
					pu.status = ?
					OR (pu.status IN (?, ?, ?) AND pu.created_at >= ?)
				  )
			), 0) AS used_count`,
			PurchaseStatusPaid,
			PurchaseStatusNew,
			PurchaseStatusPending,
			PurchaseStatusProcessing,
			now.Add(-promoReservationHoldWindow),
		),
	).Column(
		"p.valid_until",
	).Column(
		"p.created_at",
	).From("promo_codes p").PlaceholderFormat(sq.Dollar)
}

func scanPromoCode(row pgx.Row) (*PromoCode, error) {
	promo := &PromoCode{}
	if err := row.Scan(
		&promo.ID,
		&promo.Code,
		&promo.DiscountPercent,
		&promo.MaxUses,
		&promo.UsedCount,
		&promo.ValidUntil,
		&promo.CreatedAt,
	); err != nil {
		return nil, err
	}
	return promo, nil
}

func scanPromoCodeRow(rows pgx.Rows) (*PromoCode, error) {
	promo := &PromoCode{}
	if err := rows.Scan(
		&promo.ID,
		&promo.Code,
		&promo.DiscountPercent,
		&promo.MaxUses,
		&promo.UsedCount,
		&promo.ValidUntil,
		&promo.CreatedAt,
	); err != nil {
		return nil, err
	}
	return promo, nil
}

// Delete removes a promo code by its code name. Returns error if not found.
func (r *PromoCodeRepository) Delete(ctx context.Context, code string) error {
	buildDelete := sq.Delete("promo_codes").
		Where(sq.Eq{"code": code}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildDelete.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	result, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to delete promo code: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("promo code '%s' not found", code)
	}
	return nil
}

// Retire deactivates a promo code without removing historical purchase references.
func (r *PromoCodeRepository) Retire(ctx context.Context, code string, retiredAt time.Time) error {
	buildUpdate := sq.Update("promo_codes").
		Set("valid_until", retiredAt.UTC()).
		Where(sq.Eq{"code": code}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build retire query: %w", err)
	}

	result, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to retire promo code: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("promo code '%s' not found", code)
	}
	return nil
}
