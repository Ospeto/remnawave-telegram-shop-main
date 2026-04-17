package notification

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
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
	keys     []database.SubscriptionKey
	err      error
	marked   []int64
	lastMark time.Time
}

func (m *subKeyRepoMock) FindExpiringKeys(ctx context.Context, startDate, endDate time.Time) ([]database.SubscriptionKey, error) {
	return m.keys, m.err
}

func (m *subKeyRepoMock) MarkExpirationNotified(ctx context.Context, keyID int64, notifiedAt time.Time) error {
	m.marked = append(m.marked, keyID)
	m.lastMark = notifiedAt
	return nil
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
	if len(skRepo.marked) != 1 || skRepo.marked[0] != 1 {
		t.Fatalf("expected expiration marker for key 1, got %#v", skRepo.marked)
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

func TestSubscriptionService_NotificationMessageText_FallsBackToLegacyTemplate(t *testing.T) {
	dir := t.TempDir()
	writeTranslationFile(t, dir, "en", map[string]string{
		"subscription_expiring":     "Expires on %s",
		"renew_subscription_button": "Renew",
	})

	tm := translation.GetInstance()
	if err := tm.InitTranslations(dir, "en"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}

	expireAt := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	svc := NewSubscriptionService(&subKeyRepoMock{}, &customerRepoMock{}, nil, tm)

	got := svc.notificationMessageText(
		database.SubscriptionKey{ID: 1, Label: "wavy_123", ExpireAt: &expireAt},
		database.Customer{Language: "en"},
	)

	if got != "Expires on 20.04.2026" {
		t.Fatalf("notificationMessageText() = %q, want legacy fallback text", got)
	}
}

func writeTranslationFile(t *testing.T, dir string, lang string, values map[string]string) {
	t.Helper()

	content, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	path := filepath.Join(dir, lang+".json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
