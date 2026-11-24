package autorenew

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/service/wallet"
)

type erroringWallet struct {
	err error
}

func (f *erroringWallet) ExtendKeyWithBalance(context.Context, int64, int64, float64, int, int) error {
	return f.err
}

// insufficientBalanceJob wires one key that passes the pre-check (balance looks
// sufficient) but whose charge fails with a balance shortfall, which is what
// happens when the balance moves between the check and the wallet call.
func insufficientBalanceJob(t *testing.T, walletErr error, telegram *fakeTelegramClient) (*Job, *fakeAutoRenewKeyRepo, *fakeAutoRenewCustomerRepo) {
	t.Helper()

	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	expireAt := now.Add(12 * time.Hour)
	planDays := 30

	keyRepo := &fakeAutoRenewKeyRepo{
		claimAllowed: true,
		keys: []database.SubscriptionKey{
			{ID: 91, CustomerID: 61, ExpireAt: &expireAt, AutoRenewPlanDays: &planDays},
		},
	}
	customerRepo := &fakeAutoRenewCustomerRepo{
		customers: map[int64]*database.Customer{
			61: {ID: 61, TelegramID: 9061, Language: "en", Balance: 5200},
		},
	}

	job := &Job{
		subKeyRepo:    keyRepo,
		customerRepo:  customerRepo,
		walletService: &erroringWallet{err: walletErr},
		tm:            testTranslationManager(t),
		telegramBot:   telegram,
		nowFn:         func() time.Time { return now },
		selectPlanFn: func(database.SubscriptionKey) (*config.Plan, error) {
			return &config.Plan{Label: "Monthly", Days: 30, Price: 5000, TrafficLimitGB: 0}, nil
		},
	}

	return job, keyRepo, customerRepo
}

func TestProcessKeyReportsShortfallAndTopUpButtonWhenBalanceFailsTheCharge(t *testing.T) {
	restore := config.SetMiniAppURLForTesting("https://shop.example.test")
	defer restore()

	telegram := &fakeTelegramClient{}
	// Balance is 5200 for a 5000 plan, so the pre-check passes; the wallet call
	// then fails because the balance dropped to 1000 in between.
	job, keyRepo, _ := insufficientBalanceJob(t, wallet.ErrInsufficientBalance, telegram)
	job.customerRepo = &fixedCustomerRepo{customers: map[int64]*database.Customer{
		61: {ID: 61, TelegramID: 9061, Language: "en", Balance: 1000},
	}}

	job.processKey(context.Background(), (*&keyRepo.keys[0]))

	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(telegram.messages))
	}

	msg := telegram.messages[0]
	if strings.Contains(msg.Text, "contact support") {
		t.Fatalf("message = %q, want the top-up copy, not the generic support copy", msg.Text)
	}
	if !strings.Contains(msg.Text, "4000") {
		t.Fatalf("message = %q, want the 4000 MMK shortfall (5000 plan - 1000 balance)", msg.Text)
	}

	markup, ok := msg.ReplyMarkup.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("ReplyMarkup = %T, want an inline keyboard with a top-up button", msg.ReplyMarkup)
	}
	if len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("keyboard = %+v, want one top-up button", markup.InlineKeyboard)
	}
	button := markup.InlineKeyboard[0][0]
	if button.WebApp == nil || button.WebApp.URL != "https://shop.example.test/wallet" {
		t.Fatalf("button = %+v, want a Mini App button to /wallet", button)
	}
	if msg.ParseMode != models.ParseModeHTML {
		t.Fatalf("ParseMode = %q, want HTML for the localized detail copy", msg.ParseMode)
	}
	if len(keyRepo.markedNotified) != 1 || keyRepo.markedNotified[0] != 91 {
		t.Fatalf("MarkKeyAutoRenewNotified() = %v, want [91]", keyRepo.markedNotified)
	}
}

func TestProcessKeyKeepsSupportCopyForNonMoneyFailures(t *testing.T) {
	restore := config.SetMiniAppURLForTesting("https://shop.example.test")
	defer restore()

	telegram := &fakeTelegramClient{}
	job, keyRepo, _ := insufficientBalanceJob(t, errors.New("remnawave returned 500"), telegram)

	job.processKey(context.Background(), (*&keyRepo.keys[0]))

	if len(telegram.messages) != 1 {
		t.Fatalf("SendMessage() calls = %d, want 1", len(telegram.messages))
	}
	msg := telegram.messages[0]
	if !strings.Contains(msg.Text, "contact support") {
		t.Fatalf("message = %q, want the generic support copy for a panel failure", msg.Text)
	}
	if msg.ReplyMarkup != nil {
		t.Fatalf("ReplyMarkup = %+v, want no top-up button for a non-money failure", msg.ReplyMarkup)
	}
}
