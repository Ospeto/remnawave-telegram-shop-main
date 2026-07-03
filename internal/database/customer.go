package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/utils"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type CustomerRepository struct {
	pool *pgxpool.Pool
}

func NewCustomerRepository(pool *pgxpool.Pool) *CustomerRepository {
	return &CustomerRepository{pool: pool}
}

type Customer struct {
	ID                  int64      `db:"id"`
	TelegramID          int64      `db:"telegram_id"`
	ExpireAt            *time.Time `db:"expire_at"`
	CreatedAt           time.Time  `db:"created_at"`
	SubscriptionLink    *string    `db:"subscription_link"`
	TrialUsedAt         *time.Time `db:"trial_used_at"`
	Language            string     `db:"language"`
	Balance             float64    `db:"balance"`
	AutoRenew           bool       `db:"auto_renew"`
	AutoRenewDuration   int        `db:"auto_renew_duration"`
	AutoRenewTrafficGB  int        `db:"auto_renew_traffic_gb"`
	LastAutoRenewedAt   *time.Time `db:"last_auto_renewed_at"`
	AutoRenewNotifiedAt *time.Time `db:"auto_renew_notified_at"`
}

func (cr *CustomerRepository) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "trial_used_at", "language", "balance", "auto_renew", "auto_renew_duration", "auto_renew_traffic_gb", "last_auto_renewed_at", "auto_renew_notified_at").
		From("customer").
		Where(
			sq.And{
				sq.NotEq{"expire_at": nil},
				sq.GtOrEq{"expire_at": startDate},
				sq.LtOrEq{"expire_at": endDate},
			},
		).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers by expiration range: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err := rows.Scan(
			&customer.ID,
			&customer.TelegramID,
			&customer.ExpireAt,
			&customer.CreatedAt,
			&customer.SubscriptionLink,
			&customer.TrialUsedAt,
			&customer.Language,
			&customer.Balance,
			&customer.AutoRenew,
			&customer.AutoRenewDuration,
			&customer.AutoRenewTrafficGB,
			&customer.LastAutoRenewedAt,
			&customer.AutoRenewNotifiedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return &customers, nil
}

func (cr *CustomerRepository) FindById(ctx context.Context, id int64) (*Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "trial_used_at", "language", "balance", "auto_renew", "auto_renew_duration", "auto_renew_traffic_gb", "last_auto_renewed_at", "auto_renew_notified_at").
		From("customer").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var customer Customer

	err = cr.pool.QueryRow(ctx, sql, args...).Scan(
		&customer.ID,
		&customer.TelegramID,
		&customer.ExpireAt,
		&customer.CreatedAt,
		&customer.SubscriptionLink,
		&customer.TrialUsedAt,
		&customer.Language,
		&customer.Balance,
		&customer.AutoRenew,
		&customer.AutoRenewDuration,
		&customer.AutoRenewTrafficGB,
		&customer.LastAutoRenewedAt,
		&customer.AutoRenewNotifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}
	return &customer, nil
}

func (cr *CustomerRepository) FindByTelegramId(ctx context.Context, telegramId int64) (*Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "trial_used_at", "language", "balance", "auto_renew", "auto_renew_duration", "auto_renew_traffic_gb", "last_auto_renewed_at", "auto_renew_notified_at").
		From("customer").
		Where(sq.Eq{"telegram_id": telegramId}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	var customer Customer

	err = cr.pool.QueryRow(ctx, sql, args...).Scan(
		&customer.ID,
		&customer.TelegramID,
		&customer.ExpireAt,
		&customer.CreatedAt,
		&customer.SubscriptionLink,
		&customer.TrialUsedAt,
		&customer.Language,
		&customer.Balance,
		&customer.AutoRenew,
		&customer.AutoRenewDuration,
		&customer.AutoRenewTrafficGB,
		&customer.LastAutoRenewedAt,
		&customer.AutoRenewNotifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer: %w", err)
	}
	return &customer, nil
}

