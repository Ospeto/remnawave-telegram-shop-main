package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/utils"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type CustomerRepository struct {
	pool *pgxpool.Pool
}

func NewCustomerRepository(poll *pgxpool.Pool) *CustomerRepository {
	return &CustomerRepository{pool: poll}
}

type Customer struct {
	ID                int64      `db:"id"`
	TelegramID        int64      `db:"telegram_id"`
	ExpireAt          *time.Time `db:"expire_at"`
	CreatedAt         time.Time  `db:"created_at"`
	SubscriptionLink  *string    `db:"subscription_link"`
	Language          string     `db:"language"`
	Balance           float64    `db:"balance"`
	AutoRenew         bool       `db:"auto_renew"`
	AutoRenewDuration int        `db:"auto_renew_duration"`
}

func (cr *CustomerRepository) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "language", "balance", "auto_renew", "auto_renew_duration").
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
			&customer.Language,
			&customer.Balance,
			&customer.AutoRenew,
			&customer.AutoRenewDuration,
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
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "language", "balance", "auto_renew", "auto_renew_duration").
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
		&customer.Language,
		&customer.Balance,
		&customer.AutoRenew,
		&customer.AutoRenewDuration,
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
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "language", "balance", "auto_renew", "auto_renew_duration").
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
		&customer.Language,
		&customer.Balance,
		&customer.AutoRenew,
		&customer.AutoRenewDuration,
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
		RETURNING id, telegram_id, expire_at, created_at, subscription_link, language, balance, auto_renew, auto_renew_duration
	`

	row := cr.pool.QueryRow(ctx, query, customer.TelegramID, customer.ExpireAt, customer.Language, customer.Balance, customer.AutoRenew, customer.AutoRenewDuration)
	var result Customer
	if err := row.Scan(
		&result.ID,
		&result.TelegramID,
		&result.ExpireAt,
		&result.CreatedAt,
		&result.SubscriptionLink,
		&result.Language,
		&result.Balance,
		&result.AutoRenew,
		&result.AutoRenewDuration,
	); err != nil {
		return nil, fmt.Errorf("failed to find or create customer: %w", err)
	}

	slog.Info("user found or created in bot database", "telegramId", utils.MaskHalfInt64(result.TelegramID))
	return &result, nil
}

// allowedCustomerFields is a whitelist of columns that can be updated via UpdateFields.
var allowedCustomerFields = map[string]bool{
	"subscription_link":   true,
	"expire_at":           true,
	"language":            true,
	"balance":             true,
	"auto_renew":          true,
	"auto_renew_duration": true,
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

func (cr *CustomerRepository) FindByTelegramIds(ctx context.Context, telegramIDs []int64) ([]Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "language", "balance", "auto_renew", "auto_renew_duration").
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
			&customer.Language,
			&customer.Balance,
			&customer.AutoRenew,
			&customer.AutoRenewDuration,
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

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("failed to execute batch insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) UpdateBatch(ctx context.Context, customers []Customer) error {
	if len(customers) == 0 {
		return nil
	}
	query := "UPDATE customer SET expire_at = c.expire_at, subscription_link = c.subscription_link FROM (VALUES "
	var args []interface{}
	for i, cust := range customers {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($%d::bigint, $%d::timestamp, $%d::text)", i*3+1, i*3+2, i*3+3)
		args = append(args, cust.TelegramID, cust.ExpireAt, cust.SubscriptionLink)
	}
	query += ") AS c(telegram_id, expire_at, subscription_link) WHERE customer.telegram_id = c.telegram_id"

	tx, err := cr.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("failed to execute batch update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (cr *CustomerRepository) DeleteByNotInTelegramIds(ctx context.Context, telegramIDs []int64) error {
	var buildDelete sq.DeleteBuilder
	if len(telegramIDs) == 0 {
		buildDelete = sq.Delete("customer")
	} else {
		buildDelete = sq.Delete("customer").
			PlaceholderFormat(sq.Dollar).
			Where(sq.NotEq{"telegram_id": telegramIDs})
	}

	sqlStr, args, err := buildDelete.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = cr.pool.Exec(ctx, sqlStr, args...)
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

func (cr *CustomerRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	query := `UPDATE customer SET balance = balance - $1 WHERE id = $2 AND balance >= $1`
	tag, err := cr.pool.Exec(ctx, query, amount, id)
	if err != nil {
		return fmt.Errorf("failed to deduct balance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("insufficient balance or customer not found")
	}
	return nil
}

func (cr *CustomerRepository) SetAutoRenew(ctx context.Context, id int64, enabled bool, duration int) error {
	query := `UPDATE customer SET auto_renew = $1, auto_renew_duration = $2 WHERE id = $3`
	_, err := cr.pool.Exec(ctx, query, enabled, duration, id)
	if err != nil {
		return fmt.Errorf("failed to set auto renew: %w", err)
	}
	return nil
}

// FindByAutoRenewExpiring finds customers with auto_renew=true and expire_at before the given date
func (cr *CustomerRepository) FindByAutoRenewExpiring(ctx context.Context, before time.Time) ([]Customer, error) {
	buildSelect := sq.Select("id", "telegram_id", "expire_at", "created_at", "subscription_link", "language", "balance", "auto_renew", "auto_renew_duration").
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
			&customer.Language,
			&customer.Balance,
			&customer.AutoRenew,
			&customer.AutoRenewDuration,
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
