package payment

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// staleProcessingThreshold is how long a purchase may sit in 'processing'
// before the reaper treats the claim as abandoned.
//
// Screenshot verification holds a claim for at most 60 s and Remnawave calls
// time out after 15 s, so five minutes is comfortably past any live attempt
// while still short enough that a customer is not stuck for long.
const staleProcessingThreshold = 5 * time.Minute

// staleProcessingStore is the narrow persistence surface the reaper needs.
type staleProcessingStore interface {
	FindStaleProcessing(ctx context.Context, cutoff time.Time, limit int) ([]database.Purchase, error)
	ReturnStaleProcessingToPending(ctx context.Context, purchaseID int64, cutoff time.Time) (bool, error)
}

// staleProcessingReaperResult summarises one reaper pass.
type staleProcessingReaperResult struct {
	Scanned   int
	Finalized int
	Reopened  int
	Failed    int
}

// staleProcessingReaper returns abandoned 'processing' purchases to a state the
// customer can act on.
//
// A9: before this existed, a purchase whose fulfillment worker died stayed in
// 'processing' forever: the Telegram screenshot flow only accepts pending/new
// purchases and ProcessPurchaseById refuses re-claiming a processing row, so
// the customer could neither retry nor cancel it.
//
// Safety rule: a row is only returned to pending when it is a screenshot-flow
// (mobile_banking / wallet_topup) purchase with no recorded receipt
// verification; the SQL predicate enforces that. A purchase that actually
// succeeded is already 'paid' and is never selected. A wallet top-up with
// ledger proof of settlement is finalized as paid instead of reopened, because
// reopening it would let the customer trigger a second credit.
type staleProcessingReaper struct {
	store    staleProcessingStore
	finalize func(ctx context.Context, purchaseID int64) error
	now      func() time.Time
	limit    int
}

func newStaleProcessingReaper(store staleProcessingStore, finalize func(context.Context, int64) error, now func() time.Time) *staleProcessingReaper {
	if now == nil {
		now = time.Now
	}
	return &staleProcessingReaper{store: store, finalize: finalize, now: now, limit: 200}
}

func (r *staleProcessingReaper) reap(ctx context.Context) (staleProcessingReaperResult, error) {
	var result staleProcessingReaperResult
	if r == nil || r.store == nil {
		return result, nil
	}

	cutoff := r.now().Add(-staleProcessingThreshold)
	purchases, err := r.store.FindStaleProcessing(ctx, cutoff, r.limit)
	if err != nil {
		return result, err
	}
	result.Scanned = len(purchases)

	for i := range purchases {
		purchase := purchases[i]

		if purchase.InvoiceType == database.InvoiceTypeWalletTopUp && r.finalize != nil {
			finErr := r.finalize(ctx, purchase.ID)
			if finErr == nil {
				result.Finalized++
				slog.Info("Reaper finalized stale wallet top-up as paid",
					"purchase_id", purchase.ID,
					"customer_id", purchase.CustomerID,
				)
				continue
			}
			if !errors.Is(finErr, ErrPurchaseFinalizationNotSupported) &&
				!errors.Is(finErr, ErrPurchaseFinalizationEvidenceMissing) {
				result.Failed++
				slog.Warn("Reaper could not finalize stale wallet top-up",
					"purchase_id", purchase.ID,
					"error", finErr,
				)
				continue
			}
			// Not settled: fall through and reopen it so the customer can retry.
		}

		reopened, reopenErr := r.store.ReturnStaleProcessingToPending(ctx, purchase.ID, cutoff)
		if reopenErr != nil {
			result.Failed++
			slog.Warn("Reaper could not return stale purchase to pending",
				"purchase_id", purchase.ID,
				"error", reopenErr,
			)
			continue
		}
		if !reopened {
			// Another worker re-claimed or finished it between the scan and the
			// update; the SQL predicate already re-checked claim age.
			continue
		}
		result.Reopened++
		slog.Info("Reaper returned stale purchase to pending",
			"purchase_id", purchase.ID,
			"customer_id", purchase.CustomerID,
			"invoice_type", purchase.InvoiceType,
		)
	}

	if result.Scanned > 0 {
		slog.Info("Stale processing sweep finished",
			"scanned", result.Scanned,
			"finalized", result.Finalized,
			"reopened", result.Reopened,
			"failed", result.Failed,
		)
	}

	return result, nil
}

// ReapStaleProcessingPurchases runs one reaper pass over abandoned in-flight
// purchases. It is safe to run concurrently: every transition is a conditional
// UPDATE that re-checks the row state.
func (s *PaymentService) ReapStaleProcessingPurchases(ctx context.Context) (int, error) {
	if s == nil || s.purchaseRepository == nil {
		return 0, nil
	}

	reaper := newStaleProcessingReaper(
		s.purchaseRepository,
		s.FinalizeProcessingWalletTopUpIfSettled,
		s.now,
	)
	result, err := reaper.reap(ctx)
	if err != nil {
		return 0, err
	}
	return result.Finalized + result.Reopened, nil
}
