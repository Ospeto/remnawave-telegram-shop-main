package autorenew

import (
	"context"
	"errors"
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

func TestFindConfiguredRenewalPlan(t *testing.T) {
	fiftyGB := 50

	tests := []struct {
		name    string
		key     database.SubscriptionKey
		wantErr error
	}{
		{
			name:    "missing configured renewal days",
			key:     database.SubscriptionKey{TrafficLimitGB: 0},
			wantErr: errAutoRenewPlanUnknown,
		},
		{
			name:    "configured plan no longer exists",
			key:     database.SubscriptionKey{TrafficLimitGB: fiftyGB, AutoRenewPlanDays: intPtr(9999)},
			wantErr: errAutoRenewPlanUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findConfiguredRenewalPlan(tt.key)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("findConfiguredRenewalPlan() error = %v, want %v", err, tt.wantErr)
			}
			if got != nil {
				t.Fatalf("findConfiguredRenewalPlan() = %+v, want nil", got)
			}
		})
	}

	plans := config.Plans()
	if len(plans) == 0 {
		t.Skip("config plans are not loaded in this test environment")
	}

	expected := plans[0]
	got, err := findConfiguredRenewalPlan(database.SubscriptionKey{
		TrafficLimitGB:    expected.TrafficLimitGB,
		AutoRenewPlanDays: intPtr(expected.Days),
	})
	if err != nil {
		t.Fatalf("findConfiguredRenewalPlan() error = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("findConfiguredRenewalPlan() = nil, want plan")
	}
	if got.Label != expected.Label || got.Days != expected.Days || got.Price != expected.Price || got.TrafficLimitGB != expected.TrafficLimitGB {
		t.Fatalf("findConfiguredRenewalPlan() = %+v, want %+v", got, expected)
	}
}

type markedRenewal struct {
	keyID     int64
	claimedAt time.Time
}

type fakeAutoRenewKeyRepo struct {
	keys           []database.SubscriptionKey
	findAfter      time.Time
	findBefore     time.Time
	findCalls      int
	claimCalls     int
	releaseCalls   int
	lastClaimKeyID int64
	lastReleaseID  int64
	lastClaimPrev  *time.Time
	lastClaimedAt  time.Time
	lastReleasedAt time.Time
	claimAllowed   bool
	markedRenewed  []markedRenewal
	markedNotified []int64
}

func (f *fakeAutoRenewKeyRepo) FindExpiringAutoRenewKeys(_ context.Context, after time.Time, before time.Time) ([]database.SubscriptionKey, error) {
	f.findCalls++
	f.findAfter = after
	f.findBefore = before
	return append([]database.SubscriptionKey(nil), f.keys...), nil
}

func (f *fakeAutoRenewKeyRepo) TryClaimAutoRenew(_ context.Context, keyID int64, expectedLast *time.Time) (*time.Time, bool, error) {
	f.claimCalls++
	f.lastClaimKeyID = keyID
	f.lastClaimPrev = expectedLast
	if !f.claimAllowed {
		return nil, false, nil
	}
	claimedAt := time.Date(2026, time.April, 1, 9, 5, 0, 0, time.UTC)
	f.lastClaimedAt = claimedAt
	return &claimedAt, true, nil
}

func (f *fakeAutoRenewKeyRepo) ReleaseAutoRenewClaim(_ context.Context, keyID int64, claimedAt time.Time) error {
	f.releaseCalls++
	f.lastReleaseID = keyID
	f.lastReleasedAt = claimedAt
	return nil
}

func (f *fakeAutoRenewKeyRepo) MarkKeyAutoRenewed(_ context.Context, keyID int64, claimedAt time.Time) error {
	f.markedRenewed = append(f.markedRenewed, markedRenewal{keyID: keyID, claimedAt: claimedAt})
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
	sendErr  error
}

func (f *fakeTelegramClient) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	msg := *params
	f.messages = append(f.messages, &msg)
	if f.sendErr != nil {
		return nil, f.sendErr
	}
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

