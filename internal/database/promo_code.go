package database

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
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
		return nil, err // Caller checks pgx.ErrNoRows if needed
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
