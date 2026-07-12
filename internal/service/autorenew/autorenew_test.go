package autorenew

import (
	"context"
	"errors"
	"path/filepath"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
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
			name:    "missing configured renewal traffic is unknown",
			key:     database.SubscriptionKey{TrafficLimitGB: fiftyGB, AutoRenewPlanDays: intPtr(30)},
			wantErr: errAutoRenewPlanUnknown,
		},
		{
			name:    "configured plan no longer exists",
			key:     database.SubscriptionKey{TrafficLimitGB: fiftyGB, AutoRenewPlanDays: intPtr(9999), AutoRenewPlanTraffic: intPtr(fiftyGB)},
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
		TrafficLimitGB:       expected.TrafficLimitGB,
		AutoRenewPlanDays:    intPtr(expected.Days),
		AutoRenewPlanTraffic: intPtr(expected.TrafficLimitGB),
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

func TestFindConfiguredRenewalPlanPrefersStoredRenewalTraffic(t *testing.T) {
	plans := config.Plans()
	if len(plans) == 0 {
		t.Skip("config plans are not loaded in this test environment")
	}

	expected := plans[0]
	currentTraffic := expected.TrafficLimitGB * 2
	renewalTraffic := expected.TrafficLimitGB

	got, err := findConfiguredRenewalPlan(database.SubscriptionKey{
		TrafficLimitGB:       currentTraffic,
		AutoRenewPlanDays:    intPtr(expected.Days),
		AutoRenewPlanTraffic: intPtr(renewalTraffic),
	})
	if err != nil {
		t.Fatalf("findConfiguredRenewalPlan() error = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("findConfiguredRenewalPlan() = nil, want plan")
	}
	if got.Days != expected.Days || got.TrafficLimitGB != expected.TrafficLimitGB {
		t.Fatalf("findConfiguredRenewalPlan() = %+v, want traffic %d days %d", got, expected.TrafficLimitGB, expected.Days)
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

func (f *fakeAutoRenewKeyRepo) StampKeyLastAutoRenewed(_ context.Context, keyID int64, claimedAt time.Time) error {
	return nil
}

func (f *fakeAutoRenewKeyRepo) ClearKeyLastAutoRenewed(_ context.Context, keyID int64, claimedAt time.Time) error {
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
	keyID       int64
	customerID  int64
	planPrice   float64
	days        int
	trafficGB   int
	pricingTier string
}

type fakeAutoRenewWallet struct {
	calls []walletExtendCall
	err   error
}

func (f *fakeAutoRenewWallet) ExtendKeyWithBalance(ctx context.Context, keyID int64, customerID int64, planPrice float64, days int, trafficGB int) error {
	f.calls = append(f.calls, walletExtendCall{
		keyID:       keyID,
		customerID:  customerID,
		planPrice:   planPrice,
		days:        days,
		trafficGB:   trafficGB,
		pricingTier: payment.PricingTierFromContext(ctx),
	})
	return f.err
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
	if walletSvc.calls[0].planPrice != 5000 {
		t.Fatalf("ExtendKeyWithBalance() planPrice = %v, want 5000", walletSvc.calls[0].planPrice)
	}
	if walletSvc.calls[0].pricingTier != config.PricingTierRetail {
		t.Fatalf("ExtendKeyWithBalance() pricingTier = %q, want %q", walletSvc.calls[0].pricingTier, config.PricingTierRetail)
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
	if !strings.Contains(telegram.messages[0].Text, "5000") {
		t.Fatalf("SendMessage() text = %q, want retail amount 5000", telegram.messages[0].Text)
	}
}

func TestJobRunChargesResellerWholesalePrice(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(48 * time.Hour)
	wholesale := 4000
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 8, CustomerID: 43, ExpireAt: &expireAt, TrafficLimitGB: 0, AutoRenewPlanDays: intPtr(30)},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			// Balance covers wholesale but not retail — must charge wholesale.
			43: {ID: 43, TelegramID: 9002, Language: "en", Balance: 4500, IsReseller: true},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	telegram := &fakeTelegramClient{}
	plan := &config.Plan{
		Label:          "Monthly",
		Days:           30,
		Price:          5000,
		WholesalePrice: &wholesale,
		TrafficLimitGB: 0,
	}

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
			if key.ID != 8 {
				t.Fatalf("selectPlanFn key.ID = %d, want 8", key.ID)
			}
			return plan, nil
		},
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 1 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 1", len(walletSvc.calls))
	}
	if walletSvc.calls[0].planPrice != 4000 {
		t.Fatalf("ExtendKeyWithBalance() planPrice = %v, want wholesale 4000", walletSvc.calls[0].planPrice)
	}
	if walletSvc.calls[0].pricingTier != config.PricingTierWholesale {
		t.Fatalf("ExtendKeyWithBalance() pricingTier = %q, want %q", walletSvc.calls[0].pricingTier, config.PricingTierWholesale)
	}
	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(telegram.messages))
	}
	if !strings.Contains(telegram.messages[0].Text, "4000") {
		t.Fatalf("SendMessage() text = %q, want wholesale amount 4000", telegram.messages[0].Text)
	}
	if strings.Contains(telegram.messages[0].Text, "5000") {
		t.Fatalf("SendMessage() text = %q, must not show retail amount", telegram.messages[0].Text)
	}
}

