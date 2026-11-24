package database

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Referral struct {
	ID                    int64      `db:"id"`
	ReferrerID            int64      `db:"referrer_id"`
	RefereeID             int64      `db:"referee_id"`
	UsedAt                time.Time  `db:"used_at"`
	BonusGranted          bool       `db:"bonus_granted"`
	BonusGrantedAt        *time.Time `db:"bonus_granted_at"`
	RefereeBonusGranted   bool       `db:"referee_bonus_granted"`
	RefereeBonusGrantedAt *time.Time `db:"referee_bonus_granted_at"`
}

type ReferralIdentityResolver interface {
	FindById(context.Context, int64) (*Customer, error)
	FindByTelegramId(context.Context, int64) (*Customer, error)
}

type ReferralRepository struct {
	pool *pgxpool.Pool
}

func NewReferralRepository(pool *pgxpool.Pool) *ReferralRepository {
	return &ReferralRepository{pool: pool}
}

func ReferralIdentityValues(ids ...int64) []int64 {
	if len(ids) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(ids))
	values := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		values = append(values, id)
	}
	return values
}

func ResolveReferralCustomer(ctx context.Context, resolver ReferralIdentityResolver, referralIdentity int64) (*Customer, error) {
	if resolver == nil || referralIdentity == 0 {
		return nil, nil
	}

	customer, err := resolver.FindById(ctx, referralIdentity)
	if err != nil {
		return nil, err
	}
	if customer != nil {
		return customer, nil
	}

	customer, err = resolver.FindByTelegramId(ctx, referralIdentity)
	if err != nil {
		return nil, err
	}
	return customer, nil
}

func ReferralActivityAt(ref Referral) time.Time {
	if ref.BonusGranted && ref.BonusGrantedAt != nil {
		return *ref.BonusGrantedAt
	}
	return ref.UsedAt
}

