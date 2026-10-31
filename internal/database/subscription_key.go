package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	ErrKeyNotFound        = errors.New("subscription key not found")
	ErrKeyNotOwned        = errors.New("subscription key not owned by customer")
	ErrKeyActive          = errors.New("cannot hide active subscription key")
	ErrKeyDeleted         = errors.New("cannot hide deleted subscription key")
	ErrKeyIndeterminate   = errors.New("cannot hide subscription key with indeterminate expiry")
	ErrKeyRenewalInFlight = errors.New("renewal in progress, cannot hide key")
)

type SubscriptionKey struct {
	ID                   int64      `db:"id"`
	CustomerID           int64      `db:"customer_id"`
	RemnawaveUUID        uuid.UUID  `db:"remnawave_uuid"`
	Username             string     `db:"username"`
	SubscriptionURL      string     `db:"subscription_url"`
	ExpireAt             *time.Time `db:"expire_at"`
	Status               string     `db:"status"`
	CreatedAt            time.Time  `db:"created_at"`
	Label                string     `db:"label"`
	TrafficLimitGB       int        `db:"traffic_limit_gb"`
	AutoRenew            bool       `db:"auto_renew"`
	ExpirationNotifiedAt *time.Time `db:"expiration_notified_at"`
	LastAutoRenewedAt    *time.Time `db:"last_auto_renewed_at"`
	AutoRenewNotifiedAt  *time.Time `db:"auto_renew_notified_at"`
	AutoRenewPlanDays    *int       `db:"auto_renew_plan_days"`
	AutoRenewPlanTraffic *int       `db:"auto_renew_plan_traffic_gb"`
	AutoRenewClaimedAt   *time.Time `db:"auto_renew_claimed_at"`
	IsHidden             bool       `db:"is_hidden"`
	HiddenAt             *time.Time `db:"hidden_at"`
}

type SubscriptionKeyRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionKeyRepository(pool *pgxpool.Pool) *SubscriptionKeyRepository {
	return &SubscriptionKeyRepository{pool: pool}
}

func PrimarySubscriptionKey(keys []SubscriptionKey) *SubscriptionKey {
	var best *SubscriptionKey
	now := time.Now()

	for i := range keys {
		key := &keys[i]
		if key.Status == "deleted" || key.IsHidden || key.ExpireAt == nil {
			continue
		}
		if key.ExpireAt.Before(now) {
			continue
		}
		if best == nil || key.ExpireAt.After(*best.ExpireAt) || (key.ExpireAt.Equal(*best.ExpireAt) && key.CreatedAt.After(best.CreatedAt)) {
			best = key
		}
	}

	return best
}

var subKeyColumns = []string{
	"id", "customer_id", "remnawave_uuid", "username",
	"subscription_url", "expire_at", "status", "created_at", "label",
	"traffic_limit_gb", "auto_renew", "expiration_notified_at", "last_auto_renewed_at", "auto_renew_notified_at",
	"auto_renew_plan_days", "auto_renew_plan_traffic_gb", "auto_renew_claimed_at",
	"is_hidden", "hidden_at",
}

func scanSubKey(row pgx.Row) (*SubscriptionKey, error) {
	var k SubscriptionKey
	err := row.Scan(
		&k.ID, &k.CustomerID, &k.RemnawaveUUID, &k.Username,
		&k.SubscriptionURL, &k.ExpireAt, &k.Status, &k.CreatedAt, &k.Label,
		&k.TrafficLimitGB, &k.AutoRenew, &k.ExpirationNotifiedAt, &k.LastAutoRenewedAt, &k.AutoRenewNotifiedAt,
		&k.AutoRenewPlanDays, &k.AutoRenewPlanTraffic, &k.AutoRenewClaimedAt,
		&k.IsHidden, &k.HiddenAt,
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
		&k.TrafficLimitGB, &k.AutoRenew, &k.ExpirationNotifiedAt, &k.LastAutoRenewedAt, &k.AutoRenewNotifiedAt,
		&k.AutoRenewPlanDays, &k.AutoRenewPlanTraffic, &k.AutoRenewClaimedAt,
		&k.IsHidden, &k.HiddenAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan subscription_key row: %w", err)
	}
	return &k, nil
}

