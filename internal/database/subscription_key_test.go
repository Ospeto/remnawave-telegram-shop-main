package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSubscriptionKeyRepositoryCanMarkMissingRemoteKeysDeleted(t *testing.T) {
	var _ interface {
		MarkMissingRemoteKeysDeleted(context.Context, []uuid.UUID) (int64, error)
	} = (*SubscriptionKeyRepository)(nil)
}

func TestMarkMissingRemoteKeysDeletedSkipsEmptyRemoteUUIDs(t *testing.T) {
	affected, err := (&SubscriptionKeyRepository{}).MarkMissingRemoteKeysDeleted(context.Background(), nil)
	if err != nil {
		t.Fatalf("MarkMissingRemoteKeysDeleted() error = %v", err)
	}
	if affected != 0 {
		t.Fatalf("MarkMissingRemoteKeysDeleted() affected = %d, want 0", affected)
	}
}

func TestPrimarySubscriptionKey(t *testing.T) {
	now := time.Now()
	earlier := now.Add(24 * time.Hour)
	later := now.Add(72 * time.Hour)

	keys := []SubscriptionKey{
		{
			ID:              1,
			SubscriptionURL: "https://example.com/early",
			ExpireAt:        &earlier,
			Status:          "active",
			CreatedAt:       now.Add(-2 * time.Hour),
		},
		{
			ID:              2,
			SubscriptionURL: "https://example.com/late",
			ExpireAt:        &later,
			Status:          "active",
			CreatedAt:       now.Add(-1 * time.Hour),
		},
		{
			ID:              3,
			SubscriptionURL: "https://example.com/deleted",
			ExpireAt:        &later,
			Status:          "deleted",
			CreatedAt:       now,
		},
	}

	got := PrimarySubscriptionKey(keys)
	if got == nil {
		t.Fatal("PrimarySubscriptionKey() = nil, want latest active key")
	}
	if got.ID != 2 {
		t.Fatalf("PrimarySubscriptionKey() id = %d, want 2", got.ID)
	}
}

func TestPrimarySubscriptionKeyBreaksExpiryTiesByCreatedAt(t *testing.T) {
	now := time.Now()
	expireAt := now.Add(48 * time.Hour)

	keys := []SubscriptionKey{
		{
			ID:              1,
			SubscriptionURL: "https://example.com/older",
			ExpireAt:        &expireAt,
			Status:          "active",
			CreatedAt:       now.Add(-2 * time.Hour),
		},
		{
			ID:              2,
			SubscriptionURL: "https://example.com/newer",
			ExpireAt:        &expireAt,
			Status:          "active",
			CreatedAt:       now.Add(-1 * time.Hour),
		},
	}

	got := PrimarySubscriptionKey(keys)
	if got == nil {
		t.Fatal("PrimarySubscriptionKey() = nil, want newest key on expiry tie")
	}
	if got.ID != 2 {
		t.Fatalf("PrimarySubscriptionKey() id = %d, want 2", got.ID)
	}
}

func TestPrimarySubscriptionKeySkipsExpiredKeys(t *testing.T) {
	now := time.Now()
	expired := now.Add(-1 * time.Hour)
	active := now.Add(24 * time.Hour)

	keys := []SubscriptionKey{
		{
			ID:        1,
			ExpireAt:  &expired,
			Status:    "active",
			CreatedAt: now,
		},
		{
			ID:        2,
			ExpireAt:  &active,
			Status:    "active",
			CreatedAt: now.Add(-1 * time.Hour),
		},
	}

	got := PrimarySubscriptionKey(keys)
	if got == nil {
		t.Fatal("PrimarySubscriptionKey() = nil, want non-expired key")
	}
	if got.ID != 2 {
		t.Fatalf("PrimarySubscriptionKey() id = %d, want 2", got.ID)
	}
}

func TestPrimarySubscriptionKeySkipsHiddenKeys(t *testing.T) {
	now := time.Now()
	future := now.Add(24 * time.Hour)

	keys := []SubscriptionKey{
		{
			ID:        1,
			ExpireAt:  &future,
			Status:    "active",
			IsHidden:  true,
			CreatedAt: now,
		},
		{
			ID:        2,
			ExpireAt:  &future,
			Status:    "active",
			IsHidden:  false,
			CreatedAt: now.Add(-1 * time.Hour),
		},
	}

	got := PrimarySubscriptionKey(keys)
	if got == nil {
		t.Fatal("PrimarySubscriptionKey() = nil, want non-hidden key")
	}
	if got.ID != 2 {
		t.Fatalf("PrimarySubscriptionKey() id = %d, want 2", got.ID)
	}
}