func (cr *CustomerRepository) Create(ctx context.Context, customer *Customer) (*Customer, error) {
	return cr.FindOrCreate(ctx, customer)
}

func (cr *CustomerRepository) FindOrCreate(ctx context.Context, customer *Customer) (*Customer, error) {
	query := `
		INSERT INTO customer (telegram_id, expire_at, language, balance, auto_renew, auto_renew_duration)
		VALUES ($1, $2, $3, COALESCE($4, 0), COALESCE($5, false), COALESCE($6, 30))
		ON CONFLICT (telegram_id) DO UPDATE SET telegram_id = customer.telegram_id
		RETURNING id, telegram_id, expire_at, created_at, subscription_link, trial_used_at, language, balance, auto_renew, auto_renew_duration, auto_renew_traffic_gb, last_auto_renewed_at, auto_renew_notified_at
	`

	row := cr.pool.QueryRow(ctx, query, customer.TelegramID, customer.ExpireAt, customer.Language, customer.Balance, customer.AutoRenew, customer.AutoRenewDuration)
	var result Customer
	if err := row.Scan(
		&result.ID,
		&result.TelegramID,
		&result.ExpireAt,
		&result.CreatedAt,
		&result.SubscriptionLink,
		&result.TrialUsedAt,
		&result.Language,
		&result.Balance,
		&result.AutoRenew,
		&result.AutoRenewDuration,
		&result.AutoRenewTrafficGB,
		&result.LastAutoRenewedAt,
		&result.AutoRenewNotifiedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to find or create customer: %w", err)
	}

	slog.Info("user found or created in bot database", "telegramId", utils.MaskHalfInt64(result.TelegramID))
	return &result, nil
}

// allowedCustomerFields is a whitelist of columns that can be updated via UpdateFields.
var allowedCustomerFields = map[string]bool{
	"subscription_link":      true,
	"expire_at":              true,
	"trial_used_at":          true,
	"language":               true,
	"balance":                true,
	"auto_renew":             true,
	"auto_renew_duration":    true,
	"auto_renew_traffic_gb":  true,
	"last_auto_renewed_at":   true,
	"auto_renew_notified_at": true,
}

func (cr *CustomerRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("customer").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		if !allowedCustomerFields[field] {
			return fmt.Errorf("disallowed field in customer update: %s", field)
		}
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := cr.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no customer found with id: %s", utils.MaskHalfInt64(id))
	}

	return nil
}

func (cr *CustomerRepository) UpdateFieldsTx(ctx context.Context, tx pgx.Tx, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("customer").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		if !allowedCustomerFields[field] {
			return fmt.Errorf("disallowed field in customer update: %s", field)
		}
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no customer found with id: %s", utils.MaskHalfInt64(id))
	}

	return nil
}

func (cr *CustomerRepository) FindByTelegramIdForUpdateTx(ctx context.Context, tx pgx.Tx, telegramId int64) (*Customer, error) {
	const query = `
		SELECT id, telegram_id, expire_at, created_at, subscription_link, trial_used_at, language, balance, auto_renew, auto_renew_duration, auto_renew_traffic_gb, last_auto_renewed_at, auto_renew_notified_at
		FROM customer
		WHERE telegram_id = $1
		FOR UPDATE
	`

	var customer Customer
	err := tx.QueryRow(ctx, query, telegramId).Scan(
		&customer.ID,
		&customer.TelegramID,
		&customer.ExpireAt,
		&customer.CreatedAt,
		&customer.SubscriptionLink,
		&customer.TrialUsedAt,
		&customer.Language,
		&customer.Balance,
		&customer.AutoRenew,
		&customer.AutoRenewDuration,
		&customer.AutoRenewTrafficGB,
		&customer.LastAutoRenewedAt,
		&customer.AutoRenewNotifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query customer for update: %w", err)
	}

	return &customer, nil
}

