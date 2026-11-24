package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type fakeStaleProcessingStore struct {
	stale     []database.Purchase
	reopened  map[int64]bool
	reopenErr error
	cutoffs   []time.Time
	finalized map[int64]bool
}

func (f *fakeStaleProcessingStore) FindStaleProcessing(_ context.Context, cutoff time.Time, _ int) ([]database.Purchase, error) {
	f.cutoffs = append(f.cutoffs, cutoff)
	return f.stale, nil
}

func (f *fakeStaleProcessingStore) ReturnStaleProcessingToPending(_ context.Context, purchaseID int64, _ time.Time) (bool, error) {
	if f.reopenErr != nil {
		return false, f.reopenErr
	}
	if f.reopened == nil {
		f.reopened = map[int64]bool{}
	}
	f.reopened[purchaseID] = true
	return true, nil
}

func TestStaleProcessingReaperReturnsScreenshotFlowToPending(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeStaleProcessingStore{
		stale: []database.Purchase{
			{ID: 11, CustomerID: 3, InvoiceType: database.InvoiceTypeMobileBanking},
			{ID: 12, CustomerID: 4, InvoiceType: database.InvoiceTypeWalletTopUp},
		},
	}

	reaper := newStaleProcessingReaper(store, nil, func() time.Time { return now })
	result, err := reaper.reap(context.Background())
	if err != nil {
		t.Fatalf("reap() error = %v", err)
	}

	if result.Scanned != 2 {
		t.Fatalf("Scanned = %d, want 2", result.Scanned)
	}
	if result.Reopened != 2 {
		t.Fatalf("Reopened = %d, want 2 (both rows are screenshot-flow)", result.Reopened)
	}
	if result.Finalized != 0 {
		t.Fatalf("Finalized = %d, want 0 without a finalize hook", result.Finalized)
	}
	if len(store.cutoffs) != 1 {
		t.Fatalf("cutoffs recorded = %d, want 1", len(store.cutoffs))
	}
	if want := now.Add(-staleProcessingThreshold); !store.cutoffs[0].Equal(want) {
		t.Fatalf("cutoff = %v, want %v", store.cutoffs[0], want)
	}
}

func TestStaleProcessingReaperFinalizesSettledWalletTopUp(t *testing.T) {
	store := &fakeStaleProcessingStore{
		stale: []database.Purchase{
			{ID: 21, CustomerID: 5, InvoiceType: database.InvoiceTypeWalletTopUp},
		},
	}
	var finalized []int64

	reaper := newStaleProcessingReaper(store, func(_ context.Context, purchaseID int64) error {
		finalized = append(finalized, purchaseID)
		return nil
	}, time.Now)

	result, err := reaper.reap(context.Background())
	if err != nil {
		t.Fatalf("reap() error = %v", err)
	}

	if len(finalized) != 1 || finalized[0] != 21 {
		t.Fatalf("finalized = %v, want [21]", finalized)
	}
	if result.Finalized != 1 || result.Reopened != 0 {
		t.Fatalf("result = %+v, want 1 finalized and 0 reopened", result)
	}
	if store.reopened[21] {
		t.Fatal("settled wallet top-up must not be returned to pending: the customer could be credited twice")
	}
}

func TestStaleProcessingReaperReopensUnsettledWalletTopUp(t *testing.T) {
	store := &fakeStaleProcessingStore{
		stale: []database.Purchase{
			{ID: 31, CustomerID: 6, InvoiceType: database.InvoiceTypeWalletTopUp},
		},
	}

	reaper := newStaleProcessingReaper(store, func(context.Context, int64) error {
		return ErrPurchaseFinalizationEvidenceMissing
	}, time.Now)

	result, err := reaper.reap(context.Background())
	if err != nil {
		t.Fatalf("reap() error = %v", err)
	}

	if result.Finalized != 0 || result.Reopened != 1 {
		t.Fatalf("result = %+v, want 0 finalized and 1 reopened", result)
	}
	if !store.reopened[31] {
		t.Fatal("unsettled wallet top-up should return to pending so the customer can retry")
	}
}

func TestStaleProcessingReaperCountsFinalizeFailures(t *testing.T) {
	store := &fakeStaleProcessingStore{
		stale: []database.Purchase{
			{ID: 41, CustomerID: 7, InvoiceType: database.InvoiceTypeWalletTopUp},
		},
	}

	reaper := newStaleProcessingReaper(store, func(context.Context, int64) error {
		return errors.New("database is down")
	}, time.Now)

	result, err := reaper.reap(context.Background())
	if err != nil {
		t.Fatalf("reap() error = %v", err)
	}

	if result.Failed != 1 || result.Reopened != 0 || result.Finalized != 0 {
		t.Fatalf("result = %+v, want 1 failed and no transitions", result)
	}
	if store.reopened[41] {
		t.Fatal("an unexpected finalize error must not reopen the row; the ledger state is unknown")
	}
}

func TestStaleProcessingThresholdCoversLiveVerification(t *testing.T) {
	// Screenshot verification holds a processing claim for at most 60 s and the
	// Remnawave client times out after 15 s. The reaper must never fire inside a
	// live attempt.
	if staleProcessingThreshold <= 60*time.Second {
		t.Fatalf("staleProcessingThreshold = %v, want longer than the 60s verification budget", staleProcessingThreshold)
	}
}