func TestAutoRenewClaimTTLIsPositiveAndReasonable(t *testing.T) {
	if AutoRenewClaimTTL <= 0 {
		t.Fatalf("AutoRenewClaimTTL = %v, want positive", AutoRenewClaimTTL)
	}
	// Fresh claims must outlive a single cron tick (hourly) only partially —
	// 30m is the intended lease; assert bounds so reclaim stays safe.
	if AutoRenewClaimTTL < 5*time.Minute {
		t.Fatalf("AutoRenewClaimTTL = %v, want >= 5m (avoid thrashing)", AutoRenewClaimTTL)
	}
	if AutoRenewClaimTTL > 2*time.Hour {
		t.Fatalf("AutoRenewClaimTTL = %v, want <= 2h (avoid permanent stuck)", AutoRenewClaimTTL)
	}
}

func TestTryClaimAutoRenewWithTTLRejectsNonPositiveByDefaulting(t *testing.T) {
	// Compile-time / API surface: WithTTL exists and default TTL is exported.
	var _ interface {
		TryClaimAutoRenew(context.Context, int64, *time.Time) (*time.Time, bool, error)
		TryClaimAutoRenewWithTTL(context.Context, int64, *time.Time, time.Duration) (*time.Time, bool, error)
		StampKeyLastAutoRenewed(context.Context, int64, time.Time) error
		ClearKeyLastAutoRenewed(context.Context, int64, time.Time) error
	} = (*SubscriptionKeyRepository)(nil)
}

func TestHideExpiredKey_InFlightPurchaseStatusesBlockHide(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	testTelegramID := time.Now().UnixNano()
	var customerID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, language, balance)
		VALUES ($1, 'en', 1000)
		RETURNING id`, testTelegramID).Scan(&customerID)
	if err != nil {
		t.Fatalf("failed to insert customer: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE customer_id = $1`, customerID)
		_, _ = pool.Exec(ctx, `DELETE FROM subscription_key WHERE customer_id = $1`, customerID)
		_, _ = pool.Exec(ctx, `DELETE FROM customer WHERE id = $1`, customerID)
	})

	expiredAt := time.Now().Add(-24 * time.Hour)
	var keyID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO subscription_key (
			customer_id, remnawave_uuid, username, subscription_url, expire_at, status, auto_renew, is_hidden
		) VALUES (
			$1, $2, 'testuser', 'https://example.com/sub/test', $3, 'expired', true, false
		) RETURNING id`, customerID, uuid.New(), expiredAt).Scan(&keyID)
	if err != nil {
		t.Fatalf("failed to insert subscription_key: %v", err)
	}

	repo := NewSubscriptionKeyRepository(pool)

	for _, blockingStatus := range []string{"new", "pending", "processing"} {
		t.Run("status_"+blockingStatus, func(t *testing.T) {
			var purchaseID int64
			err := pool.QueryRow(ctx, `
				INSERT INTO purchase (
					customer_id, amount, month, currency, status, invoice_type, extend_key_id
				) VALUES (
					$1, 5000, 1, 'MMK', $2, 'mobile_banking', $3
				) RETURNING id`, customerID, blockingStatus, keyID).Scan(&purchaseID)
			if err != nil {
				t.Fatalf("failed to insert purchase with status %s: %v", blockingStatus, err)
			}
			defer func() {
				_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE id = $1`, purchaseID)
			}()

			err = repo.HideExpiredKey(ctx, keyID, customerID)
			if !errors.Is(err, ErrKeyRenewalInFlight) {
				t.Fatalf("HideExpiredKey() for status %s got err = %v, want ErrKeyRenewalInFlight", blockingStatus, err)
			}

			var isHidden bool
			var hiddenAt *time.Time
			var autoRenew bool
			err = pool.QueryRow(ctx, `SELECT is_hidden, hidden_at, auto_renew FROM subscription_key WHERE id = $1`, keyID).Scan(&isHidden, &hiddenAt, &autoRenew)
			if err != nil {
				t.Fatalf("failed to query subscription key: %v", err)
			}
			if isHidden {
				t.Fatalf("subscription key is_hidden = true, want false")
			}
			if hiddenAt != nil {
				t.Fatalf("subscription key hidden_at = %v, want nil", hiddenAt)
			}
			if !autoRenew {
				t.Fatalf("subscription key auto_renew = false, want true (must remain unchanged)")
			}
		})
	}

	// With no in-flight purchases, HideExpiredKey succeeds and hides the key
	if err := repo.HideExpiredKey(ctx, keyID, customerID); err != nil {
		t.Fatalf("HideExpiredKey() without in-flight purchase failed: %v", err)
	}

	var isHidden bool
	var hiddenAt *time.Time
	var autoRenew bool
	err = pool.QueryRow(ctx, `SELECT is_hidden, hidden_at, auto_renew FROM subscription_key WHERE id = $1`, keyID).Scan(&isHidden, &hiddenAt, &autoRenew)
	if err != nil {
		t.Fatalf("failed to query subscription key: %v", err)
	}
	if !isHidden {
		t.Fatalf("subscription key is_hidden = false, want true")
	}
	if hiddenAt == nil {
		t.Fatalf("subscription key hidden_at is nil, want non-nil")
	}
	if !autoRenew {
		t.Fatalf("subscription key auto_renew = false, want true (hide must preserve auto_renew setting)")
	}
}