func (cr *CustomerRepository) FindByTelegramIds(ctx context.Context, telegramIDs []int64) ([]Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "trial_used_at", "language", "balance", "auto_renew", "auto_renew_duration", "auto_renew_traffic_gb", "last_auto_renewed_at", "auto_renew_notified_at").
		From("customer").
		Where(sq.Eq{"telegram_id": telegramIDs}).
		PlaceholderFormat(sq.Dollar)

	sqlStr, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err := rows.Scan(
			&customer.ID,
			&customer.TelegramID,
			&customer.ExpireAt,
			&customer.CreatedAt,
			&customer.SubscriptionLink,
			&customer.TrialUsedAt,
			&customer.Language,
			&customer.Balance,
			&customer.AutoRenew,
			&customer.AutoRenewDuration,
			&customer.AutoRenewTrafficGB,
			&customer.LastAutoRenewedAt,
			&customer.AutoRenewNotifiedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

func (cr *CustomerRepository) CreateBatch(ctx context.Context, customers []Customer) error {
	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := createBatchCustomers(ctx, tx, customers); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) CreateBatchTx(ctx context.Context, tx pgx.Tx, customers []Customer) error {
	return createBatchCustomers(ctx, tx, customers)
}

type customerBatchExecutor interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
}

func createBatchCustomers(ctx context.Context, exec customerBatchExecutor, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	builder := sq.Insert("customer").
		Columns("telegram_id", "expire_at", "language", "subscription_link").
		PlaceholderFormat(sq.Dollar)
	for _, cust := range customers {
		builder = builder.Values(cust.TelegramID, cust.ExpireAt, cust.Language, cust.SubscriptionLink)
	}
	sqlStr, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build batch insert query: %w", err)
	}

	_, err = exec.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch insert: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) UpdateBatch(ctx context.Context, customers []Customer) error {
	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := updateBatchCustomers(ctx, tx, customers); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) UpdateBatchTx(ctx context.Context, tx pgx.Tx, customers []Customer) error {
	return updateBatchCustomers(ctx, tx, customers)
}

func updateBatchCustomers(ctx context.Context, exec customerBatchExecutor, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	query := "UPDATE customer SET expire_at = c.expire_at, subscription_link = c.subscription_link FROM (VALUES "
	var args []interface{}
	for i, cust := range customers {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($%d::bigint, $%d::timestamptz, $%d::text)", i*3+1, i*3+2, i*3+3)
		args = append(args, cust.TelegramID, cust.ExpireAt, cust.SubscriptionLink)
	}
	query += ") AS c(telegram_id, expire_at, subscription_link) WHERE customer.telegram_id = c.telegram_id"

	_, err := exec.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch update: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) DeleteByNotInTelegramIds(ctx context.Context, telegramIDs []int64) error {
	return deleteByNotInTelegramIds(ctx, cr.pool, telegramIDs)
}

func (cr *CustomerRepository) DeleteByNotInTelegramIdsTx(ctx context.Context, tx pgx.Tx, telegramIDs []int64) error {
	return deleteByNotInTelegramIds(ctx, tx, telegramIDs)
}

func deleteByNotInTelegramIds(ctx context.Context, exec customerBatchExecutor, telegramIDs []int64) error {
	// Safety guard: refuse to delete ALL customers if the caller passes an empty slice.
	// An empty slice most likely indicates a failed upstream fetch, not a genuine
	// "no users exist" scenario. A full-table delete would be catastrophic.
	if len(telegramIDs) == 0 {
		return fmt.Errorf("DeleteByNotInTelegramIds: refusing to delete all customers — empty ID list provided (possible upstream fetch failure)")
	}

	buildDelete := sq.Delete("customer").
		PlaceholderFormat(sq.Dollar).
		Where(sq.NotEq{"telegram_id": telegramIDs})

	sqlStr, args, err := buildDelete.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = exec.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("failed to delete customers: %w", err)
	}

	return nil
}

