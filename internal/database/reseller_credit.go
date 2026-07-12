package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Reseller ledger entry types and directions (AR, separate from wallet).
type ResellerLedgerEntryType string

const (
	ResellerLedgerEntryTypeSale       ResellerLedgerEntryType = "sale"
	ResellerLedgerEntryTypeSettlement ResellerLedgerEntryType = "settlement"
	ResellerLedgerEntryTypeAdjustment ResellerLedgerEntryType = "adjustment"
)

type ResellerLedgerDirection string

const (
	ResellerLedgerDirectionIncrease ResellerLedgerDirection = "increase"
	ResellerLedgerDirectionDecrease ResellerLedgerDirection = "decrease"
)

// Stable repository errors for expected AR credit failures.
var (
	ErrResellerInsufficientCredit       = errors.New("reseller insufficient credit")
	ErrResellerOverSettlement           = errors.New("reseller over-settlement")
	ErrResellerCreditLimitBelowOwed     = errors.New("reseller credit limit below balance owed")
	ErrResellerLedgerIdempotencyMismatch = errors.New("reseller ledger idempotency payload mismatch")
)

// ResellerCreditAccount is the AR credit account for a reseller customer.
// It is intentionally separate from customer.balance / wallet_transaction.
type ResellerCreditAccount struct {
	CustomerID  int64     `json:"customer_id"`
	CreditLimit float64   `json:"credit_limit"`
	BalanceOwed float64   `json:"balance_owed"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RemainingCredit returns credit_limit - balance_owed, floored at 0.
func (a ResellerCreditAccount) RemainingCredit() float64 {
	rem := a.CreditLimit - a.BalanceOwed
	if rem < 0 {
		return 0
	}
	return rem
}

// ResellerLedgerEntry is an immutable AR ledger row.
type ResellerLedgerEntry struct {
	ID             int64                   `json:"id"`
	CustomerID     int64                   `json:"customer_id"`
	EntryType      ResellerLedgerEntryType  `json:"entry_type"`
	Direction      ResellerLedgerDirection  `json:"direction"`
	Amount         float64                 `json:"amount"`
	PurchaseID     *int64                  `json:"purchase_id,omitempty"`
	EffectiveAt    time.Time               `json:"effective_at"`
	Note           string                  `json:"note"`
	CreatedBy      string                  `json:"created_by"`
	IdempotencyKey string                  `json:"idempotency_key"`
	CreatedAt      time.Time               `json:"created_at"`
}

// CreateLedgerEntryInput is the write payload for sale/settlement/adjustment.
type CreateLedgerEntryInput struct {
	CustomerID     int64
	EntryType      ResellerLedgerEntryType
	Direction      ResellerLedgerDirection
	Amount         float64
	PurchaseID     *int64
	EffectiveAt    time.Time
	Note           string
	CreatedBy      string
	IdempotencyKey string
}

// ResellerCreditRepository manages reseller AR credit accounts and ledger.
type ResellerCreditRepository struct {
	pool *pgxpool.Pool
}

func NewResellerCreditRepository(pool *pgxpool.Pool) *ResellerCreditRepository {
	return &ResellerCreditRepository{pool: pool}
}

// normalizeResellerAmount requires a positive amount and rounds to two decimals
// (half away from zero), matching financial_adjustment money handling.
func normalizeResellerAmount(amount float64) (float64, error) {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("amount must be positive")
	}
	out := roundMoney2(amount)
	if out <= 0 {
		return 0, fmt.Errorf("amount must be positive after rounding")
	}
	return out, nil
}

// validateCreditLimit rejects limit < 0 or limit < balance_owed.
func validateCreditLimit(limit, balanceOwed float64) error {
	if limit < 0 || math.IsNaN(limit) || math.IsInf(limit, 0) {
		return fmt.Errorf("credit limit must be non-negative")
	}
	if roundMoney2(limit) < roundMoney2(balanceOwed) {
		return ErrResellerCreditLimitBelowOwed
	}
	return nil
}

// canIncreaseOwed rejects when balance_owed + amount > credit_limit.
func canIncreaseOwed(balanceOwed, amount, creditLimit float64) error {
	if roundMoney2(balanceOwed+amount) > roundMoney2(creditLimit) {
		return ErrResellerInsufficientCredit
	}
	return nil
}

// canDecreaseOwed rejects when amount > balance_owed.
func canDecreaseOwed(balanceOwed, amount float64) error {
	if roundMoney2(amount) > roundMoney2(balanceOwed) {
		return ErrResellerOverSettlement
	}
	return nil
}

// validateCreateLedgerEntryInput checks type/direction/required fields and
// returns a single normalized amount. requiredDir empty means any direction
// (used for adjustments). requirePurchase forces purchase_id non-nil.
func validateCreateLedgerEntryInput(
	in CreateLedgerEntryInput,
	requiredType ResellerLedgerEntryType,
	requiredDir ResellerLedgerDirection,
	requirePurchase bool,
) (float64, error) {
	if in.CustomerID <= 0 {
		return 0, fmt.Errorf("customer_id is required")
	}
	if in.EntryType != requiredType {
		return 0, fmt.Errorf("entry_type must be %s, got %s", requiredType, in.EntryType)
	}
	if requiredDir != "" && in.Direction != requiredDir {
		return 0, fmt.Errorf("direction must be %s, got %s", requiredDir, in.Direction)
	}
	if in.Direction != ResellerLedgerDirectionIncrease && in.Direction != ResellerLedgerDirectionDecrease {
		return 0, fmt.Errorf("invalid direction: %s", in.Direction)
	}
	if requirePurchase && in.PurchaseID == nil {
		return 0, fmt.Errorf("purchase_id is required for sale")
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return 0, fmt.Errorf("idempotency_key is required")
	}
	if strings.TrimSpace(in.CreatedBy) == "" {
		return 0, fmt.Errorf("created_by is required")
	}
	if in.EffectiveAt.IsZero() {
		return 0, fmt.Errorf("effective_at is required")
	}
	return normalizeResellerAmount(in.Amount)
}

func buildEnsureAccountSQL() string {
	return `
		INSERT INTO reseller_credit_account (customer_id, credit_limit, balance_owed)
		VALUES ($1, $2, 0)
		ON CONFLICT (customer_id) DO NOTHING`
}

func buildGetAccountSQL() string {
	return `
		SELECT customer_id, credit_limit, balance_owed, created_at, updated_at
		FROM reseller_credit_account
		WHERE customer_id = $1`
}

func buildSetCreditLimitSQL() string {
	return `
		UPDATE reseller_credit_account
		SET credit_limit = $2, updated_at = NOW()
		WHERE customer_id = $1
		RETURNING customer_id, credit_limit, balance_owed, created_at, updated_at`
}

func buildLockResellerAccountSQL() string {
	return `
		SELECT customer_id, credit_limit, balance_owed, created_at, updated_at
		FROM reseller_credit_account
		WHERE customer_id = $1
		FOR UPDATE`
}

func buildInsertResellerLedgerEntrySQL() string {
	return `
		INSERT INTO reseller_ledger_entry (
			customer_id, entry_type, direction, amount, purchase_id,
			effective_at, note, created_by, idempotency_key
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, customer_id, entry_type, direction, amount, purchase_id,
		          effective_at, note, created_by, idempotency_key, created_at`
}

func buildLoadResellerLedgerByIdempotencyKeySQL() string {
	return `
		SELECT id, customer_id, entry_type, direction, amount, purchase_id,
		       effective_at, note, created_by, idempotency_key, created_at
		FROM reseller_ledger_entry
		WHERE idempotency_key = $1`
}

// buildUpdateBalanceOwedSQL returns UPDATE that adds (increase=true) or subtracts amount.
func buildUpdateBalanceOwedSQL(increase bool) string {
	op := "+"
	if !increase {
		op = "-"
	}
	return fmt.Sprintf(`
		UPDATE reseller_credit_account
		SET balance_owed = balance_owed %s $2, updated_at = NOW()
		WHERE customer_id = $1
		RETURNING customer_id, credit_limit, balance_owed, created_at, updated_at`, op)
}

func buildListLedgerSQL() string {
	return `
		SELECT id, customer_id, entry_type, direction, amount, purchase_id,
		       effective_at, COALESCE(note, ''), created_by, idempotency_key, created_at
		FROM reseller_ledger_entry
		WHERE customer_id = $1
		ORDER BY effective_at DESC, id DESC
		LIMIT $2 OFFSET $3`
}

func scanResellerCreditAccount(row pgx.Row, dest *ResellerCreditAccount) error {
	return row.Scan(
		&dest.CustomerID, &dest.CreditLimit, &dest.BalanceOwed, &dest.CreatedAt, &dest.UpdatedAt,
	)
}

func scanResellerLedgerEntry(row pgx.Row, dest *ResellerLedgerEntry) error {
	var note *string
	err := row.Scan(
		&dest.ID, &dest.CustomerID, &dest.EntryType, &dest.Direction, &dest.Amount, &dest.PurchaseID,
		&dest.EffectiveAt, &note, &dest.CreatedBy, &dest.IdempotencyKey, &dest.CreatedAt,
	)
	if err != nil {
		return err
	}
	if note != nil {
		dest.Note = *note
	}
	return nil
}

// resellerLedgerPayloadMatches compares identity fields for idempotent replay.
func resellerLedgerPayloadMatches(existing *ResellerLedgerEntry, in CreateLedgerEntryInput, amount float64) bool {
	if existing == nil {
		return false
	}
	if existing.CustomerID != in.CustomerID {
		return false
	}
	if existing.EntryType != in.EntryType {
		return false
	}
	if existing.Direction != in.Direction {
		return false
	}
	if !moneyEqual(existing.Amount, amount) {
		return false
	}
	if !purchaseIDEqual(existing.PurchaseID, in.PurchaseID) {
		return false
	}
	if !effectiveAtEqual(existing.EffectiveAt, in.EffectiveAt) {
		return false
	}
	return true
}

// EnsureAccount inserts a credit account if missing (limit=defaultLimit, owed=0)
// and returns the current row.
func (r *ResellerCreditRepository) EnsureAccount(ctx context.Context, customerID int64, defaultLimit float64) (*ResellerCreditAccount, error) {
	if customerID <= 0 {
		return nil, fmt.Errorf("customer_id is required")
	}
	if defaultLimit < 0 || math.IsNaN(defaultLimit) || math.IsInf(defaultLimit, 0) {
		return nil, fmt.Errorf("default credit limit must be non-negative")
	}
	defaultLimit = roundMoney2(defaultLimit)

	_, err := r.pool.Exec(ctx, buildEnsureAccountSQL(), customerID, defaultLimit)
	if err != nil {
		return nil, fmt.Errorf("ensure reseller_credit_account: %w", err)
	}
	acct, err := r.GetAccount(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, fmt.Errorf("ensure reseller_credit_account: row missing after insert")
	}
	return acct, nil
}

// GetAccount returns the credit account or nil if none exists.
func (r *ResellerCreditRepository) GetAccount(ctx context.Context, customerID int64) (*ResellerCreditAccount, error) {
	acct := &ResellerCreditAccount{}
	err := scanResellerCreditAccount(r.pool.QueryRow(ctx, buildGetAccountSQL(), customerID), acct)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get reseller_credit_account: %w", err)
	}
	return acct, nil
}

// SetCreditLimit ensures the account exists, then sets credit_limit.
// Rejects limit < 0 or limit < balance_owed.
func (r *ResellerCreditRepository) SetCreditLimit(ctx context.Context, customerID int64, limit float64) (*ResellerCreditAccount, error) {
	if customerID <= 0 {
		return nil, fmt.Errorf("customer_id is required")
	}
	if limit < 0 || math.IsNaN(limit) || math.IsInf(limit, 0) {
		return nil, fmt.Errorf("credit limit must be non-negative")
	}
	limit = roundMoney2(limit)

	// Ensure row exists with the requested limit as default if missing.
	if _, err := r.EnsureAccount(ctx, customerID, limit); err != nil {
		return nil, err
	}

	// Re-read to validate against current balance_owed (race-safe enough for admin path;
	// concurrent sales use FOR UPDATE on ledger paths).
	acct, err := r.GetAccount(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, fmt.Errorf("set credit limit: account missing")
	}
	if err := validateCreditLimit(limit, acct.BalanceOwed); err != nil {
		return nil, err
	}

	out := &ResellerCreditAccount{}
	err = scanResellerCreditAccount(r.pool.QueryRow(ctx, buildSetCreditLimitSQL(), customerID, limit), out)
	if err != nil {
		return nil, fmt.Errorf("set credit limit: %w", err)
	}
	return out, nil
}

// recordLedgerTx is the shared sale/settlement/adjustment path under an existing tx.
// increase=true adds to balance_owed; false subtracts. checkCredit applies the
// credit_limit cap on increases (sales); adjustments that increase skip the cap
// when checkCredit is false.
func (r *ResellerCreditRepository) recordLedgerTx(
	ctx context.Context,
	tx pgx.Tx,
	in CreateLedgerEntryInput,
	requiredType ResellerLedgerEntryType,
	requiredDir ResellerLedgerDirection,
	requirePurchase bool,
	increase bool,
	checkCredit bool,
) (*ResellerLedgerEntry, *ResellerCreditAccount, bool, error) {
	amount, err := validateCreateLedgerEntryInput(in, requiredType, requiredDir, requirePurchase)
	if err != nil {
		return nil, nil, false, err
	}
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	createdBy := strings.TrimSpace(in.CreatedBy)
	note := in.Note

	// 1) Lock account row.
	acct := &ResellerCreditAccount{}
	err = scanResellerCreditAccount(tx.QueryRow(ctx, buildLockResellerAccountSQL(), in.CustomerID), acct)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, false, fmt.Errorf("reseller credit account not found for customer %d", in.CustomerID)
		}
		return nil, nil, false, fmt.Errorf("lock reseller_credit_account: %w", err)
	}

	// 2) Insert ledger ON CONFLICT DO NOTHING first so idempotent replays can
	// short-circuit without re-applying credit/settlement checks against the
	// current balance (which may already include this entry's effect).
	entry := &ResellerLedgerEntry{}
	err = scanResellerLedgerEntry(tx.QueryRow(ctx, buildInsertResellerLedgerEntrySQL(),
		in.CustomerID, string(in.EntryType), string(in.Direction), amount, in.PurchaseID,
		in.EffectiveAt, note, createdBy, idempotencyKey,
	), entry)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, false, fmt.Errorf("insert reseller_ledger_entry: %w", err)
		}
		// 4) Conflict: reload by key and compare payload (no balance mutation).
		existing := &ResellerLedgerEntry{}
		err = scanResellerLedgerEntry(tx.QueryRow(ctx, buildLoadResellerLedgerByIdempotencyKeySQL(), idempotencyKey), existing)
		if err != nil {
			return nil, nil, false, fmt.Errorf("load existing reseller_ledger_entry: %w", err)
		}
		if !resellerLedgerPayloadMatches(existing, in, amount) {
			return nil, nil, false, ErrResellerLedgerIdempotencyMismatch
		}
		return existing, acct, false, nil
	}

	// 3) New insert: enforce balance invariants under the same lock, then update.
	// On failure the caller must roll back the tx (ledger row is not committed).
	if increase {
		if checkCredit {
			if err := canIncreaseOwed(acct.BalanceOwed, amount, acct.CreditLimit); err != nil {
				return nil, nil, false, err
			}
		}
	} else {
		if err := canDecreaseOwed(acct.BalanceOwed, amount); err != nil {
			return nil, nil, false, err
		}
	}

	// 5) Update balance_owed only when a new row was inserted.
	updated := &ResellerCreditAccount{}
	err = scanResellerCreditAccount(tx.QueryRow(ctx, buildUpdateBalanceOwedSQL(increase), in.CustomerID, amount), updated)
	if err != nil {
		return nil, nil, false, fmt.Errorf("update balance_owed: %w", err)
	}
	return entry, updated, true, nil
}

// RecordSaleTx records a postpaid sale: increase balance_owed under account lock.
// Requires entry_type=sale, direction=increase, purchase_id set, amount>0.
// Rejects when balance_owed+amount > credit_limit.
func (r *ResellerCreditRepository) RecordSaleTx(
	ctx context.Context,
	tx pgx.Tx,
	in CreateLedgerEntryInput,
) (*ResellerLedgerEntry, *ResellerCreditAccount, bool, error) {
	return r.recordLedgerTx(
		ctx, tx, in,
		ResellerLedgerEntryTypeSale, ResellerLedgerDirectionIncrease,
		true,  // require purchase
		true,  // increase owed
		true,  // check credit limit
	)
}

// RecordSettlementTx records a settlement: decrease balance_owed under account lock.
// Requires entry_type=settlement, direction=decrease; amount <= balance_owed.
func (r *ResellerCreditRepository) RecordSettlementTx(
	ctx context.Context,
	tx pgx.Tx,
	in CreateLedgerEntryInput,
) (*ResellerLedgerEntry, *ResellerCreditAccount, bool, error) {
	return r.recordLedgerTx(
		ctx, tx, in,
		ResellerLedgerEntryTypeSettlement, ResellerLedgerDirectionDecrease,
		false, // purchase optional
		false, // decrease owed
		false, // no credit check
	)
}

// RecordAdjustmentTx records an admin adjustment.
// increase adds owed (no credit-limit cap); decrease requires amount <= owed.
func (r *ResellerCreditRepository) RecordAdjustmentTx(
	ctx context.Context,
	tx pgx.Tx,
	in CreateLedgerEntryInput,
) (*ResellerLedgerEntry, *ResellerCreditAccount, bool, error) {
	// Direction is taken from input; empty requiredDir allows either.
	increase := in.Direction == ResellerLedgerDirectionIncrease
	return r.recordLedgerTx(
		ctx, tx, in,
		ResellerLedgerEntryTypeAdjustment, "", // any direction
		false,
		increase,
		false, // no credit-limit cap on adjustment increase
	)
}

// ListLedger returns paginated ledger entries for a customer (newest first).
func (r *ResellerCreditRepository) ListLedger(ctx context.Context, customerID int64, limit, offset int) ([]ResellerLedgerEntry, error) {
	if customerID <= 0 {
		return nil, fmt.Errorf("customer_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := r.pool.Query(ctx, buildListLedgerSQL(), customerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reseller_ledger_entry: %w", err)
	}
	defer rows.Close()

	var out []ResellerLedgerEntry
	for rows.Next() {
		var e ResellerLedgerEntry
		var note string
		if err := rows.Scan(
			&e.ID, &e.CustomerID, &e.EntryType, &e.Direction, &e.Amount, &e.PurchaseID,
			&e.EffectiveAt, &note, &e.CreatedBy, &e.IdempotencyKey, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		e.Note = note
		out = append(out, e)
	}
	return out, rows.Err()
}

// SettlementPeriodRow is a Yangon-bucketed sum of AR settlement ledger entries.
// Currency is always MMK for AR v1 (ledger has no currency column).
type SettlementPeriodRow struct {
	PeriodStart     string  // YYYY-MM-DD Yangon
	Currency        string  // always "MMK" for AR v1
	SettlementTotal float64
	SettlementCount int
}

// buildSumSettlementsByPeriodSQL groups settlement ledger entries by Yangon period buckets.
// Half-open [start, end). entry_type = 'settlement' only. Currency hardcoded to MMK.
func buildSumSettlementsByPeriodSQL(period RevenueSummaryPeriod) (string, error) {
	var bucket string
	switch period {
	case RevenuePeriodDay, RevenuePeriodCustom:
		bucket = `(rle.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodWeek:
		bucket = `DATE_TRUNC('week', rle.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodMonth:
		bucket = `DATE_TRUNC('month', rle.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodYear:
		bucket = `DATE_TRUNC('year', rle.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	default:
		return "", fmt.Errorf("unsupported revenue period: %s", period)
	}
	return fmt.Sprintf(`
		SELECT
			%s AS period_start,
			'MMK' AS currency,
			COALESCE(SUM(rle.amount), 0) AS settlement_total,
			COUNT(*) AS settlement_count
		FROM reseller_ledger_entry rle
		WHERE rle.entry_type = 'settlement'
		  AND rle.effective_at >= $1
		  AND rle.effective_at < $2
		GROUP BY 1, 2
		ORDER BY 1 ASC`, bucket), nil
}

// SumSettlementsByPeriod groups settlement ledger entries by Yangon period buckets.
// Half-open [start, end). entry_type = 'settlement' only.
func (r *ResellerCreditRepository) SumSettlementsByPeriod(ctx context.Context, start, end time.Time, period RevenueSummaryPeriod) ([]SettlementPeriodRow, error) {
	query, err := buildSumSettlementsByPeriodSQL(period)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, query, start, end)
	if err != nil {
		return nil, fmt.Errorf("sum settlements by period: %w", err)
	}
	defer rows.Close()
	var out []SettlementPeriodRow
	for rows.Next() {
		var sr SettlementPeriodRow
		var periodStart time.Time
		if err := rows.Scan(&periodStart, &sr.Currency, &sr.SettlementTotal, &sr.SettlementCount); err != nil {
			return nil, err
		}
		sr.PeriodStart = periodStart.Format("2006-01-02")
		out = append(out, sr)
	}
	return out, rows.Err()
}
