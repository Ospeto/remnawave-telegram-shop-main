package database

import (
	"context"

	"github.com/jackc/pgx/v4/pgxpool"
)

type AppConfigRepository struct {
	pool *pgxpool.Pool
}

func NewAppConfigRepository(pool *pgxpool.Pool) *AppConfigRepository {
	return &AppConfigRepository{pool: pool}
}

// Get finds a configuration value by key.
func (r *AppConfigRepository) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.pool.QueryRow(ctx, "SELECT value FROM app_config WHERE key = $1", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// Set inserts or updates a configuration value.
func (r *AppConfigRepository) Set(ctx context.Context, key string, value string) error {
	_, err := r.pool.Exec(ctx, `
			INSERT INTO app_config (key, value) 
			VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
		`, key, value)
	return err
}

// CompareAndSwap updates a configuration value only when the current value
// matches expected. When expected is empty, a missing row is treated as empty.
func (r *AppConfigRepository) CompareAndSwap(ctx context.Context, key string, expected string, value string) (bool, error) {
	var swapped bool
	err := r.pool.QueryRow(ctx, `
		WITH existing AS (
			SELECT value FROM app_config WHERE key = $1
		),
		updated AS (
			UPDATE app_config
			SET value = $3
			WHERE key = $1 AND value = $2
			RETURNING 1
		),
		inserted AS (
			INSERT INTO app_config (key, value)
			SELECT $1, $3
			WHERE $2 = '' AND NOT EXISTS (SELECT 1 FROM existing)
			ON CONFLICT (key) DO NOTHING
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM updated) OR EXISTS (SELECT 1 FROM inserted)
	`, key, expected, value).Scan(&swapped)
	if err != nil {
		return false, err
	}
	return swapped, nil
}