func (cr *CustomerRepository) AddBalance(ctx context.Context, id int64, amount float64) error {
	query := `UPDATE customer SET balance = balance + $1 WHERE id = $2`
	_, err := cr.pool.Exec(ctx, query, amount, id)
	if err != nil {
		return fmt.Errorf("failed to add balance: %w", err)
	}
	return nil
}

// AddBalanceTx increments a customer's balance inside an existing transaction.
// Use this whenever the balance update must be atomic with a wallet_transaction insert.
func (cr *CustomerRepository) AddBalanceTx(ctx context.Context, tx pgx.Tx, id int64, amount float64) error {
	query := `UPDATE customer SET balance = balance + $1 WHERE id = $2`
	_, err := tx.Exec(ctx, query, amount, id)
	if err != nil {
		return fmt.Errorf("failed to add balance in tx: %w", err)
	}
	return nil
}

// DeductBalanceTx decrements a customer's balance inside an existing transaction.
// Use this whenever the balance update must be atomic with a wallet_transaction insert.
func (cr *CustomerRepository) DeductBalanceTx(ctx context.Context, tx pgx.Tx, id int64, amount float64) error {
	query := `UPDATE customer SET balance = balance - $1 WHERE id = $2 AND balance >= $1`
	tag, err := tx.Exec(ctx, query, amount, id)
	if err != nil {
		return fmt.Errorf("failed to deduct balance in tx: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("insufficient balance or customer not found")
	}
	return nil
}

// BeginTx begins a new database transaction scoped to the pool.
// Use this to wrap multi-step financial operations (e.g. balance + wallet_transaction log).
func (cr *CustomerRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return cr.pool.Begin(ctx)
}

// FindByAutoRenewExpiring finds customers with auto_renew=true and expire_at between now and 'before'.
func (cr *CustomerRepository) FindByAutoRenewExpiring(ctx context.Context, before time.Time) ([]Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "trial_used_at", "language", "balance", "auto_renew", "auto_renew_duration", "auto_renew_traffic_gb", "last_auto_renewed_at", "auto_renew_notified_at").
		From("customer").
		Where(
			sq.And{
				sq.Eq{"auto_renew": true},
				sq.NotEq{"expire_at": nil},
				sq.LtOrEq{"expire_at": before},
				sq.Gt{"expire_at": time.Now()},
			},
		).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var customer Customer
		err := rows.Scan(
			&customer.ID,
			&customer.TelegramID,
			&customer.ExpireAt,
			&customer.CreatedAt,
			&customer.SubscriptionLink,
			&customer.TrialUsedAt,
			&customer.Language,
			&customer.Balance,
			&customer.AutoRenew,
			&customer.AutoRenewDuration,
			&customer.AutoRenewTrafficGB,
			&customer.LastAutoRenewedAt,
			&customer.AutoRenewNotifiedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over customer rows: %w", err)
	}

	return customers, nil
}

// MarkAutoRenewed stamps last_auto_renewed_at = now so the same expiry cycle
// cannot trigger a second charge even if the cron fires twice.
func (cr *CustomerRepository) MarkAutoRenewed(ctx context.Context, customerID int64) error {
	now := time.Now()
	query := `UPDATE customer SET last_auto_renewed_at = $1 WHERE id = $2`
	_, err := cr.pool.Exec(ctx, query, now, customerID)
	if err != nil {
		return fmt.Errorf("failed to mark auto renewed: %w", err)
	}
	return nil
}

// MarkAutoRenewNotified stamps auto_renew_notified_at = now so we don't re-spam
// the user about low balance more than once per day.
func (cr *CustomerRepository) MarkAutoRenewNotified(ctx context.Context, customerID int64) error {
	now := time.Now()
	query := `UPDATE customer SET auto_renew_notified_at = $1 WHERE id = $2`
	_, err := cr.pool.Exec(ctx, query, now, customerID)
	if err != nil {
		return fmt.Errorf("failed to mark auto renew notified: %w", err)
	}
	return nil
}
