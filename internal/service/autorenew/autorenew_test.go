package autorenew

import (
	"context"
	"path/filepath"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestFindPlanByDuration_Logic(t *testing.T) {
	plans := []config.Plan{
		{Label: "1 Month", Days: 30, Price: 5000, TrafficLimitGB: 0},
		{Label: "3 Months", Days: 90, Price: 12000, TrafficLimitGB: 0},
		{Label: "6 Months", Days: 180, Price: 20000, TrafficLimitGB: 0},
	}

	tests := []struct {
		days      int
		wantLabel string
		wantNil   bool
	}{
		{30, "1 Month", false},
		{90, "3 Months", false},
		{180, "6 Months", false},
		{365, "", true},
		{0, "", true},
	}

	findPlan := func(days int) *config.Plan {
		for _, plan := range plans {
			if plan.Days == days {
				return &plan
			}
		}
		return nil
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := findPlan(tt.days)
			if tt.wantNil {
				if got != nil {
					t.Errorf("findPlan(%d) = %+v; want nil", tt.days, got)
				}
				return
			}
			if got == nil {
				t.Errorf("findPlan(%d) = nil; want plan with label %q", tt.days, tt.wantLabel)
				return
			}
			if got.Label != tt.wantLabel {
				t.Errorf("findPlan(%d).Label = %q; want %q", tt.days, got.Label, tt.wantLabel)
			}
		})
	}
}

type fakeAutoRenewKeyRepo struct {
	keys           []database.SubscriptionKey
	findBefore     time.Time
	findCalls      int
	markedRenewed  []int64
	markedNotified []int64
}

func (f *fakeAutoRenewKeyRepo) FindExpiringAutoRenewKeys(_ context.Context, before time.Time) ([]database.SubscriptionKey, error) {
	f.findCalls++
	f.findBefore = before
	return append([]database.SubscriptionKey(nil), f.keys...), nil
}

func (f *fakeAutoRenewKeyRepo) MarkKeyAutoRenewed(_ context.Context, keyID int64) error {
	f.markedRenewed = append(f.markedRenewed, keyID)
	return nil
}

func (f *fakeAutoRenewKeyRepo) MarkKeyAutoRenewNotified(_ context.Context, keyID int64) error {
	f.markedNotified = append(f.markedNotified, keyID)
	return nil
}

type fakeAutoRenewCustomerRepo struct {
	customers map[int64]*database.Customer
	lookupIDs []int64
}

func (f *fakeAutoRenewCustomerRepo) FindById(_ context.Context, id int64) (*database.Customer, error) {
	f.lookupIDs = append(f.lookupIDs, id)
	return f.customers[id], nil
}

type walletExtendCall struct {
	keyID      int64
	customerID int64
	planPrice  float64
	days       int
	trafficGB  int
}

type fakeAutoRenewWallet struct {
	calls []walletExtendCall
}

func (f *fakeAutoRenewWallet) ExtendKeyWithBalance(_ context.Context, keyID int64, customerID int64, planPrice float64, days int, trafficGB int) error {
	f.calls = append(f.calls, walletExtendCall{
		keyID:      keyID,
		customerID: customerID,
		planPrice:  planPrice,
		days:       days,
		trafficGB:  trafficGB,
	})
	return nil
}

type fakeTelegramClient struct {
	messages []*bot.SendMessageParams
}

func (f *fakeTelegramClient) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	msg := *params
	f.messages = append(f.messages, &msg)
	return &models.Message{}, nil
}

func testTranslationManager(t *testing.T) *translation.Manager {
	t.Helper()

	tm := translation.GetInstance()
	if err := tm.InitTranslations(filepath.Join("..", "..", "..", "translations"), "en"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}
	return tm
}