func TestJobRunRenewsEligibleKeyAndMarksSuccess(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(48 * time.Hour)
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 7, CustomerID: 42, ExpireAt: &expireAt, TrafficLimitGB: 0, AutoRenewPlanDays: intPtr(30)},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 9001, Language: "en", Balance: 15000},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	telegram := &fakeTelegramClient{}
	plan := &config.Plan{Label: "Monthly", Days: 30, Price: 5000, TrafficLimitGB: 0}

	job := &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  customerRepo,
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   telegram,
		nowFn: func() time.Time {
			return now
		},
		selectPlanFn: func(key database.SubscriptionKey) (*config.Plan, error) {
			if key.ID != 7 {
				t.Fatalf("selectPlanFn key.ID = %d, want 7", key.ID)
			}
			return plan, nil
		},
	}

	job.Run(context.Background())

	if keyRepo.findCalls != 1 {
		t.Fatalf("FindExpiringAutoRenewKeys() calls = %d, want 1", keyRepo.findCalls)
	}
	if !keyRepo.findAfter.Equal(now.Add(-autoRenewLookbackWindow)) {
		t.Fatalf("FindExpiringAutoRenewKeys() after = %v, want %v", keyRepo.findAfter, now.Add(-autoRenewLookbackWindow))
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
	if keyRepo.claimCalls != 1 || keyRepo.lastClaimKeyID != 7 {
		t.Fatalf("TryClaimAutoRenew() calls = %d for key %d, want 1 for key 7", keyRepo.claimCalls, keyRepo.lastClaimKeyID)
	}
	if keyRepo.releaseCalls != 0 {
		t.Fatalf("ReleaseAutoRenewClaim() calls = %d, want 0", keyRepo.releaseCalls)
	}
	if len(keyRepo.markedRenewed) != 1 || keyRepo.markedRenewed[0].keyID != 7 {
		t.Fatalf("MarkKeyAutoRenewed() calls = %+v, want key 7", keyRepo.markedRenewed)
	}
	if !keyRepo.markedRenewed[0].claimedAt.Equal(keyRepo.lastClaimedAt) {
		t.Fatalf("MarkKeyAutoRenewed() claimedAt = %v, want %v", keyRepo.markedRenewed[0].claimedAt, keyRepo.lastClaimedAt)
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
}

func TestJobRunMarksInsufficientFundsOnlyAfterSuccessfulNotification(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(24 * time.Hour)
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 9, CustomerID: 55, ExpireAt: &expireAt, TrafficLimitGB: 50, AutoRenewPlanDays: intPtr(30)},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			55: {ID: 55, TelegramID: 9010, Language: "en", Balance: 1250},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	telegram := &fakeTelegramClient{sendErr: errors.New("telegram down")}

	job := &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  customerRepo,
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   telegram,
		nowFn: func() time.Time {
			return now
		},
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "50GB Monthly", Days: 30, Price: 5000, TrafficLimitGB: 50}, nil
		},
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 0 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 0", len(walletSvc.calls))
	}
	if keyRepo.claimCalls != 1 || keyRepo.lastClaimKeyID != 9 {
		t.Fatalf("TryClaimAutoRenew() calls = %d for key %d, want 1 for key 9", keyRepo.claimCalls, keyRepo.lastClaimKeyID)
	}
	if keyRepo.releaseCalls != 1 || keyRepo.lastReleaseID != 9 {
		t.Fatalf("ReleaseAutoRenewClaim() calls = %d for key %d, want 1 for key 9", keyRepo.releaseCalls, keyRepo.lastReleaseID)
	}
	if len(keyRepo.markedRenewed) != 0 {
		t.Fatalf("MarkKeyAutoRenewed() calls = %v, want none", keyRepo.markedRenewed)
	}
	if len(keyRepo.markedNotified) != 0 {
		t.Fatalf("MarkKeyAutoRenewNotified() calls = %v, want none when Telegram send fails", keyRepo.markedNotified)
	}
	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(telegram.messages))
	}
}

func TestJobRunWarnsAndSkipsWhenExactRenewalPlanIsUnknown(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(6 * time.Hour)
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 11, CustomerID: 77, ExpireAt: &expireAt, TrafficLimitGB: 0},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			77: {ID: 77, TelegramID: 9020, Language: "en", Balance: 9000},
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
		selectPlanFn: findConfiguredRenewalPlan,
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 0 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 0", len(walletSvc.calls))
	}
	if keyRepo.releaseCalls != 1 {
		t.Fatalf("ReleaseAutoRenewClaim() calls = %d, want 1", keyRepo.releaseCalls)
	}
	if len(keyRepo.markedNotified) != 1 || keyRepo.markedNotified[0] != 11 {
		t.Fatalf("MarkKeyAutoRenewNotified() calls = %v, want [11]", keyRepo.markedNotified)
	}
	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(telegram.messages))
	}
	if !strings.Contains(telegram.messages[0].Text, "manual renewal") {
		t.Fatalf("SendMessage() text = %q, want manual-renewal guidance", telegram.messages[0].Text)
	}
}

func intPtr(v int) *int {
	return &v
}