func TestJobRunResellerWithoutWholesaleFallsBackRetail(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(48 * time.Hour)
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 10, CustomerID: 44, ExpireAt: &expireAt, TrafficLimitGB: 0, AutoRenewPlanDays: intPtr(30)},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			44: {ID: 44, TelegramID: 9003, Language: "en", Balance: 15000, IsReseller: true},
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
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return plan, nil
		},
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 1 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 1", len(walletSvc.calls))
	}
	if walletSvc.calls[0].planPrice != 5000 {
		t.Fatalf("ExtendKeyWithBalance() planPrice = %v, want retail 5000", walletSvc.calls[0].planPrice)
	}
	if walletSvc.calls[0].pricingTier != config.PricingTierRetail {
		t.Fatalf("ExtendKeyWithBalance() pricingTier = %q, want %q", walletSvc.calls[0].pricingTier, config.PricingTierRetail)
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

func timePtr(t time.Time) *time.Time {
	return &t
}

// statefulClaimRepo simulates DB claim lease + TTL reclaim + last_auto_renewed_at.
type statefulClaimRepo struct {
	key          database.SubscriptionKey
	now          time.Time
	claimTTL     time.Duration
	findAfter    time.Time
	findBefore   time.Time
	findCalls    int
	claimCalls   int
	releaseCalls int
	markCalls    int
	stampCalls   int
	markErr      error
	// stampErr fails StampKeyLastAutoRenewed only when last_auto_renewed_at is
	// already set (post-extend refresh path). Pre-extend stamp still succeeds.
	// Set stampFailAlways to fail every stamp (including pre-extend abort path).
	stampErr         error
	stampFailAlways  bool
	// When markFailsNoStamp is true, MarkKeyAutoRenewed returns markErr and does not finalize.
	markFailsNoStamp bool
}

func (f *statefulClaimRepo) FindExpiringAutoRenewKeys(_ context.Context, after time.Time, before time.Time) ([]database.SubscriptionKey, error) {
	f.findCalls++
	f.findAfter = after
	f.findBefore = before
	k := f.key
	return []database.SubscriptionKey{k}, nil
}

func (f *statefulClaimRepo) TryClaimAutoRenew(_ context.Context, keyID int64, expectedLast *time.Time) (*time.Time, bool, error) {
	f.claimCalls++
	if keyID != f.key.ID {
		return nil, false, nil
	}
	if !autoRenewLastMatches(f.key.LastAutoRenewedAt, expectedLast) {
		return nil, false, nil
	}
	if f.key.AutoRenewClaimedAt != nil {
		if !isAutoRenewClaimExpired(*f.key.AutoRenewClaimedAt, f.now, f.claimTTL) {
			return nil, false, nil // fresh claim — not stolen
		}
		// stale claim — reclaim
	}
	claimedAt := f.now
	f.key.AutoRenewClaimedAt = &claimedAt
	return &claimedAt, true, nil
}

func (f *statefulClaimRepo) ReleaseAutoRenewClaim(_ context.Context, keyID int64, claimedAt time.Time) error {
	f.releaseCalls++
	if f.key.AutoRenewClaimedAt != nil && f.key.AutoRenewClaimedAt.Equal(claimedAt) {
		f.key.AutoRenewClaimedAt = nil
	}
	return nil
}

func (f *statefulClaimRepo) MarkKeyAutoRenewed(_ context.Context, keyID int64, claimedAt time.Time) error {
	f.markCalls++
	if f.markFailsNoStamp {
		return f.markErr
	}
	if f.markErr != nil {
		return f.markErr
	}
	if f.key.AutoRenewClaimedAt == nil || !f.key.AutoRenewClaimedAt.Equal(claimedAt) {
		return errors.New("claim lost before finalization")
	}
	stamped := f.now
	f.key.LastAutoRenewedAt = &stamped
	f.key.AutoRenewClaimedAt = nil
	f.key.AutoRenewNotifiedAt = nil
	return nil
}

func (f *statefulClaimRepo) StampKeyLastAutoRenewed(_ context.Context, keyID int64, claimedAt time.Time) error {
	f.stampCalls++
	if f.key.AutoRenewClaimedAt == nil || !f.key.AutoRenewClaimedAt.Equal(claimedAt) {
		return errors.New("claim lost before cycle stamp")
	}
	if f.stampFailAlways && f.stampErr != nil {
		return f.stampErr
	}
	// Refresh-path failure: last already set (post-extend dual-fail simulation).
	if f.stampErr != nil && f.key.LastAutoRenewedAt != nil {
		return f.stampErr
	}
	stamped := f.now
	f.key.LastAutoRenewedAt = &stamped
	// Keep claim held (post-side-effect guard).
	return nil
}

func (f *statefulClaimRepo) ClearKeyLastAutoRenewed(_ context.Context, keyID int64, claimedAt time.Time) error {
	if f.key.AutoRenewClaimedAt == nil || !f.key.AutoRenewClaimedAt.Equal(claimedAt) {
		return errors.New("claim lost before cycle clear")
	}
	f.key.LastAutoRenewedAt = nil
	return nil
}

func (f *statefulClaimRepo) MarkKeyAutoRenewNotified(_ context.Context, keyID int64) error {
	return nil
}

func autoRenewLastMatches(have, expected *time.Time) bool {
	if have == nil && expected == nil {
		return true
	}
	if have == nil || expected == nil {
		return false
	}
	return have.Equal(*expected)
}

func TestIsAutoRenewClaimExpired(t *testing.T) {
	now := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	ttl := 30 * time.Minute

	fresh := now.Add(-5 * time.Minute)
	if isAutoRenewClaimExpired(fresh, now, ttl) {
		t.Fatal("fresh claim must not be expired")
	}
	stale := now.Add(-ttl - time.Minute)
	if !isAutoRenewClaimExpired(stale, now, ttl) {
		t.Fatal("stale claim must be expired")
	}
}

func TestTryClaimSemantics_FreshClaimNotStolen(t *testing.T) {
	now := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	claimedAt := now.Add(-5 * time.Minute)
	repo := &statefulClaimRepo{
		now:      now,
		claimTTL: autoRenewClaimTTL,
		key: database.SubscriptionKey{
			ID:                 1,
			CustomerID:         10,
			AutoRenewClaimedAt: &claimedAt,
		},
	}
	_, ok, err := repo.TryClaimAutoRenew(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("TryClaimAutoRenew() error = %v", err)
	}
	if ok {
		t.Fatal("fresh claim must not be stolen by second worker")
	}
}

func TestTryClaimSemantics_StalePreSideEffectClaimReclaimable(t *testing.T) {
	now := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	staleClaim := now.Add(-autoRenewClaimTTL - time.Minute)
	repo := &statefulClaimRepo{
		now:      now,
		claimTTL: autoRenewClaimTTL,
		key: database.SubscriptionKey{
			ID:                 2,
			CustomerID:         10,
			AutoRenewClaimedAt: &staleClaim,
			LastAutoRenewedAt:  nil,
		},
	}
	got, ok, err := repo.TryClaimAutoRenew(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("TryClaimAutoRenew() error = %v", err)
	}
	if !ok || got == nil {
		t.Fatal("stale pre-side-effect claim must be reclaimable")
	}
	if !got.Equal(now) {
		t.Fatalf("reclaimed claimedAt = %v, want %v", got, now)
	}
}

func TestJobRun_PostSideEffectMarkFailureDoesNotDuplicateAfterTTLReclaim(t *testing.T) {
	// D2#3 / M1: extend succeeds, MarkKeyAutoRenewed fails, claim held; after TTL reclaim
	// the cycle/last guard must prevent a second charge.
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	// Short plan still selectable after extend (expire stays within 3d window).
	expireAt := now.Add(2 * time.Hour)
	repo := &statefulClaimRepo{
		now:              now,
		claimTTL:         autoRenewClaimTTL,
		markFailsNoStamp: true,
		markErr:          errors.New("db mark failed"),
		key: database.SubscriptionKey{
			ID:                3,
			CustomerID:        42,
			ExpireAt:          &expireAt,
			TrafficLimitGB:    0,
			AutoRenewPlanDays: intPtr(1),
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 9001, Language: "en", Balance: 15000},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	plan := &config.Plan{Label: "Daily", Days: 1, Price: 500, TrafficLimitGB: 0}

	job := &Job{
		subKeyRepo:    repo,
		customerRepo:  customerRepo,
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return repo.now },
		selectPlanFn:  func(database.SubscriptionKey) (*config.Plan, error) { return plan, nil },
	}

	// Run 1: claim + extend + mark fails (must not release claim).
	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("first run Extend calls = %d, want 1", len(walletSvc.calls))
	}
	if repo.releaseCalls != 0 {
		t.Fatalf("post-side-effect mark failure must not release claim; releaseCalls=%d", repo.releaseCalls)
	}
	if repo.key.AutoRenewClaimedAt == nil {
		t.Fatal("claim must remain held after post-side-effect mark failure")
	}

	// Simulate successful extend updating local expiry (short plan still in window).
	newExpire := now.Add(26 * time.Hour)
	repo.key.ExpireAt = &newExpire

	// Run 2: still within TTL — fresh claim not stolen, no second charge.
	repo.now = now.Add(5 * time.Minute)
	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("within-TTL re-run Extend calls = %d, want 1", len(walletSvc.calls))
	}

	// After failed mark, production must have stamped last_auto_renewed_at.
	if repo.key.LastAutoRenewedAt == nil {
		t.Fatal("post-side-effect path must stamp last_auto_renewed_at for cycle guard")
	}

	// Run 3: past TTL — reclaim allowed, but cycle guard must not duplicate charge.
	repo.now = now.Add(autoRenewClaimTTL + time.Minute)
	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("after TTL reclaim Extend calls = %d, want 1 (no duplicate)", len(walletSvc.calls))
	}
}