func TestJobRunRenewsEligibleKeyAndNotifiesCustomer(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(48 * time.Hour)
	keyRepo := &fakeAutoRenewKeyRepo{
		keys: []database.SubscriptionKey{
			{ID: 7, CustomerID: 42, ExpireAt: &expireAt, TrafficLimitGB: 0},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 9001, Language: "en", Balance: 15000},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	telegram := &fakeTelegramClient{}
	plan := config.Plan{Label: "Monthly", Days: 30, Price: 5000, TrafficLimitGB: 0}

	job := &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  customerRepo,
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   telegram,
		nowFn: func() time.Time {
			return now
		},
		selectPlanFn: func(key database.SubscriptionKey, balance float64) (*config.Plan, bool) {
			if key.ID != 7 {
				t.Fatalf("selectPlanFn key.ID = %d, want 7", key.ID)
			}
			if balance != 15000 {
				t.Fatalf("selectPlanFn balance = %v, want 15000", balance)
			}
			return &plan, false
		},
	}

	job.Run(context.Background())

	if keyRepo.findCalls != 1 {
		t.Fatalf("FindExpiringAutoRenewKeys() calls = %d, want 1", keyRepo.findCalls)
	}
	if !keyRepo.findBefore.Equal(now.Add(3 * 24 * time.Hour)) {
		t.Fatalf("FindExpiringAutoRenewKeys() before = %v, want %v", keyRepo.findBefore, now.Add(3*24*time.Hour))
	}
	if len(customerRepo.lookupIDs) != 1 || customerRepo.lookupIDs[0] != 42 {
		t.Fatalf("FindById() lookups = %v, want [42]", customerRepo.lookupIDs)
	}
	if len(walletSvc.calls) != 1 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 1", len(walletSvc.calls))
	}
	call := walletSvc.calls[0]
	if call.keyID != 7 || call.customerID != 42 {
		t.Fatalf("ExtendKeyWithBalance() ids = (%d, %d), want (7, 42)", call.keyID, call.customerID)
	}
	if call.planPrice != 5000 || call.days != 30 || call.trafficGB != 0 {
		t.Fatalf("ExtendKeyWithBalance() args = (%.0f, %d, %d), want (5000, 30, 0)", call.planPrice, call.days, call.trafficGB)
	}
	if len(keyRepo.markedRenewed) != 1 || keyRepo.markedRenewed[0] != 7 {
		t.Fatalf("MarkKeyAutoRenewed() calls = %v, want [7]", keyRepo.markedRenewed)
	}
	if len(keyRepo.markedNotified) != 0 {
		t.Fatalf("MarkKeyAutoRenewNotified() calls = %v, want none", keyRepo.markedNotified)
	}
	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(telegram.messages))
	}
	if telegram.messages[0].ChatID != int64(9001) {
		t.Fatalf("SendMessage() chat_id = %v, want %d", telegram.messages[0].ChatID, 9001)
	}
	if !strings.Contains(telegram.messages[0].Text, "Auto-renewal complete") {
		t.Fatalf("SendMessage() text = %q, want success notification", telegram.messages[0].Text)
	}
	if !strings.Contains(telegram.messages[0].Text, "Monthly") {
		t.Fatalf("SendMessage() text = %q, want selected plan label", telegram.messages[0].Text)
	}
	if strings.Contains(telegram.messages[0].Text, "%!(EXTRA") {
		t.Fatalf("SendMessage() text contains fmt artifact: %q", telegram.messages[0].Text)
	}
}

func TestJobRunMarksInsufficientFundsAndNotifiesCustomer(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(24 * time.Hour)
	keyRepo := &fakeAutoRenewKeyRepo{
		keys: []database.SubscriptionKey{
			{ID: 9, CustomerID: 55, ExpireAt: &expireAt, TrafficLimitGB: 50},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			55: {ID: 55, TelegramID: 9010, Language: "en", Balance: 1250},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	telegram := &fakeTelegramClient{}

	job := &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  customerRepo,
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   telegram,
		nowFn: func() time.Time {
			return now
		},
		selectPlanFn: func(database.SubscriptionKey, float64) (*config.Plan, bool) {
			return nil, false
		},
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 0 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 0", len(walletSvc.calls))
	}
	if len(keyRepo.markedRenewed) != 0 {
		t.Fatalf("MarkKeyAutoRenewed() calls = %v, want none", keyRepo.markedRenewed)
	}
	if len(keyRepo.markedNotified) != 1 || keyRepo.markedNotified[0] != 9 {
		t.Fatalf("MarkKeyAutoRenewNotified() calls = %v, want [9]", keyRepo.markedNotified)
	}
	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(telegram.messages))
	}
	if telegram.messages[0].ChatID != int64(9010) {
		t.Fatalf("SendMessage() chat_id = %v, want %d", telegram.messages[0].ChatID, 9010)
	}
	if !strings.Contains(telegram.messages[0].Text, "Action Required") {
		t.Fatalf("SendMessage() text = %q, want insufficient-balance notification", telegram.messages[0].Text)
	}
	if strings.Contains(telegram.messages[0].Text, "%!(EXTRA") {
		t.Fatalf("SendMessage() text contains fmt artifact: %q", telegram.messages[0].Text)
	}
}
