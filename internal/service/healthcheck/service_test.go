package healthcheck

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/gemini"
)

type fakeAnalyzer struct {
	readiness gemini.AnalyzerReadiness
}

func (f fakeAnalyzer) AnalyzePaymentScreenshot(context.Context, []byte, string, []gemini.ConfiguredProvider) (*gemini.PaymentInfo, error) {
	return nil, nil
}

func (f fakeAnalyzer) Readiness(context.Context) gemini.AnalyzerReadiness {
	return f.readiness
}

type fakeCustomerStore struct {
	customer *database.Customer
	created  int
	updated  []map[string]interface{}
}

func (f *fakeCustomerStore) FindByTelegramId(context.Context, int64) (*database.Customer, error) {
	return f.customer, nil
}

func (f *fakeCustomerStore) Create(context.Context, *database.Customer) (*database.Customer, error) {
	f.created++
	if f.customer == nil {
		f.customer = &database.Customer{ID: 99, TelegramID: 900000000000001, Language: "en"}
	}
	return f.customer, nil
}

func (f *fakeCustomerStore) UpdateFields(_ context.Context, _ int64, updates map[string]interface{}) error {
	f.updated = append(f.updated, updates)
	return nil
}

type fakePaymentRunner struct {
	purchaseID int64
	err        error
	called     int
	onCreate   func()
}

func (f *fakePaymentRunner) CreatePurchase(_ context.Context, _ float64, _ int, _ int, _ *database.Customer, _ database.InvoiceType, _ string) (string, int64, error) {
	f.called++
	if f.onCreate != nil {
		f.onCreate()
	}
	return "", f.purchaseID, f.err
}

type fakeKeyStore struct {
	keys    []database.SubscriptionKey
	updates []struct {
		id     int64
		status string
	}
}

func (f *fakeKeyStore) FindByCustomerID(context.Context, int64) ([]database.SubscriptionKey, error) {
	return f.keys, nil
}

func (f *fakeKeyStore) UpdateStatus(_ context.Context, id int64, status string) error {
	f.updates = append(f.updates, struct {
		id     int64
		status string
	}{id: id, status: status})
	return nil
}

type fakeRemnawaveDeleter struct {
	deleted []uuid.UUID
	err     error
}

func (f *fakeRemnawaveDeleter) DeleteUser(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func TestRunSyntheticCheckSuccess(t *testing.T) {
	keyID := uuid.New()
	customers := &fakeCustomerStore{}
	keys := &fakeKeyStore{}
	payments := &fakePaymentRunner{
		purchaseID: 42,
		onCreate: func() {
			keys.keys = []database.SubscriptionKey{
				{ID: 7, CustomerID: 99, RemnawaveUUID: keyID, Status: "active"},
			}
		},
	}
	deleter := &fakeRemnawaveDeleter{}

	service := NewService(ServiceOptions{
		Analyzer: fakeAnalyzer{readiness: gemini.AnalyzerReadiness{
			Status: "ok",
			Providers: map[string]string{
				"openrouter": "ok",
			},
		}},
		Customers:           customers,
		Payments:            payments,
		SubscriptionKeys:    keys,
		RemnawaveUsers:      deleter,
		SyntheticTelegramID: 900000000000001,
		CanaryDays:          1,
		CanaryTrafficGB:     1,
	})

	report := service.Run(context.Background())
	if !report.Success {
		t.Fatalf("Run() success = false, want true: %+v", report)
	}
	if payments.called != 1 {
		t.Fatalf("CreatePurchase() called %d times, want 1", payments.called)
	}
	if customers.created != 1 {
		t.Fatalf("Create() called %d times, want 1", customers.created)
	}
	if len(deleter.deleted) != 1 || deleter.deleted[0] != keyID {
		t.Fatalf("DeleteUser() deleted = %#v, want [%v]", deleter.deleted, keyID)
	}
	if len(keys.updates) != 1 || keys.updates[0].status != "deleted" {
		t.Fatalf("UpdateStatus() = %#v, want one deleted update", keys.updates)
	}
	if len(report.Steps) < 3 {
		t.Fatalf("Run() steps = %d, want at least 3", len(report.Steps))
	}
}

func TestRunSyntheticCheckFailsWhenAnalyzerDegraded(t *testing.T) {
	payments := &fakePaymentRunner{}
	service := NewService(ServiceOptions{
		Analyzer: fakeAnalyzer{readiness: gemini.AnalyzerReadiness{
			Status: "degraded",
			Providers: map[string]string{
				"openrouter": "error: unauthorized",
			},
		}},
		Customers:           &fakeCustomerStore{},
		Payments:            payments,
		SubscriptionKeys:    &fakeKeyStore{},
		RemnawaveUsers:      &fakeRemnawaveDeleter{},
		SyntheticTelegramID: 900000000000001,
		CanaryDays:          1,
		CanaryTrafficGB:     1,
	})

	report := service.Run(context.Background())
	if report.Success {
		t.Fatal("Run() success = true, want false")
	}
	if payments.called != 0 {
		t.Fatalf("CreatePurchase() called %d times, want 0 when analyzer is degraded", payments.called)
	}
	if report.Steps[0].Status != StepFail {
		t.Fatalf("first step status = %q, want %q", report.Steps[0].Status, StepFail)
	}
}

func TestRunSyntheticCheckWarnsWhenCleanupFails(t *testing.T) {
	keyID := uuid.New()
	keys := &fakeKeyStore{}
	service := NewService(ServiceOptions{
		Analyzer: fakeAnalyzer{readiness: gemini.AnalyzerReadiness{Status: "ok"}},
		Customers: &fakeCustomerStore{
			customer: &database.Customer{ID: 99, TelegramID: 900000000000001, Language: "en"},
		},
		Payments: &fakePaymentRunner{
			purchaseID: 42,
			onCreate: func() {
				keys.keys = []database.SubscriptionKey{
					{ID: 7, CustomerID: 99, RemnawaveUUID: keyID, Status: "active"},
				}
			},
		},
		SubscriptionKeys:    keys,
		RemnawaveUsers:      &fakeRemnawaveDeleter{err: errors.New("delete failed")},
		SyntheticTelegramID: 900000000000001,
		CanaryDays:          1,
		CanaryTrafficGB:     1,
	})

	report := service.Run(context.Background())
	if report.Success {
		t.Fatal("Run() success = true, want false when cleanup fails")
	}
	foundWarn := false
	for _, step := range report.Steps {
		if step.Status == StepWarn && strings.Contains(strings.ToLower(step.Detail), "cleanup") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("Run() steps = %#v, want cleanup warning", report.Steps)
	}
}

func TestRunSyntheticCheckRejectsConcurrentExecution(t *testing.T) {
	service := NewService(ServiceOptions{
		Analyzer:            fakeAnalyzer{readiness: gemini.AnalyzerReadiness{Status: "ok"}},
		Customers:           &fakeCustomerStore{},
		Payments:            &fakePaymentRunner{},
		SubscriptionKeys:    &fakeKeyStore{},
		RemnawaveUsers:      &fakeRemnawaveDeleter{},
		SyntheticTelegramID: 900000000000001,
		CanaryDays:          1,
		CanaryTrafficGB:     1,
		Now:                 func() time.Time { return time.Unix(1000, 0) },
	})

	service.running.Store(true)
	report := service.Run(context.Background())
	if report.Success {
		t.Fatal("Run() success = true, want false while another run is active")
	}
	if len(report.Steps) != 1 || report.Steps[0].Status != StepFail {
		t.Fatalf("Run() steps = %#v, want one fail step", report.Steps)
	}
}
