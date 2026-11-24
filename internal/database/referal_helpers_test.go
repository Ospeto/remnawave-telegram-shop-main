package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeReferralIdentityResolver struct {
	byID         map[int64]*Customer
	byTelegramID map[int64]*Customer
	errByID      error
	errByTG      error
}

func (f fakeReferralIdentityResolver) FindById(_ context.Context, id int64) (*Customer, error) {
	if f.errByID != nil {
		return nil, f.errByID
	}
	return f.byID[id], nil
}

func (f fakeReferralIdentityResolver) FindByTelegramId(_ context.Context, telegramID int64) (*Customer, error) {
	if f.errByTG != nil {
		return nil, f.errByTG
	}
	return f.byTelegramID[telegramID], nil
}

func TestResolveReferralCustomerFallsBackToTelegramID(t *testing.T) {
	resolver := fakeReferralIdentityResolver{
		byID: map[int64]*Customer{},
		byTelegramID: map[int64]*Customer{
			999001: {ID: 42, TelegramID: 999001},
		},
	}

	customer, err := ResolveReferralCustomer(context.Background(), resolver, 999001)
	if err != nil {
		t.Fatalf("ResolveReferralCustomer() error = %v", err)
	}
	if customer == nil || customer.ID != 42 {
		t.Fatalf("ResolveReferralCustomer() = %#v, want customer ID 42", customer)
	}
}

func TestResolveReferralCustomerReturnsLookupError(t *testing.T) {
	wantErr := errors.New("db down")
	resolver := fakeReferralIdentityResolver{errByID: wantErr}

	_, err := ResolveReferralCustomer(context.Background(), resolver, 1)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResolveReferralCustomer() error = %v, want %v", err, wantErr)
	}
}

func TestSelectPreferredReferralPrefersGrantedReferral(t *testing.T) {
	now := time.Now().UTC()
	older := now.Add(-time.Hour)
	refs := []Referral{
		{ID: 2, RefereeID: 1001, UsedAt: now},
		{ID: 1, RefereeID: 1001, UsedAt: older, BonusGranted: true},
	}

	got := SelectPreferredReferral(refs)
	if got == nil || got.ID != 1 {
		t.Fatalf("SelectPreferredReferral() = %#v, want referral ID 1", got)
	}
}

