package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgconn"
)

type fakeRefundTx struct {
	committed  bool
	rolledBack bool
}

func (t *fakeRefundTx) Commit(context.Context) error {
	t.committed = true
	return nil
}

func (t *fakeRefundTx) Rollback(context.Context) error {
	t.rolledBack = true
	return nil
}

type fakeWalletRefundStore struct {
	existingRefund bool
	existsErr      error
	logErr         error
	balanceErr     error
	commitErr      error
	balances       []float64
	refundRows     int
	refunds        map[int64]int
	tx             *fakeRefundTx
	beginCount     int
	logCalls       int
	purchaseIDs    []int64
	descriptions   []string
	balanceCalls   int
	storePurchases int
}

func (s *fakeWalletRefundStore) BeginTx(context.Context) (walletRefundTx, error) {
	s.beginCount++
	s.tx = &fakeRefundTx{}
	return s.tx, nil
}

func (s *fakeWalletRefundStore) ExistsRefund(_ context.Context, purchaseID int64) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	if s.refunds == nil {
		return s.existingRefund, nil
	}
	return s.refunds[purchaseID] > 0, nil
}

func (s *fakeWalletRefundStore) LogRefund(_ context.Context, _ walletRefundTx, _ int64, _ float64, purchaseID int64, description string) error {
	s.logCalls++
	s.purchaseIDs = append(s.purchaseIDs, purchaseID)
	s.descriptions = append(s.descriptions, description)
	if s.logErr != nil {
		return s.logErr
	}
	s.refundRows++
	return nil
}

func (s *fakeWalletRefundStore) AddBalance(_ context.Context, _ walletRefundTx, _ int64, amount float64) error {
	s.balanceCalls++
	if s.balanceErr != nil {
		return s.balanceErr
	}
	s.balances = append(s.balances, amount)
	return nil
}

func TestRefundWalletChargeSkipsWhenRefundAlreadyRecorded(t *testing.T) {
	store := &fakeWalletRefundStore{existingRefund: true}
	service := &PaymentService{}

	if err := service.refundWalletChargeWith(context.Background(), store, 7, 5000, 42, "Refund wallet purchase #42"); err != nil {
		t.Fatalf("refundWalletChargeWith() error = %v", err)
	}

	if store.beginCount != 0 {
		t.Fatalf("begin count = %d, want 0 (pre-check must short-circuit)", store.beginCount)
	}
	if store.balanceCalls != 0 {
		t.Fatalf("balance credits = %d, want 0", store.balanceCalls)
	}
}

func TestRefundWalletChargeCreditsOnceAndLogsRefund(t *testing.T) {
	store := &fakeWalletRefundStore{}
	service := &PaymentService{}

	if err := service.refundWalletChargeWith(context.Background(), store, 7, 5000, 42, "Refund wallet purchase #42"); err != nil {
		t.Fatalf("refundWalletChargeWith() error = %v", err)
	}

	if store.refundRows != 1 {
		t.Fatalf("refund rows = %d, want 1", store.refundRows)
	}
	if len(store.balances) != 1 || store.balances[0] != 5000 {
		t.Fatalf("balances = %v, want [5000]", store.balances)
	}
	if store.tx == nil || !store.tx.committed {
		t.Fatal("refund transaction was not committed")
	}
	if len(store.purchaseIDs) != 1 || store.purchaseIDs[0] != 42 {
		t.Fatalf("refund purchase ids = %v, want [42]", store.purchaseIDs)
	}
}

func TestRefundWalletChargeSecondAttemptRefusedByDatabase(t *testing.T) {
	// The pre-check races: both attempts believe no refund exists, so the
	// database's partial unique index rejects the second ledger insert.
	store := &fakeWalletRefundStore{}
	store.refunds = map[int64]int{}
	service := &PaymentService{}

	// First attempt succeeds and records the refund row.
	if err := service.refundWalletChargeWith(context.Background(), store, 7, 5000, 42, "Refund wallet purchase #42"); err != nil {
		t.Fatalf("first refund error = %v", err)
	}
	store.refunds[42] = 1

	// Second attempt bypasses the pre-check (map reset) to simulate the race and
	// must be refused by the ledger uniqueness itself.
	store.refunds = nil
	store.existingRefund = false
	store.logErr = &pgconn.PgError{Code: "23505", ConstraintName: "idx_wallet_transaction_refund_once"}

	if err := service.refundWalletChargeWith(context.Background(), store, 7, 5000, 42, "Refund wallet purchase #42"); err != nil {
		t.Fatalf("second refund error = %v, want nil (idempotent no-op)", err)
	}

	if store.balanceCalls != 1 {
		t.Fatalf("balance credits = %d, want 1 (the refused attempt must not credit)", store.balanceCalls)
	}
	if store.tx == nil {
		t.Fatal("expected the second attempt to open a transaction before the index refused it")
	}
	if store.tx.committed {
		t.Fatal("the refused transaction must not be committed")
	}
}

func TestRefundWalletChargePropagatesLedgerErrors(t *testing.T) {
	want := errors.New("ledger unavailable")
	store := &fakeWalletRefundStore{logErr: want}
	service := &PaymentService{}

	err := service.refundWalletChargeWith(context.Background(), store, 7, 5000, 42, "Refund wallet purchase #42")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if store.balanceCalls != 0 {
		t.Fatalf("balance credits = %d, want 0 when the ledger write failed", store.balanceCalls)
	}
}

func TestRefundWalletChargePropagatesBalanceErrors(t *testing.T) {
	want := errors.New("balance update failed")
	store := &fakeWalletRefundStore{balanceErr: want}
	service := &PaymentService{}

	err := service.refundWalletChargeWith(context.Background(), store, 7, 5000, 42, "Refund wallet purchase #42")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if store.tx == nil || store.tx.committed {
		t.Fatal("a failed balance credit must not commit the refund transaction")
	}
}

func TestRefundWalletChargeExistsCheckErrorIsFatal(t *testing.T) {
	want := errors.New("lookup failed")
	store := &fakeWalletRefundStore{existsErr: want}
	service := &PaymentService{}

	err := service.refundWalletChargeWith(context.Background(), store, 7, 5000, 42, "Refund wallet purchase #42")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if store.beginCount != 0 {
		t.Fatalf("begin count = %d, want 0", store.beginCount)
	}
}