func SelectPreferredReferral(referrals []Referral) *Referral {
	if len(referrals) == 0 {
		return nil
	}

	ordered := make([]Referral, len(referrals))
	copy(ordered, referrals)
	sort.SliceStable(ordered, func(i, j int) bool {
		iGranted := ordered[i].BonusGranted || ordered[i].RefereeBonusGranted
		jGranted := ordered[j].BonusGranted || ordered[j].RefereeBonusGranted
		if iGranted != jGranted {
			return iGranted
		}
		if !ordered[i].UsedAt.Equal(ordered[j].UsedAt) {
			return ordered[i].UsedAt.Before(ordered[j].UsedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})

	selected := ordered[0]
	return &selected
}

func NormalizeReferralsByReferee(ctx context.Context, referrals []Referral, resolver ReferralIdentityResolver) ([]Referral, error) {
	if len(referrals) == 0 {
		return nil, nil
	}

	resolvedIDs := make(map[int64]int64, len(referrals))
	resolvedCustomers := make(map[int64]*Customer, len(referrals))
	for _, ref := range referrals {
		normalizedID := ref.RefereeID
		if _, ok := resolvedIDs[ref.RefereeID]; !ok {
			customer, ok := resolvedCustomers[ref.RefereeID]
			if !ok {
				var err error
				customer, err = ResolveReferralCustomer(ctx, resolver, ref.RefereeID)
				if err != nil {
					return nil, err
				}
				resolvedCustomers[ref.RefereeID] = customer
			}
			if customer != nil {
				normalizedID = customer.ID
			}
			resolvedIDs[ref.RefereeID] = normalizedID
		}
	}

	return normalizeReferralsByResolvedID(referrals, func(ref Referral) int64 {
		if id, ok := resolvedIDs[ref.RefereeID]; ok {
			return id
		}
		return ref.RefereeID
	}), nil
}

// ReferralWithReferee is a referral row joined with the customer identity of its referee.
// The join exists because the schema carries two referee identities (customer_id and
// telegram_id), so resolving each referee took up to two extra queries per row.
type ReferralWithReferee struct {
	Referral
	RefereeCustomerID int64 `db:"referee_customer_id"`
	RefereeTelegramID int64 `db:"referee_telegram_id"`
}

// NormalizeReferralsWithResolvedReferee deduplicates referrals by the referee customer
// identity already resolved by the JOIN, so no per-referee lookups are needed.
func NormalizeReferralsWithResolvedReferee(referrals []ReferralWithReferee) []Referral {
	if len(referrals) == 0 {
		return nil
	}

	refs := make([]Referral, len(referrals))
	resolvedIDs := make(map[int64]int64, len(referrals))
	for i, ref := range referrals {
		refs[i] = ref.Referral
		id := ref.RefereeCustomerID
		if id == 0 {
			id = ref.RefereeID
		}
		resolvedIDs[ref.Referral.ID] = id
	}

	return normalizeReferralsByResolvedID(refs, func(ref Referral) int64 {
		if id, ok := resolvedIDs[ref.ID]; ok {
			return id
		}
		return ref.RefereeID
	})
}

func normalizeReferralsByResolvedID(referrals []Referral, resolvedID func(Referral) int64) []Referral {
	grouped := make(map[int64][]Referral, len(referrals))
	for _, ref := range referrals {
		normalizedID := resolvedID(ref)
		if normalizedID == 0 {
			normalizedID = ref.RefereeID
		}
		grouped[normalizedID] = append(grouped[normalizedID], ref)
	}

	normalized := make([]Referral, 0, len(grouped))
	for _, refs := range grouped {
		if preferred := SelectPreferredReferral(refs); preferred != nil {
			normalized = append(normalized, *preferred)
		}
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		iTime := ReferralActivityAt(normalized[i])
		jTime := ReferralActivityAt(normalized[j])
		if !iTime.Equal(jTime) {
			return iTime.After(jTime)
		}
		return normalized[i].ID > normalized[j].ID
	})

	return normalized
}

func (r *ReferralRepository) Create(ctx context.Context, referrerID, refereeID int64) (*Referral, error) {
	query := sq.Insert("referral").
		Columns("referrer_id", "referee_id", "used_at", "bonus_granted", "bonus_granted_at", "referee_bonus_granted", "referee_bonus_granted_at").
		Values(referrerID, refereeID, sq.Expr("NOW()"), false, nil, false, nil).
		Suffix("RETURNING id, referrer_id, referee_id, used_at, bonus_granted, bonus_granted_at, referee_bonus_granted, referee_bonus_granted_at").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build insert referral query: %w", err)
	}

	row := r.pool.QueryRow(ctx, sql, args...)
	var ref Referral
	if err := row.Scan(
		&ref.ID,
		&ref.ReferrerID,
		&ref.RefereeID,
		&ref.UsedAt,
		&ref.BonusGranted,
		&ref.BonusGrantedAt,
		&ref.RefereeBonusGranted,
		&ref.RefereeBonusGrantedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to scan inserted referral: %w", err)
	}
	return &ref, nil
}

func (r *ReferralRepository) FindByReferrer(ctx context.Context, referrerID int64) ([]Referral, error) {
	query := sq.Select("id", "referrer_id", "referee_id", "used_at", "bonus_granted", "bonus_granted_at", "referee_bonus_granted", "referee_bonus_granted_at").
		From("referral").
		Where(sq.Eq{"referrer_id": referrerID}).
		OrderBy("used_at DESC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referrals by referrer query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query referrals by referrer: %w", err)
	}
	defer rows.Close()

	var list []Referral
	for rows.Next() {
		var ref Referral
		if err := rows.Scan(
			&ref.ID,
			&ref.ReferrerID,
			&ref.RefereeID,
			&ref.UsedAt,
			&ref.BonusGranted,
			&ref.BonusGrantedAt,
			&ref.RefereeBonusGranted,
			&ref.RefereeBonusGrantedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan referral row: %w", err)
		}
		list = append(list, ref)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referral rows: %w", rows.Err())
	}
	return list, nil
}