func TestJobRun_PostSideEffectMarkAndStampBothFailDoesNotChargeAfterTTL(t *testing.T) {
	// Oracle blocker: extend succeeds, Mark fails, post-extend Stamp refresh fails.
	// Pre-extend durable cycle lock must still prevent a second charge after TTL.
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(2 * time.Hour)
	repo := &statefulClaimRepo{
		now:              now,
		claimTTL:         autoRenewClaimTTL,
		markFailsNoStamp: true,
		markErr:          errors.New("db mark failed"),
		stampErr:         errors.New("db stamp refresh failed"),
		key: database.SubscriptionKey{
			ID:                5,
			CustomerID:        42,
			ExpireAt:          &expireAt,
			TrafficLimitGB:    0,
			AutoRenewPlanDays: intPtr(1),
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	job := &Job{
		subKeyRepo: repo,
		customerRepo: &fakeAutoRenewCustomerRepo{customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 9001, Language: "en", Balance: 15000},
		}},
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return repo.now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Daily", Days: 1, Price: 500, TrafficLimitGB: 0}, nil
		},
	}

	// Run 1: pre-extend stamp + extend + mark fail + post-stamp refresh fail.
	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("first run Extend calls = %d, want 1", len(walletSvc.calls))
	}
	if repo.releaseCalls != 0 {
		t.Fatalf("post-side-effect path must not release claim; releaseCalls=%d", repo.releaseCalls)
	}
	if repo.key.LastAutoRenewedAt == nil {
		t.Fatal("pre-extend cycle lock must leave last_auto_renewed_at set even if mark+refresh stamp fail")
	}

	// Simulate short-plan still selectable after extend.
	newExpire := now.Add(26 * time.Hour)
	repo.key.ExpireAt = &newExpire

	// After TTL: cycle lock must block second charge (not claim-hold alone).
	repo.now = now.Add(autoRenewClaimTTL + time.Minute)
	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("after TTL with mark+stamp fail Extend calls = %d, want 1 (no duplicate)", len(walletSvc.calls))
	}
}

