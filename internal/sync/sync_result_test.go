package sync

import (
	"context"
	"errors"
	"testing"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"

	"remnawave-tg-shop-bot/internal/database"
)

// TestSyncWithContextReportsCountsAndDeletions pins the U10 contract: the admin
// report needs what the panel returned and what actually changed locally.
func TestSyncWithContextReportsCountsAndDeletions(t *testing.T) {
	remoteUUID := uuid.New()
	users, customers := observedSnapshot(remoteUUID)
	keyStore := &recordingSubscriptionKeyStore{}

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: users},
		customerRepository: customers,
		subscriptionKeys:   keyStore,
	}

	result := service.SyncWithContext(context.Background())

	if !result.OK() {
		t.Fatalf("SyncWithContext() reported failures on a healthy pass: %v", result.Errors)
	}
	if result.RemoteUsers != 1 {
		t.Fatalf("RemoteUsers = %d, want 1", result.RemoteUsers)
	}
	if result.Updated != 1 {
		t.Fatalf("Updated = %d, want 1 for the one observed customer", result.Updated)
	}
	if result.Added != 0 {
		t.Fatalf("Added = %d, want 0 when every panel user already exists locally", result.Added)
	}
	if result.Deleted != 1 {
		t.Fatalf("Deleted = %d, want the reconciler's deleted-key count", result.Deleted)
	}
	if result.Failed != 0 {
		t.Fatalf("Failed = %d, want 0", result.Failed)
	}
	if result.Duration <= 0 {
		t.Fatalf("Duration = %v, want a measured pass duration", result.Duration)
	}
}

// TestSyncWithContextCountsNewCustomers covers the added path so an admin report
// can distinguish "everything already existed" from "new customers appeared".
func TestSyncWithContextCountsNewCustomers(t *testing.T) {
	users, _ := observedSnapshot(uuid.New())
	customers := &fakeCustomerStore{}

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: users},
		customerRepository: customers,
		subscriptionKeys:   &recordingSubscriptionKeyStore{},
	}

	result := service.SyncWithContext(context.Background())

	if result.Added != 1 {
		t.Fatalf("Added = %d, want 1 for a panel user the shop has never seen", result.Added)
	}
	if result.Updated != 0 {
		t.Fatalf("Updated = %d, want 0", result.Updated)
	}
}

// TestSyncWithContextReportsPanelReadFailure keeps a failed read from looking like
// a clean pass with zero changes.
func TestSyncWithContextReportsPanelReadFailure(t *testing.T) {
	_, customers := observedSnapshot(uuid.New())

	service := SyncService{
		client:             fakeRemnawaveUserLister{err: errors.New("remnawave unreachable")},
		customerRepository: customers,
		subscriptionKeys:   &recordingSubscriptionKeyStore{},
	}

	result := service.SyncWithContext(context.Background())

	if result.OK() {
		t.Fatal("SyncWithContext() reported success after a failed panel read")
	}
	if result.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %v, want one description", result.Errors)
	}
}

// TestSyncWithContextReportsReconciliationFailure covers the deleted-key step: a
// rejected deletion plan must be visible in the report, not only in the log.
func TestSyncWithContextReportsReconciliationFailure(t *testing.T) {
	users, customers := observedSnapshot(uuid.New())

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: users},
		customerRepository: customers,
		subscriptionKeys:   &recordingSubscriptionKeyStore{err: database.ErrSyncDeletionPlanTooLarge},
	}

	result := service.SyncWithContext(context.Background())

	if result.Failed != 1 {
		t.Fatalf("Failed = %d, want 1 when the deletion plan is rejected", result.Failed)
	}
	if result.Deleted != 0 {
		t.Fatalf("Deleted = %d, want 0 when nothing was reconciled", result.Deleted)
	}
	if result.Updated != 1 {
		t.Fatalf("Updated = %d, want the non-destructive upsert to still run", result.Updated)
	}
}

// TestSyncWithContextReportsEmptyPanel keeps an empty panel distinguishable from a
// failed read: no panel users is a fact, not an error.
func TestSyncWithContextReportsEmptyPanel(t *testing.T) {
	empty := []remapi.User{}

	service := SyncService{
		client:             fakeRemnawaveUserLister{users: &empty},
		customerRepository: &fakeCustomerStore{},
		subscriptionKeys:   &recordingSubscriptionKeyStore{},
	}

	result := service.SyncWithContext(context.Background())

	if !result.OK() {
		t.Fatalf("SyncWithContext() = %v, want a clean pass for an empty panel", result.Errors)
	}
	if result.RemoteUsers != 0 {
		t.Fatalf("RemoteUsers = %d, want 0", result.RemoteUsers)
	}
}
