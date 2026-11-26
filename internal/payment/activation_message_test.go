package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
)

type activationTestSend struct {
	Text         string
	ParseMode    string
	Buttons      [][]map[string]interface{}
	HasReplyMark bool
}

func activationTestTranslations(t *testing.T) *translation.Manager {
	t.Helper()
	tm := translation.GetInstance()
	if err := tm.InitTranslations(filepath.Join("..", "..", "translations"), "en"); err != nil {
		t.Fatalf("InitTranslations() error = %v", err)
	}
	return tm
}

// newActivationTestBot returns a bot whose Telegram calls are captured, plus the
// captured sends.
func newActivationTestBot(t *testing.T, status int) (*bot.Bot, *[]activationTestSend) {
	t.Helper()
	var mu sync.Mutex
	sends := make([]activationTestSend, 0, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The bot client posts sendMessage as multipart/form-data.
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm() error = %v", err)
			return
		}

		mu.Lock()
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			record := activationTestSend{
				Text:      r.FormValue("text"),
				ParseMode: r.FormValue("parse_mode"),
			}
			if markup := r.FormValue("reply_markup"); markup != "" {
				record.HasReplyMark = true
				decoded := struct {
					InlineKeyboard [][]map[string]interface{} `json:"inline_keyboard"`
				}{}
				if err := json.Unmarshal([]byte(markup), &decoded); err != nil {
					t.Errorf("reply_markup is not JSON: %v", err)
				}
				record.Buttons = decoded.InlineKeyboard
			}
			sends = append(sends, record)
		}
		mu.Unlock()

		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1700000000,"chat":{"id":42,"type":"private"}}}`))
	}))
	t.Cleanup(server.Close)

	b, err := bot.New("123:TEST", bot.WithServerURL(server.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatalf("bot.New() error = %v", err)
	}
	return b, &sends
}

func TestSendKeyReadyMessageDeliversKeyAndSetupGuidance(t *testing.T) {
	restoreMini := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restoreMini()

	tm := activationTestTranslations(t)
	b, sends := newActivationTestBot(t, http.StatusOK)

	sub := "https://sub.example.com/key"
	service := &PaymentService{telegramBot: b, translation: tm}
	customer := &database.Customer{ID: 7, TelegramID: 42, Language: "en", SubscriptionLink: &sub}

	service.sendKeyReadyMessage(context.Background(), customer, 99)

	if len(*sends) != 1 {
		t.Fatalf("sent %d messages, want 1", len(*sends))
	}
	sent := (*sends)[0]
	if sent.ParseMode != "HTML" {
		t.Fatalf("parse mode = %q, want HTML", sent.ParseMode)
	}
	if !strings.Contains(sent.Text, "<code>https://sub.example.com/key</code>") {
		t.Fatalf("message does not carry the key in a code block: %q", sent.Text)
	}
	if strings.Contains(sent.Text, "Your Free Trial") {
		t.Fatalf("paid activation reused the trial copy: %q", sent.Text)
	}
	if strings.Contains(sent.Text, "%!") {
		t.Fatalf("message formatting failed: %q", sent.Text)
	}

	// The trial keyboard is the bar: import, downloads, connect, support, back.
	if len(sent.Buttons) != 5 {
		t.Fatalf("keyboard rows = %d, want 5", len(sent.Buttons))
	}
	if len(sent.Buttons[1]) != 2 {
		t.Fatalf("download row = %d buttons, want 2", len(sent.Buttons[1]))
	}
	if got, _ := sent.Buttons[3][0]["url"].(string); got != "https://t.me/ospeto" {
		t.Fatalf("support url = %q, want default support link", got)
	}
}

func TestSendKeyReadyMessageFailureIsNonFatal(t *testing.T) {
	tm := activationTestTranslations(t)
	restoreMini := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restoreMini()

	b, _ := newActivationTestBot(t, http.StatusInternalServerError)

	sub := "https://sub.example.com/key"
	service := &PaymentService{telegramBot: b, translation: tm}
	customer := &database.Customer{ID: 7, TelegramID: 42, Language: "my", SubscriptionLink: &sub}

	// A failed notification must not panic and must not surface as an error to
	// the purchase path: the key is already provisioned and the purchase is paid.
	service.sendKeyReadyMessage(context.Background(), customer, 99)
}

func TestSendKeyReadyMessageToleratesMissingDependencies(t *testing.T) {
	tm := activationTestTranslations(t)
	sub := "https://sub.example.com/key"

	cases := []struct {
		name     string
		service  *PaymentService
		customer *database.Customer
	}{
		{name: "no bot", service: &PaymentService{translation: tm}, customer: &database.Customer{TelegramID: 1}},
		{name: "no translation", service: &PaymentService{}, customer: &database.Customer{TelegramID: 1}},
		{name: "no customer", service: &PaymentService{translation: tm}, customer: nil},
		{name: "no subscription link", service: &PaymentService{translation: tm}, customer: &database.Customer{TelegramID: 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.service.sendKeyReadyMessage(context.Background(), tc.customer, 1)
		})
	}

	_ = sub
}

func TestSendKeyReadyMessageUsesCustomerLanguage(t *testing.T) {
	restoreMini := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restoreMini()

	tm := activationTestTranslations(t)
	b, sends := newActivationTestBot(t, http.StatusOK)

	sub := "https://sub.example.com/key"
	service := &PaymentService{telegramBot: b, translation: tm}
	service.sendKeyReadyMessage(context.Background(), &database.Customer{TelegramID: 42, Language: "my", SubscriptionLink: &sub}, 1)

	if len(*sends) != 1 {
		t.Fatalf("sent %d messages, want 1", len(*sends))
	}
	wantPrefix := tm.GetText("my", "key_ready_message")
	if strings.HasPrefix(wantPrefix, "<code>") {
		t.Fatalf("unexpected translation content")
	}
	// Burmese copy renders the Happ guide in Burmese, not the English fallback.
	if !strings.Contains((*sends)[0].Text, "Happ Proxy ဖြင့်") {
		t.Fatalf("Burmese customer received non-Burmese copy: %q", (*sends)[0].Text)
	}
}
