package notification

import (
	"context"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type customerRepoMock struct {
	customers map[int64]*database.Customer
	err       error
}

func (m *customerRepoMock) FindById(ctx context.Context, id int64) (*database.Customer, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.customers[id], nil
}

type subKeyRepoMock struct {
	keys []database.SubscriptionKey
	err  error
}

func (m *subKeyRepoMock) FindExpiringKeys(ctx context.Context, startDate, endDate time.Time) ([]database.SubscriptionKey, error) {
	return m.keys, m.err
}

func TestSubscriptionService_ProcessSubscriptionExpiration_SendsNotification(t *testing.T) {
	expireAt := time.Now().Add(24 * time.Hour)
	keys := []database.SubscriptionKey{{ID: 1, CustomerID: 10, ExpireAt: &expireAt}}
	custs := map[int64]*database.Customer{
		10: {ID: 10, TelegramID: 100},
	}

	skRepo := &subKeyRepoMock{keys: keys}
	cRepo := &customerRepoMock{customers: custs}
	notifyCalls := 0

	svc := NewSubscriptionService(skRepo, cRepo, nil, nil)
	svc.notify = func(ctx context.Context, key database.SubscriptionKey, customer database.Customer) error {
		notifyCalls++
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}

	if notifyCalls != 1 {
		t.Fatalf("expected notification to be sent once, got %d", notifyCalls)
	}
}

func TestSubscriptionService_ProcessSubscriptionExpiration_NoCustomers(t *testing.T) {
	skRepo := &subKeyRepoMock{keys: []database.SubscriptionKey{}}
	cRepo := &customerRepoMock{customers: map[int64]*database.Customer{}}

	svc := NewSubscriptionService(skRepo, cRepo, nil, nil)
	svc.notify = func(ctx context.Context, key database.SubscriptionKey, customer database.Customer) error {
		t.Fatalf("sendNotification should not be called when there are no keys")
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}
}

func TestSubscriptionService_SendNotification_NilExpireAt(t *testing.T) {
	svc := NewSubscriptionService(&subKeyRepoMock{}, &customerRepoMock{}, nil, nil)

	err := svc.SendNotification(
		context.Background(),
		database.SubscriptionKey{ID: 1},
		database.Customer{ID: 1, TelegramID: 12345},
	)
	if err == nil {
		t.Fatal("expected error for key without expiration date")
	}
}