func (r *SubscriptionKeyRepository) Create(ctx context.Context, key *SubscriptionKey) (int64, error) {
	return createSubscriptionKey(ctx, r.pool, key)
}

func (r *SubscriptionKeyRepository) CreateTx(ctx context.Context, tx pgx.Tx, key *SubscriptionKey) (int64, error) {
	return createSubscriptionKey(ctx, tx, key)
}

func createSubscriptionKey(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}, key *SubscriptionKey) (int64, error) {
	query := sq.Insert("subscription_key").
		Columns("customer_id", "remnawave_uuid", "username", "subscription_url", "expire_at", "status", "label", "traffic_limit_gb", "auto_renew_plan_days", "auto_renew_plan_traffic_gb").
		Values(key.CustomerID, key.RemnawaveUUID, key.Username, key.SubscriptionURL, key.ExpireAt, key.Status, key.Label, key.TrafficLimitGB, key.AutoRenewPlanDays, key.AutoRenewPlanTraffic).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build insert: %w", err)
	}

	var id int64
	err = db.QueryRow(ctx, sql, args...).Scan(&id)
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

// LinkRemoteIdentity repairs legacy placeholder UUIDs using a verified remote
// user's subscription URL. It never changes ownership or revives deleted keys.
func (r *SubscriptionKeyRepository) LinkRemoteIdentity(ctx context.Context, key *SubscriptionKey, remoteUUID uuid.UUID, username string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE subscription_key
		SET remnawave_uuid = $1, username = $2
		WHERE id = $3 AND customer_id = $4 AND subscription_url = $5 AND status <> 'deleted'`,
		remoteUUID, username, key.ID, key.CustomerID, key.SubscriptionURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscription_key %d no longer available for reconciliation", key.ID)
	}
	return nil
}

func (r *SubscriptionKeyRepository) UpdateExpiry(ctx context.Context, id int64, expireAt time.Time) error {
	query := sq.Update("subscription_key").
		Set("expire_at", expireAt).
		Set("status", "active").
		Set("expiration_notified_at", nil).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscription_key %d not found", id)
	}
	return nil
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

// SyncExpiryStatus atomically persists synced remote expiry and status for a key
// that has not been locally deleted (WHERE status <> 'deleted'). It resets
// expiration_notified_at to NULL only when expiry or status changes.
// Returns affected=true if a row was updated, or affected=false if the key was deleted/missing.
func (r *SubscriptionKeyRepository) SyncExpiryStatus(ctx context.Context, id int64, expireAt time.Time, status string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE subscription_key
		SET expire_at = $1::timestamptz,
		    status = $2::text,
		    expiration_notified_at = CASE
		        WHEN expire_at IS DISTINCT FROM $1::timestamptz OR status IS DISTINCT FROM $2::text
		        THEN NULL ELSE expiration_notified_at
		    END
		WHERE id = $3 AND status <> 'deleted'`,
		expireAt, status, id)
	if err != nil {
		return false, fmt.Errorf("failed to sync expiry and status: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *SubscriptionKeyRepository) MarkMissingRemoteKeysDeleted(ctx context.Context, remoteUUIDs []uuid.UUID) (int64, error) {
	if len(remoteUUIDs) == 0 {
		return 0, nil
	}

	query := sq.Update("subscription_key").
		Set("status", "deleted").
		Where(sq.NotEq{"status": "deleted"}).
		Where(sq.NotEq{"remnawave_uuid": remoteUUIDs}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build missing remote key update: %w", err)
	}

	tag, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to mark missing remote keys deleted: %w", err)
	}
	return tag.RowsAffected(), nil
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
	tag, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscription_key %d not found", id)
	}
	return nil
}