func (r *ReferralRepository) FindByReferrerAny(ctx context.Context, referrerIDs ...int64) ([]Referral, error) {
	return r.findByIdentityColumn(ctx, "referrer_id", referrerIDs)
}

func (r *ReferralRepository) CountByReferrer(ctx context.Context, referrerID int64) (int, error) {
	query := sq.Select("COUNT(*)").
		From("referral").
		Where(sq.Eq{"referrer_id": referrerID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count referrals by referrer query: %w", err)
	}

	var count int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to scan count of referrals: %w", err)
	}
	return count, nil
}

func buildReferralsWithResolvedRefereeSelect(identityIDs []int64) sq.SelectBuilder {
	return sq.Select(
		"r.id", "r.referrer_id", "r.referee_id", "r.used_at", "r.bonus_granted", "r.bonus_granted_at",
		"r.referee_bonus_granted", "r.referee_bonus_granted_at",
		"COALESCE(c1.id, c2.id, r.referee_id) AS referee_customer_id",
		"COALESCE(c1.telegram_id, c2.telegram_id, r.referee_id) AS referee_telegram_id",
	).
		From("referral r").
		LeftJoin("customer c1 ON c1.id = r.referee_id").
		LeftJoin("customer c2 ON c1.id IS NULL AND c2.telegram_id = r.referee_id").
		Where(sq.Eq{"r.referrer_id": identityIDs}).
		OrderBy("r.used_at DESC", "r.id DESC").
		PlaceholderFormat(sq.Dollar)
}

func (r *ReferralRepository) FindByReferee(ctx context.Context, refereeID int64) (*Referral, error) {
	query := sq.Select("id", "referrer_id", "referee_id", "used_at", "bonus_granted", "bonus_granted_at", "referee_bonus_granted", "referee_bonus_granted_at").
		From("referral").
		Where(sq.Eq{"referee_id": refereeID}).
		Limit(1).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referral by referee query: %w", err)
	}

	var ref Referral
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&ref.ID,
		&ref.ReferrerID,
		&ref.RefereeID,
		&ref.UsedAt,
		&ref.BonusGranted,
		&ref.BonusGrantedAt,
		&ref.RefereeBonusGranted,
		&ref.RefereeBonusGrantedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query referral by referee: %w", err)
	}
	return &ref, nil
}

// FindByReferrerWithResolvedReferee loads a referrer's referrals in ONE query, resolving each
// referee to its customer row through a LEFT JOIN instead of a per-referee lookup loop.
// A referee identity matches by customer.id first and falls back to customer.telegram_id
// (legacy rows), exactly like ResolveReferralCustomer, and the raw identity is kept when
// neither matches.
func (r *ReferralRepository) FindByReferrerWithResolvedReferee(ctx context.Context, referrerIDs ...int64) ([]ReferralWithReferee, error) {
	identityIDs := ReferralIdentityValues(referrerIDs...)
	if len(identityIDs) == 0 {
		return nil, nil
	}

	query := buildReferralsWithResolvedRefereeSelect(identityIDs)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referrals with resolved referee query: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query referrals with resolved referee: %w", err)
	}
	defer rows.Close()

	var list []ReferralWithReferee
	for rows.Next() {
		var ref ReferralWithReferee
		if err := rows.Scan(
			&ref.ID,
			&ref.ReferrerID,
			&ref.RefereeID,
			&ref.UsedAt,
			&ref.BonusGranted,
			&ref.BonusGrantedAt,
			&ref.RefereeBonusGranted,
			&ref.RefereeBonusGrantedAt,
			&ref.RefereeCustomerID,
			&ref.RefereeTelegramID,
		); err != nil {
			return nil, fmt.Errorf("failed to scan referral with resolved referee: %w", err)
		}
		list = append(list, ref)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referrals with resolved referee: %w", rows.Err())
	}

	return list, nil
}

