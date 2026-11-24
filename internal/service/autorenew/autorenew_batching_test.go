package autorenew

import (
	"context"
	"sync"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

// blockingKeyRepo records claim traffic for the bounded-batch tests.
type blockingKeyRepo struct {
	keys []database.SubscriptionKey
	mu   sync.Mutex
	// claims counts keys that actually reached the claim step, i.e. keys the
	// run did not skip for lack of budget.
	claims int
}

func (f *blockingKeyRepo) FindExpiringAutoRenewKeys(context.Context, time.Time, time.Time) ([]database.SubscriptionKey, error) {
	return f.keys, nil
}

func (f *blockingKeyRepo) TryClaimAutoRenew(_ context.Context, keyID int64, _ *time.Time) (*time.Time, bool, error) {
	f.mu.Lock()
	f.claims++
	f.mu.Unlock()
	claimedAt := time.Date(2026, time.April, 1, 9, 5, 0, 0, time.UTC)
	return &claimedAt, true, nil
}

func (f *blockingKeyRepo) ReleaseAutoRenewClaim(context.Context, int64, time.Time) error { return nil }
func (f *blockingKeyRepo) MarkKeyAutoRenewed(context.Context, int64, time.Time) error     { return nil }
func (f *blockingKeyRepo) StampKeyLastAutoRenewed(context.Context, int64, time.Time) error {
	return nil
}
func (f *blockingKeyRepo) ClearKeyLastAutoRenewed(context.Context, int64, time.Time) error {
	return nil
}
func (f *blockingKeyRepo) MarkKeyAutoRenewNotified(context.Context, int64) error { return nil }

type fixedCustomerRepo struct {
	customers map[int64]*database.Customer
}

func (f *fixedCustomerRepo) FindById(_ context.Context, id int64) (*database.Customer, error) {
	return f.customers[id], nil
}

// concurrencyProbeWallet records the peak number of in-flight renewals and can
// block until the caller's context is done.
type concurrencyProbeWallet struct {
	mu      sync.Mutex
	inflight int
	peak    int
	block   bool
	calls   int
}

func (f *concurrencyProbeWallet) ExtendKeyWithBalance(ctx context.Context, _ int64, _ int64, _ float64, _ int, _ int) error {
	f.mu.Lock()
	f.inflight++
	f.calls++
	if f.inflight > f.peak {
		f.peak = f.inflight
	}
	block := f.block
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.inflight--
		f.mu.Unlock()
	}()

	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	// Hold briefly so parallel workers overlap.
	time.Sleep(2 * time.Millisecond)
	return nil
}

func batchTestJob(t *testing.T, keyRepo keyRepository, walletExtender walletExtender, keyCount int) *Job {
	t.Helper()

	customers := map[int64]*database.Customer{}
	for i := 1; i <= keyCount; i++ {
		customers[int64(i)] = &database.Customer{ID: int64(i), TelegramID: 1000 + int64(i), Language: "en", Balance: 100000}
	}

	return &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  &fixedCustomerRepo{customers: customers},
		walletService: walletExtender,
		tm:            testTranslationManager(t),
		nowFn:         func() time.Time { return time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC) },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Monthly", Days: 30, Price: 5000, TrafficLimitGB: 0}, nil
		},
	}
}

func batchTestKeys(count int) []database.SubscriptionKey {
	expireAt := time.Date(2026, time.April, 2, 9, 0, 0, 0, time.UTC)
	keys := make([]database.SubscriptionKey, 0, count)
	for i := 1; i <= count; i++ {
		days := 30
		keys = append(keys, database.SubscriptionKey{
			ID:                int64(i),
			CustomerID:        int64(i),
			ExpireAt:          &expireAt,
			AutoRenewPlanDays: &days,
		})
	}
	return keys
}

func TestProcessKeysRunsInBoundedBatches(t *testing.T) {
	const keyCount = 12
	keyRepo := &blockingKeyRepo{keys: batchTestKeys(keyCount)}
	wallet := &concurrencyProbeWallet{}
	job := batchTestJob(t, keyRepo, wallet, keyCount)

	result := job.processKeys(context.Background(), keyRepo.keys, autoRenewKeyTimeout)

	if result.Processed != keyCount {
		t.Fatalf("Processed = %d, want %d", result.Processed, keyCount)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("Skipped = %v, want none", result.Skipped)
	}
	if wallet.peak > autoRenewParallelism {
		t.Fatalf("peak concurrency = %d, want <= %d", wallet.peak, autoRenewParallelism)
	}
	if wallet.peak < 2 {
		t.Fatalf("peak concurrency = %d, want the run to overlap keys (>1); it is still sequential", wallet.peak)
	}
}

func TestProcessKeysUsesPerKeyTimeoutInsteadOfTheWholeRun(t *testing.T) {
	const keyCount = 4
	keyRepo := &blockingKeyRepo{keys: batchTestKeys(keyCount)}
	wallet := &concurrencyProbeWallet{block: true}
	job := batchTestJob(t, keyRepo, wallet, keyCount)

	// A long run budget: without a per-key timeout the blocked wallet would hold
	// every key until this deadline. The per-key timeout must cut each key far
	// sooner, so the whole run finishes well inside the budget.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	result := job.processKeys(ctx, keyRepo.keys, 100*time.Millisecond)
	elapsed := time.Since(start)

	if result.Processed != keyCount {
		t.Fatalf("Processed = %d, want %d", result.Processed, keyCount)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("processKeys took %v, want each key bounded near 100ms rather than the 10s run budget", elapsed)
	}
}

func TestProcessKeysReportsSkippedKeysWhenBudgetIsGone(t *testing.T) {
	const keyCount = 3
	keyRepo := &blockingKeyRepo{keys: batchTestKeys(keyCount)}
	wallet := &concurrencyProbeWallet{}
	job := batchTestJob(t, keyRepo, wallet, keyCount)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := job.processKeys(ctx, keyRepo.keys, autoRenewKeyTimeout)

	if result.Processed != 0 {
		t.Fatalf("Processed = %d, want 0 with an exhausted budget", result.Processed)
	}
	if len(result.Skipped) != keyCount {
		t.Fatalf("Skipped = %v, want all %d key ids", result.Skipped, keyCount)
	}
	if keyRepo.claims != 0 {
		t.Fatalf("claims = %d, want 0: skipped keys must never be claimed", keyRepo.claims)
	}
	for _, keyID := range result.Skipped {
		if keyID <= 0 {
			t.Fatalf("Skipped contains a bogus key id: %v", result.Skipped)
		}
	}
}

func TestKeyTimeoutIsCappedByTheKeyBudgetAndTheRunDeadline(t *testing.T) {
	job := &Job{}

	if got := job.keyTimeout(context.Background()); got != autoRenewKeyTimeout {
		t.Fatalf("keyTimeout() = %v, want %v without a run deadline", got, autoRenewKeyTimeout)
	}

	tight, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if got := job.keyTimeout(tight); got > autoRenewKeyTimeout {
		t.Fatalf("keyTimeout() = %v, want it capped by the run deadline", got)
	}
	if got := job.keyTimeout(tight); got <= 0 {
		t.Fatalf("keyTimeout() = %v, want a positive timeout", got)
	}

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	if got := job.keyTimeout(expired); got <= 0 {
		t.Fatalf("keyTimeout() = %v for an expired run deadline, want a positive value", got)
	}
}
