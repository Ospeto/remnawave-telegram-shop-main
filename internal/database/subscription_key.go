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

type SubscriptionKey struct {
	ID                  int64      `db:"id"`
	CustomerID          int64      `db:"customer_id"`
	RemnawaveUUID       uuid.UUID  `db:"remnawave_uuid"`
	Username            string     `db:"username"`
	SubscriptionURL     string     `db:"subscription_url"`
	ExpireAt            *time.Time `db:"expire_at"`
	Status              string     `db:"status"`
	CreatedAt           time.Time  `db:"created_at"`
	Label               string     `db:"label"`
	TrafficLimitGB      int        `db:"traffic_limit_gb"`
	AutoRenew           bool       `db:"auto_renew"`
	LastAutoRenewedAt   *time.Time `db:"last_auto_renewed_at"`
	AutoRenewNotifiedAt *time.Time `db:"auto_renew_notified_at"`
}

type SubscriptionKeyRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionKeyRepository(pool *pgxpool.Pool) *SubscriptionKeyRepository {
	return &SubscriptionKeyRepository{pool: pool}
}

var subKeyColumns = []string{
	"id", "customer_id", "remnawave_uuid", "username",
	"subscription_url", "expire_at", "status", "created_at", "label",
	"traffic_limit_gb", "auto_renew", "last_auto_renewed_at", "auto_renew_notified_at",
}

func scanSubKey(row pgx.Row) (*SubscriptionKey, error) {
	var k SubscriptionKey
	err := row.Scan(
		&k.ID, &k.CustomerID, &k.RemnawaveUUID, &k.Username,
		&k.SubscriptionURL, &k.ExpireAt, &k.Status, &k.CreatedAt, &k.Label,
		&k.TrafficLimitGB, &k.AutoRenew, &k.LastAutoRenewedAt, &k.AutoRenewNotifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan subscription_key: %w", err)
	}
	return &k, nil
}

func scanSubKeyFromRows(rows pgx.Rows) (*SubscriptionKey, error) {
	var k SubscriptionKey
	err := rows.Scan(
		&k.ID, &k.CustomerID, &k.RemnawaveUUID, &k.Username,
		&k.SubscriptionURL, &k.ExpireAt, &k.Status, &k.CreatedAt, &k.Label,
		&k.TrafficLimitGB, &k.AutoRenew, &k.LastAutoRenewedAt, &k.AutoRenewNotifiedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan subscription_key row: %w", err)
	}
	return &k, nil
}

func (r *SubscriptionKeyRepository) Create(ctx context.Context, key *SubscriptionKey) (int64, error) {
	query := sq.Insert("subscription_key").
		Columns("customer_id", "remnawave_uuid", "username", "subscription_url", "expire_at", "status", "label", "traffic_limit_gb").
		Values(key.CustomerID, key.RemnawaveUUID, key.Username, key.SubscriptionURL, key.ExpireAt, key.Status, key.Label, key.TrafficLimitGB).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build insert: %w", err)
	}

	var id int64
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create subscription_key: %w", err)
	}
	return id, nil
}

func (r *SubscriptionKeyRepository) FindByCustomerID(ctx context.Context, customerID int64) ([]SubscriptionKey, error) {
	query := sq.Select(subKeyColumns...).
		From("subscription_key").
		Where(sq.Eq{"customer_id": customerID}).
		OrderBy("created_at DESC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscription_keys: %w", err)
	}
	defer rows.Close()

	var keys []SubscriptionKey
	for rows.Next() {
		k, err := scanSubKeyFromRows(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

func (r *SubscriptionKeyRepository) FindByID(ctx context.Context, id int64) (*SubscriptionKey, error) {
	query := sq.Select(subKeyColumns...).
		From("subscription_key").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}
	return scanSubKey(r.pool.QueryRow(ctx, sql, args...))
}

func (r *SubscriptionKeyRepository) FindByRemnawaveUUID(ctx context.Context, uuid uuid.UUID) (*SubscriptionKey, error) {
	query := sq.Select(subKeyColumns...).
		From("subscription_key").
		Where(sq.Eq{"remnawave_uuid": uuid}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}
	return scanSubKey(r.pool.QueryRow(ctx, sql, args...))
}

func (r *SubscriptionKeyRepository) UpdateExpiry(ctx context.Context, id int64, expireAt time.Time) error {
	query := sq.Update("subscription_key").
		Set("expire_at", expireAt).
		Set("status", "active").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sql, args...)
	return err
}

func (r *SubscriptionKeyRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	query := sq.Update("subscription_key").
		Set("status", status).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sql, args...)
	return err
}

func (r *SubscriptionKeyRepository) UpdateSubscriptionURL(ctx context.Context, id int64, url string) error {
	query := sq.Update("subscription_key").
		Set("subscription_url", url).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sql, args...)
	return err
}

func (r *SubscriptionKeyRepository) CountByCustomerID(ctx context.Context, customerID int64) (int, error) {
	query := sq.Select("COUNT(*)").
		From("subscription_key").
		Where(sq.Eq{"customer_id": customerID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, err
	}

	var count int
	err = r.pool.QueryRow(ctx, sql, args...).Scan(&count)
	return count, err
}

// SetAutoRenew enables or disables auto-renew for a specific subscription key.
// Ownership check (customerID) is enforced to prevent cross-user tampering.
func (r *SubscriptionKeyRepository) SetAutoRenew(ctx context.Context, keyID int64, customerID int64, enabled bool) error {
	query := `
		UPDATE subscription_key
		SET auto_renew = $1
		WHERE id = $2 AND customer_id = $3
	`
	tag, err := r.pool.Exec(ctx, query, enabled, keyID, customerID)
	if err != nil {
		return fmt.Errorf("failed to set key auto_renew: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key %d not found or not owned by customer %d", keyID, customerID)
	}
	return nil
}

// FindExpiringAutoRenewKeys returns all active keys with auto_renew=true
// whose expire_at is between now and 'before' (e.g. now+3days).
// Used exclusively by the auto-renew cron job.
func (r *SubscriptionKeyRepository) FindExpiringAutoRenewKeys(ctx context.Context, before time.Time) ([]SubscriptionKey, error) {
	query := sq.Select(subKeyColumns...).
		From("subscription_key").
		Where(sq.And{
			sq.Eq{"auto_renew": true},
			sq.Eq{"status": "active"},
			sq.NotEq{"expire_at": nil},
			sq.Gt{"expire_at": time.Now()},
			sq.LtOrEq{"expire_at": before},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build expiring auto-renew query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query expiring auto-renew keys: %w", err)
	}
	defer rows.Close()

	var keys []SubscriptionKey
	for rows.Next() {
		k, err := scanSubKeyFromRows(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

// MarkKeyAutoRenewed stamps last_auto_renewed_at = now to prevent double-charge
// if the cron fires twice before the subscription is actually extended.
func (r *SubscriptionKeyRepository) MarkKeyAutoRenewed(ctx context.Context, keyID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE subscription_key SET last_auto_renewed_at = NOW() WHERE id = $1`,
		keyID)
	if err != nil {
		return fmt.Errorf("failed to mark key auto renewed: %w", err)
	}
	return nil
}

// MarkKeyAutoRenewNotified stamps auto_renew_notified_at = now so we don't
// re-send a low-balance warning more than once per day.
func (r *SubscriptionKeyRepository) MarkKeyAutoRenewNotified(ctx context.Context, keyID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE subscription_key SET auto_renew_notified_at = NOW() WHERE id = $1`,
		keyID)
	if err != nil {
		return fmt.Errorf("failed to mark key auto renew notified: %w", err)
	}
	return nil
}
