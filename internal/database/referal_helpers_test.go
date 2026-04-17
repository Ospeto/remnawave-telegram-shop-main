package database

import (
	"context"
	"errors"
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
