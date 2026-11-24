package sync

import (
	"context"
	"errors"
	"testing"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"

	"remnawave-tg-shop-bot/internal/database"
)

type fakeRemnawaveUserLister struct {
	users *[]remapi.User
	err   error
}

func (f fakeRemnawaveUserLister) GetUsers(context.Context) (*[]remapi.User, error) {
	return f.users, f.err
}

type recordingSubscriptionKeyStore struct {
	calls       int
	remoteUUIDs []uuid.UUID
	customerIDs []int64
	err         error
}

func (r *recordingSubscriptionKeyStore) MarkMissingRemoteKeysDeletedForCustomers(_ context.Context, customerIDs []int64, remoteUUIDs []uuid.UUID) (int64, error) {
	r.calls++
	r.customerIDs = append([]int64(nil), customerIDs...)
	r.remoteUUIDs = append([]uuid.UUID(nil), remoteUUIDs...)
	if r.err != nil {
		return 0, r.err
	}
	return 1, nil
}

type fakeCustomerStore struct {
	customers []database.Customer
	findErr   error
}

func (f *fakeCustomerStore) FindByTelegramIds(_ context.Context, telegramIDs []int64) ([]database.Customer, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	if telegramIDs == nil {
		return nil, nil
	}
	return append([]database.Customer(nil), f.customers...), nil
}

func (f *fakeCustomerStore) BeginTx(context.Context) (pgx.Tx, error) {
	return &fakeTx{}, nil
}

func (f *fakeCustomerStore) CreateBatchTx(context.Context, pgx.Tx, []database.Customer) error {
	return nil
}

func (f *fakeCustomerStore) UpdateBatchTx(context.Context, pgx.Tx, []database.Customer) error {
	return nil
}

// fakeTx satisfies pgx.Tx only far enough for Sync's batch-write path.
type fakeTx struct{}

func (t *fakeTx) Begin(context.Context) (pgx.Tx, error)               { return t, nil }
func (t *fakeTx) BeginFunc(context.Context, func(pgx.Tx) error) error { return nil }
func (t *fakeTx) Commit(context.Context) error                        { return nil }
func (t *fakeTx) Rollback(context.Context) error                      { return nil }
func (t *fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *fakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (t *fakeTx) LargeObjects() pgx.LargeObjects                       { return pgx.LargeObjects{} }
func (t *fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *fakeTx) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (t *fakeTx) Query(context.Context, string, ...interface{}) (pgx.Rows, error) { return nil, nil }
func (t *fakeTx) QueryRow(context.Context, string, ...interface{}) pgx.Row        { return nil }
func (t *fakeTx) QueryFunc(context.Context, string, []interface{}, []interface{}, func(pgx.QueryFuncRow) error) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (t *fakeTx) Conn() *pgx.Conn { return nil }

const observedTelegramID = int64(555000111)
const observedCustomerID = int64(4242)

// observedSnapshot builds the panel snapshot + customer store a healthy sync run
// sees: one panel user that maps onto one known local customer.
func observedSnapshot(remoteUUID uuid.UUID, extraUsers ...remapi.User) (*[]remapi.User, *fakeCustomerStore) {
	users := append([]remapi.User{{
		UUID:       remoteUUID,
		TelegramId: remapi.NewNilInt(int(observedTelegramID)),
		Username:   "wavy_observed_user",
	}}, extraUsers...)
	return &users, &fakeCustomerStore{customers: []database.Customer{{ID: observedCustomerID, TelegramID: observedTelegramID}}}
}

// TestSyncScopesKeyDeletionToObservedCustomers is the A3 regression test: the
// reconciliation must be driven by the customers the snapshot actually covered,
// never by a global "not in this list" update.
func TestSyncScopesKeyDeletionToObservedCustomers(t *testing.T) {
	remoteUUID := uuid.New()
	users, customers := observedSnapshot(remoteUUID)
	keyStore := &recordingSubscriptionKeyStore{}

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: users},
		customerRepository: customers,
		subscriptionKeys:   keyStore,
	}

	service.Sync()

	if keyStore.calls != 1 {
		t.Fatalf("MarkMissingRemoteKeysDeletedForCustomers calls = %d, want 1", keyStore.calls)
	}
	if len(keyStore.customerIDs) != 1 || keyStore.customerIDs[0] != observedCustomerID {
		t.Fatalf("reconciliation customer scope = %v, want [%d]", keyStore.customerIDs, observedCustomerID)
	}
	if len(keyStore.remoteUUIDs) != 1 || keyStore.remoteUUIDs[0] != remoteUUID {
		t.Fatalf("remote UUIDs = %v, want [%s]", keyStore.remoteUUIDs, remoteUUID)
	}
}