// TestJobRun_CrashAfterPreExtendStampIsMoneySafeStuck documents the intentional
// residual: process death after StampKeyLastAutoRenewed succeeds but before
// ExtendKeyWithBalance starts/completes leaves last_auto_renewed_at set with no
// charge. Later TTL/catch-up runs skip (alreadyRenewedThisCycle) instead of
// charging. Money-safe; requires manual recovery (clear last_auto_renewed_at).
// Not a bug to "fix" with automatic recharge — oracle-approved contract.
func TestJobRun_CrashAfterPreExtendStampIsMoneySafeStuck(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(2 * time.Hour)
	// Simulate durable state after crash: cycle lock written, claim may still be
	// held or TTL-expired; no successful extend occurred (wallet never charged).
	stampedAt := now
	staleClaim := now.Add(-autoRenewClaimTTL - time.Minute) // past TTL → reclaimable if lock absent
	repo := &statefulClaimRepo{
		now:      now.Add(autoRenewClaimTTL + time.Minute),
		claimTTL: autoRenewClaimTTL,
		key: database.SubscriptionKey{
			ID:                 8,
			CustomerID:         42,
			ExpireAt:           &expireAt,
			TrafficLimitGB:     0,
			AutoRenewPlanDays:  intPtr(1),
			LastAutoRenewedAt:  &stampedAt, // pre-extend lock survived crash
			AutoRenewClaimedAt: &staleClaim,
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	job := &Job{
		subKeyRepo: repo,
		customerRepo: &fakeAutoRenewCustomerRepo{customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 1, Language: "en", Balance: 15000},
		}},
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return repo.now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Daily", Days: 1, Price: 500, TrafficLimitGB: 0}, nil
		},
	}

	// Catch-up/TTL re-run: key still in selection window, but cycle lock must skip.
	if !alreadyRenewedThisCycle(repo.key.LastAutoRenewedAt, repo.key.ExpireAt) {
		t.Fatal("contract: pre-extend stamp without charge must still count as renewed this cycle")
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 0 {
		t.Fatalf("money-safe stuck residual: Extend calls = %d, want 0 (skip, not recharge)", len(walletSvc.calls))
	}
	if repo.claimCalls != 0 {
		t.Fatalf("cycle guard must skip before claim; claimCalls=%d", repo.claimCalls)
	}
	// Manual recovery path remains: clearing last_auto_renewed_at would allow retry
	// (not exercised here — ops/manual only; automatic recharge is forbidden).
}