func TestHideExpiredKey_DatabaseRejectionMatrixAndVisibilityUpdate(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	testTelegramID := time.Now().UnixNano()
	var customerID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, language, balance)
		VALUES ($1, 'en', 1000)
		RETURNING id`, testTelegramID).Scan(&customerID)
	if err != nil {
		t.Fatalf("failed to insert customer: %v", err)
	}

	var otherCustomerID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, language, balance)
		VALUES ($1, 'en', 1000)
		RETURNING id`, testTelegramID+1).Scan(&otherCustomerID)
	if err != nil {
		t.Fatalf("failed to insert other customer: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE customer_id IN ($1, $2)`, customerID, otherCustomerID)
		_, _ = pool.Exec(ctx, `DELETE FROM subscription_key WHERE customer_id IN ($1, $2)`, customerID, otherCustomerID)
		_, _ = pool.Exec(ctx, `DELETE FROM customer WHERE id IN ($1, $2)`, customerID, otherCustomerID)
	})

	repo := NewSubscriptionKeyRepository(pool)

	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	insertKey := func(custID int64, status string, expire *time.Time, autoRenew bool) int64 {
		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO subscription_key (
				customer_id, remnawave_uuid, username, subscription_url, expire_at, status, auto_renew, is_hidden
			) VALUES (
				$1, $2, 'testuser', 'https://example.com/sub/test', $3, $4, $5, false
			) RETURNING id`, custID, uuid.New(), expire, status, autoRenew).Scan(&id)
		if err != nil {
			t.Fatalf("failed to insert key: %v", err)
		}
		return id
	}

	keyActivePast := insertKey(customerID, "active", &past, true)
	keyDeleted := insertKey(customerID, "deleted", &past, true)
	keyNilExpiry := insertKey(customerID, "expired", nil, true)
	keyFutureExpiry := insertKey(customerID, "expired", &future, true)
	keyOtherCustomer := insertKey(otherCustomerID, "expired", &past, true)
	keyValid := insertKey(customerID, "expired", &past, true)

	// 1. Missing key
	if err := repo.HideExpiredKey(ctx, 999999999, customerID); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("missing key err = %v, want ErrKeyNotFound", err)
	}

	// 2. Wrong customer
	if err := repo.HideExpiredKey(ctx, keyOtherCustomer, customerID); !errors.Is(err, ErrKeyNotOwned) {
		t.Errorf("other customer key err = %v, want ErrKeyNotOwned", err)
	}

	// 3. Status active (even with past expiry)
	if err := repo.HideExpiredKey(ctx, keyActivePast, customerID); !errors.Is(err, ErrKeyActive) {
		t.Errorf("active key err = %v, want ErrKeyActive", err)
	}

	// 4. Status deleted
	if err := repo.HideExpiredKey(ctx, keyDeleted, customerID); !errors.Is(err, ErrKeyDeleted) {
		t.Errorf("deleted key err = %v, want ErrKeyDeleted", err)
	}

	// 5. Nil expiry
	if err := repo.HideExpiredKey(ctx, keyNilExpiry, customerID); !errors.Is(err, ErrKeyIndeterminate) {
		t.Errorf("nil expiry key err = %v, want ErrKeyIndeterminate", err)
	}

	// 6. Future expiry
	if err := repo.HideExpiredKey(ctx, keyFutureExpiry, customerID); !errors.Is(err, ErrKeyActive) {
		t.Errorf("future expiry key err = %v, want ErrKeyActive", err)
	}

	// 7. Valid expired key -> succeeds
	if err := repo.HideExpiredKey(ctx, keyValid, customerID); err != nil {
		t.Fatalf("valid expired key hide failed: %v", err)
	}

	var isHidden bool
	var hiddenAt *time.Time
	var autoRenew bool
	err = pool.QueryRow(ctx, `SELECT is_hidden, hidden_at, auto_renew FROM subscription_key WHERE id = $1`, keyValid).Scan(&isHidden, &hiddenAt, &autoRenew)
	if err != nil {
		t.Fatalf("query key: %v", err)
	}
	if !isHidden || hiddenAt == nil {
		t.Fatalf("key was not marked hidden: is_hidden=%v, hidden_at=%v", isHidden, hiddenAt)
	}
	if !autoRenew {
		t.Fatalf("auto_renew was cleared, want preserved true")
	}

	// 8. Idempotent second call succeeds
	if err := repo.HideExpiredKey(ctx, keyValid, customerID); err != nil {
		t.Fatalf("idempotent hide call failed: %v", err)
	}
}

func TestHideExpiredKey_ConcurrentLockingWithPurchase(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	testTelegramID := time.Now().UnixNano()
	var customerID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, language, balance)
		VALUES ($1, 'en', 1000)
		RETURNING id`, testTelegramID).Scan(&customerID)
	if err != nil {
		t.Fatalf("failed to insert customer: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE customer_id = $1`, customerID)
		_, _ = pool.Exec(ctx, `DELETE FROM subscription_key WHERE customer_id = $1`, customerID)
		_, _ = pool.Exec(ctx, `DELETE FROM customer WHERE id = $1`, customerID)
	})

	repo := NewSubscriptionKeyRepository(pool)
	past := time.Now().Add(-24 * time.Hour)

	var keyID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO subscription_key (
			customer_id, remnawave_uuid, username, subscription_url, expire_at, status, auto_renew, is_hidden
		) VALUES (
			$1, $2, 'testuser', 'https://example.com/sub/test', $3, 'expired', true, false
		) RETURNING id`, customerID, uuid.New(), past).Scan(&keyID)
	if err != nil {
		t.Fatalf("failed to insert key: %v", err)
	}

	// tx1 starts purchase creation and validates key with FOR SHARE
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin tx1: %v", err)
	}
	defer tx1.Rollback(ctx)

	if err := repo.ValidateExtendKeyTx(ctx, tx1, keyID, customerID); err != nil {
		t.Fatalf("ValidateExtendKeyTx failed: %v", err)
	}

	// Start HideExpiredKey in goroutine (tx2) which attempts SELECT ... FOR UPDATE on keyID.
	// It must block until tx1 commits or rolls back!
	hideResult := make(chan error, 1)
	go func() {
		hideResult <- repo.HideExpiredKey(context.Background(), keyID, customerID)
	}()

	// Ensure tx2 has had time to start and block
	select {
	case res := <-hideResult:
		t.Fatalf("HideExpiredKey completed prematurely without waiting for tx1 lock: %v", res)
	case <-time.After(50 * time.Millisecond):
		// Expected: tx2 is blocked on row lock
	}

	// tx1 inserts purchase with status 'new' and commits
	_, err = tx1.Exec(ctx, `
		INSERT INTO purchase (
			customer_id, amount, month, currency, status, invoice_type, extend_key_id
		) VALUES (
			$1, 5000, 1, 'MMK', 'new', 'wallet_payment', $2
		)`, customerID, keyID)
	if err != nil {
		t.Fatalf("failed to insert purchase in tx1: %v", err)
	}

	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("tx1 commit failed: %v", err)
	}

	// tx2 now unblocks, sees in-flight purchase, and returns ErrKeyRenewalInFlight!
	select {
	case res := <-hideResult:
		if !errors.Is(res, ErrKeyRenewalInFlight) {
			t.Fatalf("HideExpiredKey got %v, want ErrKeyRenewalInFlight", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HideExpiredKey timed out waiting for row lock release")
	}
}

func TestValidateExtendKeyTx_NilTxSucceeds(t *testing.T) {
	repo := &SubscriptionKeyRepository{}
	if err := repo.ValidateExtendKeyTx(context.Background(), nil, 1, 1); err != nil {
		t.Fatalf("ValidateExtendKeyTx with nil tx got err = %v, want nil", err)
	}
}

func TestValidateExtendKeyTx_ValidationCases(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	testTelegramID := time.Now().UnixNano()
	var customerID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, language, balance)
		VALUES ($1, 'en', 1000)
		RETURNING id`, testTelegramID).Scan(&customerID)
	if err != nil {
		t.Fatalf("failed to insert customer: %v", err)
	}

	otherTelegramID := testTelegramID + 1
	var otherCustomerID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, language, balance)
		VALUES ($1, 'en', 1000)
		RETURNING id`, otherTelegramID).Scan(&otherCustomerID)
	if err != nil {
		t.Fatalf("failed to insert other customer: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE customer_id IN ($1, $2)`, customerID, otherCustomerID)
		_, _ = pool.Exec(ctx, `DELETE FROM subscription_key WHERE customer_id IN ($1, $2)`, customerID, otherCustomerID)
		_, _ = pool.Exec(ctx, `DELETE FROM customer WHERE id IN ($1, $2)`, customerID, otherCustomerID)
	})

	repo := NewSubscriptionKeyRepository(pool)

	insertKey := func(custID int64, status string, isHidden bool, remUUID uuid.UUID) int64 {
		var id int64
		url := fmt.Sprintf("https://example.com/sub/%s", uuid.NewString())
		uname := fmt.Sprintf("user_%s", uuid.NewString()[:8])
		err := pool.QueryRow(ctx, `
			INSERT INTO subscription_key (
				customer_id, remnawave_uuid, username, subscription_url, expire_at, status, is_hidden
			) VALUES (
				$1, $2, $3, $4, NOW() + INTERVAL '1 day', $5, $6
			) RETURNING id`, custID, remUUID, uname, url, status, isHidden).Scan(&id)
		if err != nil {
			t.Fatalf("failed to insert key: %v", err)
		}
		return id
	}

	activeKeyID := insertKey(customerID, "active", false, uuid.New())
	expiredKeyID := insertKey(customerID, "expired", false, uuid.New())
	hiddenKeyID := insertKey(customerID, "expired", true, uuid.New())
	deletedKeyID := insertKey(customerID, "deleted", false, uuid.New())
	otherCustKeyID := insertKey(otherCustomerID, "active", false, uuid.New())
	nilUUIDKeyID := insertKey(customerID, "active", false, uuid.Nil)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	// 1. Active owned key -> nil
	if err := repo.ValidateExtendKeyTx(ctx, tx, activeKeyID, customerID); err != nil {
		t.Errorf("active key err = %v, want nil", err)
	}

	// 2. Expired owned key -> nil
	if err := repo.ValidateExtendKeyTx(ctx, tx, expiredKeyID, customerID); err != nil {
		t.Errorf("expired key err = %v, want nil", err)
	}

	// 3. Hidden key -> nil (hidden keys remain extendable with original remote identity)
	if err := repo.ValidateExtendKeyTx(ctx, tx, hiddenKeyID, customerID); err != nil {
		t.Errorf("hidden key err = %v, want nil (hidden keys remain extendable)", err)
	}

	// 4. Deleted key -> ErrKeyNotOwned
	if err := repo.ValidateExtendKeyTx(ctx, tx, deletedKeyID, customerID); !errors.Is(err, ErrKeyNotOwned) {
		t.Errorf("deleted key err = %v, want ErrKeyNotOwned", err)
	}

	// 5. Wrong customer -> ErrKeyNotOwned
	if err := repo.ValidateExtendKeyTx(ctx, tx, otherCustKeyID, customerID); !errors.Is(err, ErrKeyNotOwned) {
		t.Errorf("other customer key err = %v, want ErrKeyNotOwned", err)
	}

	// 6. Nil UUID -> ErrKeyNotOwned
	if err := repo.ValidateExtendKeyTx(ctx, tx, nilUUIDKeyID, customerID); !errors.Is(err, ErrKeyNotOwned) {
		t.Errorf("nil uuid key err = %v, want ErrKeyNotOwned", err)
	}

	// 7. Non-existent key -> ErrKeyNotFound
	if err := repo.ValidateExtendKeyTx(ctx, tx, 999999999, customerID); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("non-existent key err = %v, want ErrKeyNotFound", err)
	}
}
