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
