package sync

import (
	"context"
	"testing"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
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
}

func (r *recordingSubscriptionKeyStore) MarkMissingRemoteKeysDeleted(_ context.Context, remoteUUIDs []uuid.UUID) (int64, error) {
	r.calls++
	r.remoteUUIDs = append([]uuid.UUID(nil), remoteUUIDs...)
	return 1, nil
}

func TestSyncMarksLocalKeysMissingFromPanelDeleted(t *testing.T) {
	remoteUUID := uuid.New()
	users := []remapi.User{{UUID: remoteUUID}}
	keyStore := &recordingSubscriptionKeyStore{}

	service := SyncService{
		client:           fakeRemnawaveUserLister{users: &users},
		subscriptionKeys: keyStore,
	}

	service.Sync()

	if keyStore.calls != 1 {
		t.Fatalf("MarkMissingRemoteKeysDeleted calls = %d, want 1", keyStore.calls)
	}
	if len(keyStore.remoteUUIDs) != 1 || keyStore.remoteUUIDs[0] != remoteUUID {
		t.Fatalf("remote UUIDs = %v, want [%s]", keyStore.remoteUUIDs, remoteUUID)
	}
}

func TestSyncMarksAllLocalKeysDeletedWhenPanelHasNoUsers(t *testing.T) {
	users := []remapi.User{}
	keyStore := &recordingSubscriptionKeyStore{}

	service := SyncService{
		client:           fakeRemnawaveUserLister{users: &users},
		subscriptionKeys: keyStore,
	}

	service.Sync()

	if keyStore.calls != 1 {
		t.Fatalf("MarkMissingRemoteKeysDeleted calls = %d, want 1", keyStore.calls)
	}
	if len(keyStore.remoteUUIDs) != 0 {
		t.Fatalf("remote UUIDs = %v, want empty slice so all non-deleted local keys are hidden", keyStore.remoteUUIDs)
	}
}

func TestSyncSkipsDeletedKeyReconciliationWhenRemoteUsersHaveNoUUIDs(t *testing.T) {
	users := []remapi.User{{}}
	keyStore := &recordingSubscriptionKeyStore{}

	service := SyncService{
		client:           fakeRemnawaveUserLister{users: &users},
		subscriptionKeys: keyStore,
	}

	service.Sync()

	if keyStore.calls != 0 {
		t.Fatalf("MarkMissingRemoteKeysDeleted calls = %d, want 0 for malformed remote user list", keyStore.calls)
	}
}