func TestJobRun_PreExtendStampFailureDoesNotCharge(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(2 * time.Hour)
	repo := &statefulClaimRepo{
		now:             now,
		claimTTL:        autoRenewClaimTTL,
		stampFailAlways: true,
		stampErr:        errors.New("db stamp failed"),
		key: database.SubscriptionKey{
			ID:                6,
			CustomerID:        42,
			ExpireAt:          &expireAt,
			TrafficLimitGB:    0,
			AutoRenewPlanDays: intPtr(1),
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	job := &Job{
		subKeyRepo: repo,
		customerRepo: &fakeAutoRenewCustomerRepo{customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 1, Language: "en", Balance: 15000},
		}},
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return repo.now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Daily", Days: 1, Price: 500, TrafficLimitGB: 0}, nil
		},
	}

	job.Run(context.Background())
	if len(walletSvc.calls) != 0 {
		t.Fatalf("Extend calls = %d, want 0 when pre-extend stamp fails", len(walletSvc.calls))
	}
	// Pre-side-effect abort should release claim for retry.
	if repo.releaseCalls != 1 {
		t.Fatalf("releaseCalls = %d, want 1 after pre-extend stamp abort", repo.releaseCalls)
	}
}

func TestJobRun_ExtendFailureClearsCycleLockAndAllowsRetry(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(2 * time.Hour)
	repo := &statefulClaimRepo{
		now:      now,
		claimTTL: autoRenewClaimTTL,
		key: database.SubscriptionKey{
			ID:                7,
			CustomerID:        42,
			ExpireAt:          &expireAt,
			TrafficLimitGB:    0,
			AutoRenewPlanDays: intPtr(1),
		},
	}
	walletSvc := &fakeAutoRenewWallet{err: errors.New("extend failed")}
	job := &Job{
		subKeyRepo: repo,
		customerRepo: &fakeAutoRenewCustomerRepo{customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 1, Language: "en", Balance: 15000},
		}},
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return repo.now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Daily", Days: 1, Price: 500, TrafficLimitGB: 0}, nil
		},
	}

	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("Extend calls = %d, want 1", len(walletSvc.calls))
	}
	if repo.key.LastAutoRenewedAt != nil {
		t.Fatal("extend failure must clear pre-extend cycle lock for retry")
	}
	if repo.releaseCalls != 1 {
		t.Fatalf("releaseCalls = %d, want 1 after extend failure", repo.releaseCalls)
	}

	// Retry after claim release should be allowed (pre-side-effect path).
	walletSvc.err = nil
	job.Run(context.Background())
	if len(walletSvc.calls) != 2 {
		t.Fatalf("retry Extend calls = %d, want 2", len(walletSvc.calls))
	}
}