func (r *SubscriptionKeyRepository) UpdateTrafficLimit(ctx context.Context, id int64, trafficLimitGB int) error {
	query := sq.Update("subscription_key").
		Set("traffic_limit_gb", trafficLimitGB).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscription_key %d not found", id)
	}
	return nil
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

// HideExpiredKey atomically hides an expired subscription key for the given customer.
// It uses row-level locking (SELECT ... FOR UPDATE) inside a transaction to prevent
// race conditions with concurrent renewals. It verifies:
// - The key exists and belongs to customerID
// - The key status is not 'active' and not 'deleted'
// - The key has a non-nil expire_at strictly in the past (expire_at < NOW())
// - The key has no active auto-renew claim in progress
// - The key has no in-flight purchase extension in progress
// If the key is already hidden, it succeeds idempotently.
// On success, sets is_hidden = true, hidden_at = NOW(). Preserves auto_renew and lifecycle configuration.
func (r *SubscriptionKeyRepository) HideExpiredKey(ctx context.Context, keyID int64, customerID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch with row lock to serialize with concurrent renewals
	row := tx.QueryRow(ctx, `
		SELECT `+strings.Join(subKeyColumns, ", ")+`
		FROM subscription_key
		WHERE id = $1
		FOR UPDATE
	`, keyID)

	key, err := scanSubKey(row)
	if err != nil {
		return fmt.Errorf("failed to scan subscription key: %w", err)
	}
	if key == nil {
		return ErrKeyNotFound
	}
	if key.CustomerID != customerID {
		return ErrKeyNotOwned
	}
	if key.Status == "deleted" {
		return ErrKeyDeleted
	}
	if key.Status == "active" {
		return ErrKeyActive
	}
	if key.ExpireAt == nil {
		return ErrKeyIndeterminate
	}
	now := time.Now()
	if !key.ExpireAt.Before(now) {
		return ErrKeyActive
	}

	// Check if auto-renew claim is active
	if key.AutoRenewClaimedAt != nil && key.AutoRenewClaimedAt.After(now.Add(-AutoRenewClaimTTL)) {
		return ErrKeyRenewalInFlight
	}

	// Check if an in-flight purchase extension exists for this key
	var pending int
	err = tx.QueryRow(ctx, `
		SELECT 1 FROM purchase
		WHERE extend_key_id = $1 AND status IN ('new', 'pending', 'processing')
		LIMIT 1
	`, keyID).Scan(&pending)
	if err == nil {
		return ErrKeyRenewalInFlight
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to check in-flight purchase extension: %w", err)
	}

	if key.IsHidden {
		// Already hidden, commit idempotently
		return tx.Commit(ctx)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE subscription_key
		SET is_hidden = TRUE,
		    hidden_at = NOW()
		WHERE id = $1 AND customer_id = $2
	`, keyID, customerID)
	if err != nil {
		return fmt.Errorf("failed to update subscription key to hidden: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrKeyNotFound
	}

	return tx.Commit(ctx)
}

// ValidateExtendKeyTx validates that a key can be extended while holding a FOR SHARE lock
// on the subscription_key row inside tx, preventing concurrent hide transactions.
func (r *SubscriptionKeyRepository) ValidateExtendKeyTx(ctx context.Context, tx pgx.Tx, keyID int64, customerID int64) error {
	if tx == nil {
		return nil
	}
	var status string
	var custID int64
	var remUUID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT customer_id, status, remnawave_uuid
		FROM subscription_key
		WHERE id = $1
		FOR SHARE
	`, keyID).Scan(&custID, &status, &remUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKeyNotFound
		}
		return fmt.Errorf("failed to validate extend key: %w", err)
	}
	if custID != customerID || (status != "active" && status != "expired") || remUUID == uuid.Nil {
		return ErrKeyNotOwned
	}
	return nil
}