// TestSyncSkipsKeyDeletionWhenSnapshotFails refuses to reconcile at all when the
// fleet read is rejected, so a failed read cannot look like an empty fleet.
func TestSyncSkipsKeyDeletionWhenSnapshotFails(t *testing.T) {
	keyStore := &recordingSubscriptionKeyStore{}
	_, customers := observedSnapshot(uuid.New())

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: nil, err: errors.New("remnawave returned 137 of 400 users, refusing partial fleet read")},
		customerRepository: customers,
		subscriptionKeys:   keyStore,
	}

	service.Sync()

	if keyStore.calls != 0 {
		t.Fatalf("reconciliation calls = %d, want 0 when the fleet snapshot is refused", keyStore.calls)
	}
}

// TestSyncSkipsKeyDeletionWhenNoCustomerIsObserved covers the case where the
// panel lists users the shop has never recorded: nothing is in scope, so
// nothing may be deleted.
func TestSyncSkipsKeyDeletionWhenNoCustomerIsObserved(t *testing.T) {
	users, _ := observedSnapshot(uuid.New())
	keyStore := &recordingSubscriptionKeyStore{}

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: users},
		customerRepository: &fakeCustomerStore{},
		subscriptionKeys:   keyStore,
	}

	service.Sync()

	if keyStore.calls != 0 {
		t.Fatalf("reconciliation calls = %d, want 0 when no local customer is observed", keyStore.calls)
	}
}

func TestSyncSkipsDeletedKeyReconciliationWhenPanelHasNoUsers(t *testing.T) {
	users := []remapi.User{}
	keyStore := &recordingSubscriptionKeyStore{}
	_, customers := observedSnapshot(uuid.New())

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: &users},
		customerRepository: customers,
		subscriptionKeys:   keyStore,
	}

	service.Sync()

	if keyStore.calls != 0 {
		t.Fatalf("reconciliation calls = %d, want 0 for empty full-panel snapshot", keyStore.calls)
	}
}

func TestSyncSkipsDeletedKeyReconciliationWhenRemoteUsersHaveNoUUIDs(t *testing.T) {
	users, customers := observedSnapshot(uuid.New())
	users = &[]remapi.User{{TelegramId: remapi.NewNilInt(int(observedTelegramID))}}
	keyStore := &recordingSubscriptionKeyStore{}

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: users},
		customerRepository: customers,
		subscriptionKeys:   keyStore,
	}

	service.Sync()

	if keyStore.calls != 0 {
		t.Fatalf("reconciliation calls = %d, want 0 for malformed remote user list", keyStore.calls)
	}
}

// TestSyncReconciliationErrorDoesNotAbortCustomerSync keeps the destructive step
// isolated: a rejected deletion plan must not stop the non-destructive upsert.
func TestSyncReconciliationErrorDoesNotAbortCustomerSync(t *testing.T) {
	remoteUUID := uuid.New()
	users, customers := observedSnapshot(remoteUUID)
	keyStore := &recordingSubscriptionKeyStore{err: database.ErrSyncDeletionPlanTooLarge}

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: users},
		customerRepository: customers,
		subscriptionKeys:   keyStore,
	}

	service.Sync()

	if keyStore.calls != 1 {
		t.Fatalf("reconciliation calls = %d, want 1", keyStore.calls)
	}
}