func TestJobRun_ShortPlanFailedMarkDoesNotChargeTwiceAfterReclaim(t *testing.T) {
	// M1 dedicated: short-plan remains selectable; failed mark must not double-charge.
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(1 * time.Hour)
	repo := &statefulClaimRepo{
		now:              now,
		claimTTL:         autoRenewClaimTTL,
		markFailsNoStamp: true,
		markErr:          errors.New("mark failed"),
		key: database.SubscriptionKey{
			ID:                4,
			CustomerID:        55,
			ExpireAt:          &expireAt,
			TrafficLimitGB:    0,
			AutoRenewPlanDays: intPtr(1),
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	job := &Job{
		subKeyRepo:   repo,
		customerRepo: &fakeAutoRenewCustomerRepo{customers: map[int64]*database.Customer{
			55: {ID: 55, TelegramID: 1, Language: "en", Balance: 99999},
		}},
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return repo.now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Daily", Days: 1, Price: 100, TrafficLimitGB: 0}, nil
		},
	}

	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("Extend calls = %d, want 1", len(walletSvc.calls))
	}

	// After failed mark, production must leave a durable cycle marker and hold claim.
	if repo.key.LastAutoRenewedAt == nil {
		t.Fatal("after failed mark, expected last_auto_renewed_at stamped (M1 cycle guard)")
	}
	if repo.key.AutoRenewClaimedAt == nil {
		t.Fatal("after failed mark, expected claim held (no release)")
	}

	newExpire := now.Add(25 * time.Hour)
	repo.key.ExpireAt = &newExpire
	repo.now = now.Add(autoRenewClaimTTL + 2*time.Minute)
	job.Run(context.Background())
	if len(walletSvc.calls) != 1 {
		t.Fatalf("short-plan after reclaim Extend calls = %d, want 1 (no duplicate charge)", len(walletSvc.calls))
	}
}

func TestJobRun_CatchUpLookbackSelectsKeyExpiredThreeHoursAgo(t *testing.T) {
	now := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	expireAt := now.Add(-3 * time.Hour)
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 20, CustomerID: 42, ExpireAt: &expireAt, TrafficLimitGB: 0, AutoRenewPlanDays: intPtr(30)},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	job := &Job{
		subKeyRepo:   keyRepo,
		customerRepo: &fakeAutoRenewCustomerRepo{customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 1, Language: "en", Balance: 99999},
		}},
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Monthly", Days: 30, Price: 5000, TrafficLimitGB: 0}, nil
		},
	}

	job.Run(context.Background())

	if keyRepo.findCalls != 1 {
		t.Fatalf("findCalls = %d, want 1", keyRepo.findCalls)
	}
	// Catch-up lookback must reach at least 3h into the past.
	if !keyRepo.findAfter.Equal(now.Add(-autoRenewLookbackWindow)) {
		t.Fatalf("findAfter = %v, want %v", keyRepo.findAfter, now.Add(-autoRenewLookbackWindow))
	}
	if autoRenewLookbackWindow < 3*time.Hour {
		t.Fatalf("autoRenewLookbackWindow = %v, want >= 3h for catch-up", autoRenewLookbackWindow)
	}
	if keyRepo.findAfter.After(expireAt) {
		t.Fatalf("lookback start %v is after expire_at %v — key would be missed", keyRepo.findAfter, expireAt)
	}
	if len(walletSvc.calls) != 1 {
		t.Fatalf("Extend calls = %d, want 1 for now-3h key", len(walletSvc.calls))
	}
}

