package notification

import (
	"context"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type customerRepoMock struct {
	customers *[]database.Customer
	err       error
}

func (m *customerRepoMock) FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error) {
	return m.customers, m.err
}

func TestSubscriptionService_ProcessSubscriptionExpiration_SendsNotification(t *testing.T) {
	expireAt := time.Now().Add(24 * time.Hour)
	customers := []database.Customer{{ID: 1, ExpireAt: &expireAt}}

	cRepo := &customerRepoMock{customers: &customers}
	notifyCalls := 0

	svc := NewSubscriptionService(cRepo, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
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
	customers := []database.Customer{}

	cRepo := &customerRepoMock{customers: &customers}

	svc := NewSubscriptionService(cRepo, nil, nil)
	svc.notify = func(ctx context.Context, customer database.Customer) error {
		t.Fatalf("sendNotification should not be called when there are no customers")
		return nil
	}

	if err := svc.ProcessSubscriptionExpiration(); err != nil {
		t.Fatalf("ProcessSubscriptionExpiration returned error: %v", err)
	}
}