// FindExpiringAutoRenewKeys returns all active keys with auto_renew=true
// whose expire_at is between 'after' and 'before'.
// Used exclusively by the auto-renew cron job.
func (r *SubscriptionKeyRepository) FindExpiringAutoRenewKeys(ctx context.Context, after time.Time, before time.Time) ([]SubscriptionKey, error) {
	query := sq.Select(subKeyColumns...).
		From("subscription_key").
		Where(sq.And{
			sq.Eq{"auto_renew": true},
			sq.Eq{"status": "active"},
			sq.NotEq{"expire_at": nil},
			sq.Gt{"expire_at": after},
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

// AutoRenewClaimTTL is the lease duration for auto_renew_claimed_at.
// Stale pre-side-effect claims older than this may be reclaimed by another
// worker. Post-side-effect safety relies on last_auto_renewed_at cycle locks
// (service stamps before extend), not on never reclaiming.
const AutoRenewClaimTTL = 30 * time.Minute

// TryClaimAutoRenew atomically claims a key for processing by setting
// auto_renew_claimed_at=NOW() only if it still has the expected successful
// renewal marker and is either unclaimed or has a stale (TTL-expired) claim.
// Fresh claims are never stolen. Returns (claimedAt, true, nil) on success.
func (r *SubscriptionKeyRepository) TryClaimAutoRenew(ctx context.Context, keyID int64, expectedLast *time.Time) (*time.Time, bool, error) {
	return r.TryClaimAutoRenewWithTTL(ctx, keyID, expectedLast, AutoRenewClaimTTL)
}

// TryClaimAutoRenewWithTTL is like TryClaimAutoRenew but accepts an explicit TTL
// (for tests and shared constants).
func (r *SubscriptionKeyRepository) TryClaimAutoRenewWithTTL(ctx context.Context, keyID int64, expectedLast *time.Time, claimTTL time.Duration) (*time.Time, bool, error) {
	if claimTTL <= 0 {
		claimTTL = AutoRenewClaimTTL
	}
	// Pass seconds as float so Postgres can build an interval without driver-specific types.
	ttlSeconds := claimTTL.Seconds()
	var claimedAt time.Time
	err := r.pool.QueryRow(ctx, `
		UPDATE subscription_key
		SET auto_renew_claimed_at = NOW()
		WHERE id = $1
		  AND auto_renew = TRUE
		  AND status = 'active'
		  AND last_auto_renewed_at IS NOT DISTINCT FROM $2
		  AND (
		        auto_renew_claimed_at IS NULL
		        OR auto_renew_claimed_at <= NOW() - make_interval(secs => $3::double precision)
		      )
		RETURNING auto_renew_claimed_at
	`, keyID, expectedLast, ttlSeconds).Scan(&claimedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to claim key auto-renew: %w", err)
	}
	return &claimedAt, true, nil
}

// ReleaseAutoRenewClaim clears an in-flight claim after a failed attempt.
// It only releases the claim if this worker still owns it.
func (r *SubscriptionKeyRepository) ReleaseAutoRenewClaim(ctx context.Context, keyID int64, claimedAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscription_key
		SET auto_renew_claimed_at = NULL
		WHERE id = $1 AND auto_renew_claimed_at = $2
	`, keyID, claimedAt)
	if err != nil {
		return fmt.Errorf("failed to release auto-renew claim: %w", err)
	}
	return nil
}

// FindExpiringKeys returns all active keys whose expire_at is between startDate and endDate.
func (r *SubscriptionKeyRepository) FindExpiringKeys(ctx context.Context, startDate, endDate time.Time) ([]SubscriptionKey, error) {
	query := sq.Select(subKeyColumns...).
		From("subscription_key").
		Where(sq.And{
			sq.Eq{"status": "active"},
			sq.NotEq{"expire_at": nil},
			sq.GtOrEq{"expire_at": startDate},
			sq.LtOrEq{"expire_at": endDate},
			sq.Eq{"expiration_notified_at": nil},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build expiring query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query expiring keys: %w", err)
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

func (r *SubscriptionKeyRepository) MarkExpirationNotified(ctx context.Context, keyID int64, notifiedAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE subscription_key SET expiration_notified_at = $1 WHERE id = $2`,
		notifiedAt, keyID,
	)
	if err != nil {
		return fmt.Errorf("failed to mark expiration notified: %w", err)
	}
	return nil
}

// MarkKeyAutoRenewed stamps the successful renewal marker and clears the
// transient claim so future runs can safely continue from the new state.
func (r *SubscriptionKeyRepository) MarkKeyAutoRenewed(ctx context.Context, keyID int64, claimedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE subscription_key
		 SET last_auto_renewed_at = NOW(),
		     auto_renew_claimed_at = NULL,
		     auto_renew_notified_at = NULL
		 WHERE id = $1 AND auto_renew_claimed_at = $2`,
		keyID, claimedAt)
	if err != nil {
		return fmt.Errorf("failed to mark key auto renewed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key %d auto-renew claim was lost before finalization", keyID)
	}
	return nil
}

// StampKeyLastAutoRenewed sets last_auto_renewed_at while keeping the claim held.
// Used as a durable cycle lock before side effects (and as a best-effort refresh
// if MarkKeyAutoRenewed fails after extend) so TTL reclaim cannot double-charge.
//
// Residual (intentional, money-safe): if the process dies after this stamp and
// before ExtendKeyWithBalance completes, later auto-renew runs skip the key via
// the cycle guard until ops clears last_auto_renewed_at. Prefer stuck/manual
// recovery over a second charge.
func (r *SubscriptionKeyRepository) StampKeyLastAutoRenewed(ctx context.Context, keyID int64, claimedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE subscription_key
		 SET last_auto_renewed_at = NOW()
		 WHERE id = $1 AND auto_renew_claimed_at = $2`,
		keyID, claimedAt)
	if err != nil {
		return fmt.Errorf("failed to stamp key last auto renewed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key %d auto-renew claim was lost before cycle stamp", keyID)
	}
	return nil
}

// ClearKeyLastAutoRenewed sets last_auto_renewed_at = NULL while the worker still
// owns the claim. Used only to undo an optimistic pre-extend cycle lock after
// extend fails so the same cycle remains retryable.
//
// This does not restore any prior last_auto_renewed_at history — it clears the
// optimistic lock only. Callers must still own auto_renew_claimed_at = claimedAt.
func (r *SubscriptionKeyRepository) ClearKeyLastAutoRenewed(ctx context.Context, keyID int64, claimedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE subscription_key
		 SET last_auto_renewed_at = NULL
		 WHERE id = $1 AND auto_renew_claimed_at = $2`,
		keyID, claimedAt)
	if err != nil {
		return fmt.Errorf("failed to clear key last auto renewed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key %d auto-renew claim was lost before cycle clear", keyID)
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

func (r *SubscriptionKeyRepository) UpdateAutoRenewPlan(ctx context.Context, keyID int64, days int, planTrafficGB int) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE subscription_key
		 SET auto_renew_plan_days = $1,
		     auto_renew_plan_traffic_gb = $2
		 WHERE id = $3`,
		days, planTrafficGB, keyID)
	if err != nil {
		return fmt.Errorf("failed to update auto-renew plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscription_key %d not found", keyID)
	}
	return nil
}

// DeleteByCustomerID deletes all subscription keys for a given customer.
// It first unlinks purchase.extend_key_id references to prevent foreign key constraint violations.
func (r *SubscriptionKeyRepository) DeleteByCustomerID(ctx context.Context, customerID int64) (int64, error) {
	// First NULL out any purchases pointing to this customer's subscription keys
	_, err := r.pool.Exec(ctx, `
		UPDATE purchase 
		SET extend_key_id = NULL 
		WHERE customer_id = $1 
		   OR extend_key_id IN (SELECT id FROM subscription_key WHERE customer_id = $1)
	`, customerID)
	if err != nil {
		return 0, fmt.Errorf("failed to unlink purchase.extend_key_id for customer %d: %w", customerID, err)
	}

	tag, err := r.pool.Exec(ctx, "DELETE FROM subscription_key WHERE customer_id = $1", customerID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete subscription keys for customer %d: %w", customerID, err)
	}
	return tag.RowsAffected(), nil
}