func (r *ReferralRepository) FindByRefereeAny(ctx context.Context, refereeIDs ...int64) ([]Referral, error) {
	return r.findByIdentityColumn(ctx, "referee_id", refereeIDs)
}

func (r *ReferralRepository) findByIdentityColumn(ctx context.Context, column string, identityIDs []int64) ([]Referral, error) {
	identityIDs = ReferralIdentityValues(identityIDs...)
	if len(identityIDs) == 0 {
		return nil, nil
	}

	query := sq.Select("id", "referrer_id", "referee_id", "used_at", "bonus_granted", "bonus_granted_at", "referee_bonus_granted", "referee_bonus_granted_at").
		From("referral").
		Where(sq.Eq{column: identityIDs}).
		OrderBy("used_at DESC", "id DESC").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select referrals by %s query: %w", column, err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query referrals by %s: %w", column, err)
	}
	defer rows.Close()

	var list []Referral
	for rows.Next() {
		var ref Referral
		if err := rows.Scan(
			&ref.ID,
			&ref.ReferrerID,
			&ref.RefereeID,
			&ref.UsedAt,
			&ref.BonusGranted,
			&ref.BonusGrantedAt,
			&ref.RefereeBonusGranted,
			&ref.RefereeBonusGrantedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan referral row: %w", err)
		}
		list = append(list, ref)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating referral rows: %w", rows.Err())
	}

	return list, nil
}

func (r *ReferralRepository) MarkBonusGranted(ctx context.Context, referralID int64) error {
	query := sq.Update("referral").
		Set("bonus_granted", true).
		Set("bonus_granted_at", sq.Expr("COALESCE(bonus_granted_at, NOW())")).
		Where(sq.Eq{"id": referralID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update bonus_granted query: %w", err)
	}

	res, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update bonus_granted: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("no referral record updated")
	}
	return nil
}

// MarkRefereeBonusGranted marks that the referee (new user) has received their welcome bonus.
func (r *ReferralRepository) MarkRefereeBonusGranted(ctx context.Context, referralID int64) error {
	query := sq.Update("referral").
		Set("referee_bonus_granted", true).
		Set("referee_bonus_granted_at", sq.Expr("COALESCE(referee_bonus_granted_at, NOW())")).
		Where(sq.Eq{"id": referralID}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update referee_bonus_granted query: %w", err)
	}

	res, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update referee_bonus_granted: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("no referral record updated for referee bonus")
	}
	return nil
}

// TryMarkBonusGrantedTx atomically claims the referrer bonus in an existing transaction.
// Returns true only for the first claimant.
func (r *ReferralRepository) TryMarkBonusGrantedTx(ctx context.Context, tx pgx.Tx, referralID int64) (bool, error) {
	tag, err := tx.Exec(ctx, `
			UPDATE referral
			SET bonus_granted = TRUE,
			    bonus_granted_at = COALESCE(bonus_granted_at, NOW())
			WHERE id = $1 AND bonus_granted = FALSE
		`, referralID)
	if err != nil {
		return false, fmt.Errorf("failed to claim referrer bonus: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// TryMarkRefereeBonusGrantedTx atomically claims the referee bonus in an existing transaction.
// Returns true only for the first claimant.
func (r *ReferralRepository) TryMarkRefereeBonusGrantedTx(ctx context.Context, tx pgx.Tx, referralID int64) (bool, error) {
	tag, err := tx.Exec(ctx, `
			UPDATE referral
			SET referee_bonus_granted = TRUE,
			    referee_bonus_granted_at = COALESCE(referee_bonus_granted_at, NOW())
			WHERE id = $1 AND referee_bonus_granted = FALSE
		`, referralID)
	if err != nil {
		return false, fmt.Errorf("failed to claim referee bonus: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
