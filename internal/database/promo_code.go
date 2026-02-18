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
	buildSelect := sq.Select("*").
		From("promo_codes").
		Where(sq.Eq{"code": code}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var promo PromoCode
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&promo.ID,
		&promo.Code,
		&promo.DiscountPercent,
		&promo.MaxUses,
		&promo.UsedCount,
		&promo.ValidUntil,
		&promo.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find promo code: %w", err)
	}

	return &promo, nil
}

func (r *PromoCodeRepository) FindByID(ctx context.Context, id int64) (*PromoCode, error) {
	buildSelect := sq.Select("*").
		From("promo_codes").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var promo PromoCode
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&promo.ID,
		&promo.Code,
		&promo.DiscountPercent,
		&promo.MaxUses,
		&promo.UsedCount,
		&promo.ValidUntil,
		&promo.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &promo, nil
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

// ListAll returns all promo codes ordered by creation date descending.
func (r *PromoCodeRepository) ListAll(ctx context.Context) ([]PromoCode, error) {
	buildSelect := sq.Select("*").
		From("promo_codes").
		OrderBy("created_at DESC").
		PlaceholderFormat(sq.Dollar)

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
		var p PromoCode
		if err := rows.Scan(&p.ID, &p.Code, &p.DiscountPercent, &p.MaxUses, &p.UsedCount, &p.ValidUntil, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan promo code: %w", err)
		}
		codes = append(codes, p)
	}
	return codes, rows.Err()
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