func TestNormalizeReferralsByRefereeDeduplicatesLegacyAndCurrentRows(t *testing.T) {
	now := time.Now().UTC()
	refs := []Referral{
		{ID: 1, ReferrerID: 77, RefereeID: 1001, UsedAt: now.Add(-2 * time.Hour)},
		{ID: 2, ReferrerID: 77, RefereeID: 9000001, UsedAt: now.Add(-time.Hour), BonusGranted: true},
	}
	resolver := fakeReferralIdentityResolver{
		byID: map[int64]*Customer{
			1001: {ID: 1001, TelegramID: 9000001},
		},
		byTelegramID: map[int64]*Customer{
			9000001: {ID: 1001, TelegramID: 9000001},
		},
	}

	got, err := NormalizeReferralsByReferee(context.Background(), refs, resolver)
	if err != nil {
		t.Fatalf("NormalizeReferralsByReferee() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("NormalizeReferralsByReferee() len = %d, want 1", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("NormalizeReferralsByReferee() kept ID %d, want granted referral ID 2", got[0].ID)
	}
}

func TestNormalizeReferralsWithResolvedRefereeUsesJoinIdentity(t *testing.T) {
	now := time.Now().UTC()
	// A legacy row stores the referee's telegram_id, a newer row stores its customer id;
	// the JOIN resolves both to the same customer, so they must collapse into one entry.
	referrals := []ReferralWithReferee{
		{
			Referral:          Referral{ID: 1, ReferrerID: 77, RefereeID: 9000001, UsedAt: now.Add(-2 * time.Hour)},
			RefereeCustomerID: 1001,
			RefereeTelegramID: 9000001,
		},
		{
			Referral:          Referral{ID: 2, ReferrerID: 77, RefereeID: 1001, UsedAt: now.Add(-time.Hour), BonusGranted: true},
			RefereeCustomerID: 1001,
			RefereeTelegramID: 9000001,
		},
	}

	got := NormalizeReferralsWithResolvedReferee(referrals)
	if len(got) != 1 {
		t.Fatalf("NormalizeReferralsWithResolvedReferee() len = %d, want 1", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("NormalizeReferralsWithResolvedReferee() kept ID %d, want granted referral ID 2", got[0].ID)
	}
}

func TestNormalizeReferralsWithResolvedRefereeKeepsUnresolvedIdentity(t *testing.T) {
	now := time.Now().UTC()
	// A referral whose referee row no longer exists keeps its raw identity and stays its own entry.
	referrals := []ReferralWithReferee{
		{
			Referral:          Referral{ID: 3, ReferrerID: 77, RefereeID: 4242, UsedAt: now},
			RefereeCustomerID: 4242,
			RefereeTelegramID: 4242,
		},
		{
			Referral:          Referral{ID: 4, ReferrerID: 77, RefereeID: 4243, UsedAt: now.Add(-time.Hour)},
			RefereeCustomerID: 4243,
			RefereeTelegramID: 4243,
		},
	}

	got := NormalizeReferralsWithResolvedReferee(referrals)
	if len(got) != 2 {
		t.Fatalf("NormalizeReferralsWithResolvedReferee() len = %d, want 2 distinct referees", len(got))
	}
}

func TestNormalizeReferralsByRefereeMatchesJoinFromSingleLookup(t *testing.T) {
	now := time.Now().UTC()
	refs := []Referral{
		{ID: 1, ReferrerID: 77, RefereeID: 9000001, UsedAt: now.Add(-2 * time.Hour)},
		{ID: 2, ReferrerID: 77, RefereeID: 9000001, UsedAt: now.Add(-time.Hour), BonusGranted: true},
		{ID: 3, ReferrerID: 77, RefereeID: 1002, UsedAt: now},
	}
	counting := &countingReferralIdentityResolver{
		inner: fakeReferralIdentityResolver{
			byID: map[int64]*Customer{1002: {ID: 1002, TelegramID: 9000002}},
			byTelegramID: map[int64]*Customer{
				9000001: {ID: 1001, TelegramID: 9000001},
				9000002: {ID: 1002, TelegramID: 9000002},
			},
		},
	}

	got, err := NormalizeReferralsByReferee(context.Background(), refs, counting)
	if err != nil {
		t.Fatalf("NormalizeReferralsByReferee() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("NormalizeReferralsByReferee() len = %d, want 2", len(got))
	}
	// Each distinct referee is resolved once: one by-id hit for 1002 and two by-telegram-id
	// fallbacks for 9000001 — three lookups total, never one pair per row.
	if counting.calls != 3 {
		t.Fatalf("resolver lookups = %d, want 3 (one resolve per distinct referee)", counting.calls)
	}
}

type countingReferralIdentityResolver struct {
	inner fakeReferralIdentityResolver
	calls int
}

func (c *countingReferralIdentityResolver) FindById(ctx context.Context, id int64) (*Customer, error) {
	c.calls++
	return c.inner.FindById(ctx, id)
}

func (c *countingReferralIdentityResolver) FindByTelegramId(ctx context.Context, telegramID int64) (*Customer, error) {
	c.calls++
	return c.inner.FindByTelegramId(ctx, telegramID)
}

// TestReferralsWithResolvedRefereeQueryIsOneJoinedStatement pins the P3 fix: the referee
// identity must come from a single JOIN against customer, not from a per-referee lookup loop,
// so one query replaces the previous 4+N fan-out.
func TestReferralsWithResolvedRefereeQueryIsOneJoinedStatement(t *testing.T) {
	sql, args, err := buildReferralsWithResolvedRefereeSelect([]int64{1201, 9000001201}).ToSql()
	if err != nil {
		t.Fatalf("buildReferralsWithResolvedRefereeSelect().ToSql() error = %v", err)
	}

	for _, want := range []string{
		"FROM referral r",
		"LEFT JOIN customer c1 ON c1.id = r.referee_id",
		"LEFT JOIN customer c2 ON c1.id IS NULL AND c2.telegram_id = r.referee_id",
		"COALESCE(c1.id, c2.id, r.referee_id) AS referee_customer_id",
		"COALESCE(c1.telegram_id, c2.telegram_id, r.referee_id) AS referee_telegram_id",
		"ORDER BY r.used_at DESC, r.id DESC",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("joined referral query missing %q; got:\n%s", want, sql)
		}
	}
	if got := strings.Count(sql, "SELECT"); got != 1 {
		t.Errorf("joined referral query has %d SELECT statements, want exactly 1", got)
	}
	if len(args) != 2 {
		t.Fatalf("referrer identity arg count = %d, want 2 (one placeholder per identity, still ONE statement)", len(args))
	}
}