func TestJobRun_CatchUpDoesNotRenewAlreadyRenewedCycle(t *testing.T) {
	now := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	// Expired 3h ago but already renewed this cycle (last stamp near previous expiry window).
	expireAt := now.Add(-3 * time.Hour)
	// After a prior successful renew that failed to push expire far enough / catch-up reselects:
	// last_auto_renewed_at within 4d before expire_at means already renewed this cycle.
	last := expireAt.Add(-1 * time.Hour) // after expireAt-4d
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{
				ID:                21,
				CustomerID:        42,
				ExpireAt:          &expireAt,
				LastAutoRenewedAt: &last,
				TrafficLimitGB:    0,
				AutoRenewPlanDays: intPtr(30),
			},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	job := &Job{
		subKeyRepo:   keyRepo,
		customerRepo: &fakeAutoRenewCustomerRepo{customers: map[int64]*database.Customer{
			42: {ID: 42, TelegramID: 1, Language: "en", Balance: 99999},
		}},
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Monthly", Days: 30, Price: 5000, TrafficLimitGB: 0}, nil
		},
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 0 {
		t.Fatalf("already-renewed cycle Extend calls = %d, want 0", len(walletSvc.calls))
	}
	if keyRepo.claimCalls != 0 {
		t.Fatalf("already-renewed cycle must skip before claim; claimCalls=%d", keyRepo.claimCalls)
	}
}

func TestAlreadyRenewedThisCycle(t *testing.T) {
	expireAt := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
	lastInCycle := expireAt.Add(-2 * 24 * time.Hour)
	lastOld := expireAt.Add(-10 * 24 * time.Hour)

	if !alreadyRenewedThisCycle(&lastInCycle, &expireAt) {
		t.Fatal("last within 4d of expire must count as renewed this cycle")
	}
	if alreadyRenewedThisCycle(&lastOld, &expireAt) {
		t.Fatal("old last must not count as renewed this cycle")
	}
	if alreadyRenewedThisCycle(nil, &expireAt) {
		t.Fatal("nil last must not count as renewed")
	}
}

func TestJobRunChargesNonResellerRetailPrice(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(48 * time.Hour)
	wholesale := 4000
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 71, CustomerID: 43, ExpireAt: &expireAt, TrafficLimitGB: 0, AutoRenewPlanDays: intPtr(30)},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			43: {ID: 43, TelegramID: 9002, Language: "en", Balance: 15000, IsReseller: false},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	plan := &config.Plan{
		Label:          "Monthly",
		Days:           30,
		Price:          5000,
		WholesalePrice: &wholesale,
		TrafficLimitGB: 0,
	}

	job := &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  customerRepo,
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   &fakeTelegramClient{},
		nowFn:         func() time.Time { return now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return plan, nil
		},
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 1 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 1", len(walletSvc.calls))
	}
	if walletSvc.calls[0].planPrice != 5000 {
		t.Fatalf("ExtendKeyWithBalance() planPrice = %v, want 5000 (retail)", walletSvc.calls[0].planPrice)
	}
	if walletSvc.calls[0].pricingTier != config.PricingTierRetail {
		t.Fatalf("ExtendKeyWithBalance() pricingTier = %q, want %q", walletSvc.calls[0].pricingTier, config.PricingTierRetail)
	}
}

func TestJobRunResellerInsufficientAgainstWholesaleDoesNotExtend(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(24 * time.Hour)
	wholesale := 4000
	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 72, CustomerID: 44, ExpireAt: &expireAt, TrafficLimitGB: 0, AutoRenewPlanDays: intPtr(30)},
		},
	}
	// Balance between wholesale and retail: must NOT charge retail, and must NOT extend.
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			44: {ID: 44, TelegramID: 9003, Language: "en", Balance: 3500, IsReseller: true},
		},
	}
	walletSvc := &fakeAutoRenewWallet{}
	telegram := &fakeTelegramClient{}
	plan := &config.Plan{
		Label:          "Monthly",
		Days:           30,
		Price:          5000,
		WholesalePrice: &wholesale,
		TrafficLimitGB: 0,
	}

	job := &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  customerRepo,
		walletService: walletSvc,
		tm:            testTranslationManager(t),
		telegramBot:   telegram,
		nowFn:         func() time.Time { return now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return plan, nil
		},
	}

	job.Run(context.Background())

	if len(walletSvc.calls) != 0 {
		t.Fatalf("ExtendKeyWithBalance() calls = %d, want 0 (insufficient for wholesale)", len(walletSvc.calls))
	}
	if len(keyRepo.markedRenewed) != 0 {
		t.Fatalf("MarkKeyAutoRenewed() calls = %v, want none", keyRepo.markedRenewed)
	}
	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1 insufficient-funds notice", len(telegram.messages))
	}
}
