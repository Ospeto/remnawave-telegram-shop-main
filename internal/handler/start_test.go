package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
)

type capturedTelegramCall struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

func newTestBot(t *testing.T) (*bot.Bot, *[]capturedTelegramCall) {
	t.Helper()
	var captured []capturedTelegramCall
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body := make(map[string]interface{})
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "multipart/form-data") {
			if err := r.ParseMultipartForm(10 << 20); err == nil && r.MultipartForm != nil {
				for k, v := range r.MultipartForm.Value {
					if len(v) > 0 {
						body[k] = v[0]
					}
				}
			}
		} else {
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) > 0 {
				_ = json.Unmarshal(bodyBytes, &body)
			}
		}

		captured = append(captured, capturedTelegramCall{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   body,
		})

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":999,"date":1700000000,"chat":{"id":12345,"type":"private"}}}`))
	}))
	t.Cleanup(ts.Close)

	b, err := bot.New("123456:TEST_TOKEN", bot.WithServerURL(ts.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatalf("failed to create test bot: %v", err)
	}
	return b, &captured
}

func TestStartCommandHandler_EligibleCustomerAutoActivatesTrial(t *testing.T) {
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	subURL := "https://example.com/sub/auto-start-token"
	paymentSvc := &payment.PaymentService{}
	paymentSvc.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return true, nil
		},
		func(ctx context.Context, telegramID int64) (string, error) {
			return subURL, nil
		},
	)

	h := Handler{
		translation:    tm,
		paymentService: paymentSvc,
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 10,
			Chat: models.Chat{
				ID: 12345,
			},
			From: &models.User{
				ID:           12345,
				Username:     "testcustomer",
				LanguageCode: "en",
			},
			Text: "/start",
		},
	}

	h.StartCommandHandler(context.Background(), b, update)

	if len(*captured) != 1 {
		t.Fatalf("captured calls = %d, want exactly 1 (trial activation message)", len(*captured))
	}

	call := (*captured)[0]
	if !strings.HasSuffix(call.Path, "/sendMessage") {
		t.Fatalf("call path = %q, want suffix /sendMessage", call.Path)
	}

	text, _ := call.Body["text"].(string)
	if !strings.Contains(text, "<code>https://example.com/sub/auto-start-token</code>") {
		t.Fatalf("message text = %q, want code block with subURL", text)
	}

	// Verify it does not display the start menu greeting
	if strings.Contains(text, "greeting") || strings.Contains(text, tm.GetText("en", "greeting")) {
		t.Fatalf("message text should be trial activation, got start greeting: %q", text)
	}
}

func TestStartCommandHandler_EnsureCustomerUsesFromID(t *testing.T) {
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	var ensuredID int64
	paymentSvc := &payment.PaymentService{}
	paymentSvc.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return false, nil
		},
		nil,
	)

	h := Handler{
		translation:    tm,
		paymentService: paymentSvc,
		testEnsureCustomer: func(ctx context.Context, telegramID int64, langCode string) (*database.Customer, bool, error) {
			ensuredID = telegramID
			return &database.Customer{TelegramID: telegramID, Language: langCode}, false, nil
		},
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 11,
			Chat: models.Chat{
				ID: -1001234567890, // group chat id, distinct from From.ID
			},
			From: &models.User{
				ID:           55555,
				Username:     "groupuser",
				LanguageCode: "en",
			},
			Text: "/start",
		},
	}

	h.StartCommandHandler(context.Background(), b, update)

	if ensuredID != 55555 {
		t.Fatalf("ensureCustomer telegramID = %d, want From.ID 55555 (not Chat.ID)", ensuredID)
	}
	if len(*captured) == 0 {
		t.Fatal("expected start menu send after ineligible /start")
	}
}

func TestStartCommandHandler_IneligibleCustomerShowsStartMenu(t *testing.T) {
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	paymentSvc := &payment.PaymentService{}
	paymentSvc.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return false, nil // ineligible!
		},
		nil,
	)

	h := Handler{
		translation:    tm,
		paymentService: paymentSvc,
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 10,
			Chat: models.Chat{
				ID: 12345,
			},
			From: &models.User{
				ID:           12345,
				Username:     "testcustomer",
				LanguageCode: "en",
			},
			Text: "/start",
		},
	}

	h.StartCommandHandler(context.Background(), b, update)

	// In sendStartMenu, remove keyboard message is sent, deleted, then greeting sent
	if len(*captured) < 2 {
		t.Fatalf("captured calls = %d, want at least 2", len(*captured))
	}

	// Last call should be the start menu greeting (Burmese by default)
	lastCall := (*captured)[len(*captured)-1]
	text, _ := lastCall.Body["text"].(string)
	greeting := tm.GetText("my", "greeting")
	if !strings.Contains(text, greeting) {
		t.Fatalf("last call text = %q, want greeting %q", text, greeting)
	}
}

func TestCustomerTextMessageHandler_EligibleCustomerAutoActivatesOnGreetings(t *testing.T) {
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	greetings := []string{"hi", "hello", "မင်္ဂလာပါ", "hey", "Hi there"}

	for _, greeting := range greetings {
		t.Run(greeting, func(t *testing.T) {
			b, captured := newTestBot(t)
			tm := loadHandlerTestTranslations(t)

			subURL := "https://example.com/sub/auto-trial-" + greeting
			paymentSvc := &payment.PaymentService{}
			paymentSvc.SetTestTrialHooks(
				func(ctx context.Context, telegramID int64) (bool, error) {
					return true, nil
				},
				func(ctx context.Context, telegramID int64) (string, error) {
					return subURL, nil
				},
			)

			h := Handler{
				translation:    tm,
				paymentService: paymentSvc,
			}

			update := &models.Update{
				Message: &models.Message{
					ID: 20,
					Chat: models.Chat{
						ID: 54321,
					},
					From: &models.User{
						ID:           54321,
						Username:     "greeter",
						LanguageCode: "en",
					},
					Text: greeting,
				},
			}

			h.CustomerTextMessageHandler(context.Background(), b, update)

			if len(*captured) != 1 {
				t.Fatalf("for greeting %q, captured calls = %d, want 1", greeting, len(*captured))
			}

			call := (*captured)[0]
			text, _ := call.Body["text"].(string)
			if !strings.Contains(text, "<code>"+subURL+"</code>") {
				t.Fatalf("message text = %q, want code block with %q", text, subURL)
			}
		})
	}
}

func TestCustomerTextMessageHandler_IneligibleCustomerNoSubShowsStartMenu(t *testing.T) {
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	paymentSvc := &payment.PaymentService{}
	paymentSvc.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return false, nil // ineligible
		},
		nil,
	)

	h := Handler{
		translation:    tm,
		paymentService: paymentSvc,
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 30,
			Chat: models.Chat{
				ID: 67890,
			},
			From: &models.User{
				ID:           67890,
				Username:     "usedtrial",
				LanguageCode: "en",
			},
			Text: "hi",
		},
	}

	h.CustomerTextMessageHandler(context.Background(), b, update)

	// Since customer has no active sub, start menu should be sent
	if len(*captured) < 2 {
		t.Fatalf("captured calls = %d, want start menu", len(*captured))
	}

	lastCall := (*captured)[len(*captured)-1]
	text, _ := lastCall.Body["text"].(string)
	greeting := tm.GetText("my", "greeting")
	if !strings.Contains(text, greeting) {
		t.Fatalf("last call text = %q, want greeting %q", text, greeting)
	}
}

func TestCustomerTextMessageHandler_CustomerWithActiveSubShowsConnectionInfo(t *testing.T) {
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	paymentSvc := &payment.PaymentService{}
	paymentSvc.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return false, nil // ineligible for trial
		},
		nil,
	)

	activeLink := "https://example.com/sub/active-sub-key"
	futureExpire := time.Now().Add(48 * time.Hour)
	activeCustomer := &database.Customer{
		ID:               42,
		TelegramID:       77777,
		Language:         "en",
		SubscriptionLink: &activeLink,
		ExpireAt:         &futureExpire,
	}

	h := Handler{
		translation:    tm,
		paymentService: paymentSvc,
		testEnsureCustomer: func(ctx context.Context, telegramID int64, langCode string) (*database.Customer, bool, error) {
			return activeCustomer, false, nil
		},
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 40,
			Chat: models.Chat{
				ID: 77777,
			},
			From: &models.User{
				ID:           77777,
				Username:     "activesubber",
				LanguageCode: "en",
			},
			Text: "hi",
		},
	}

	h.CustomerTextMessageHandler(context.Background(), b, update)

	if len(*captured) != 1 {
		t.Fatalf("captured calls = %d, want 1 (connect info message)", len(*captured))
	}

	call := (*captured)[0]
	text, _ := call.Body["text"].(string)
	burmeseActive := tm.GetText("my", "subscription_active")
	subTextPrefix := strings.Split(burmeseActive, ":")[0]
	if !strings.Contains(text, subTextPrefix) || !strings.Contains(text, activeLink) {
		t.Fatalf("text = %q, want active subscription info with activeLink %q", text, activeLink)
	}
}

func TestCustomerTextMessageHandler_AdminIsIgnoredWhenIneligible(t *testing.T) {
	restoreAdmin := config.SetAdminTelegramIdForTesting(99999)
	defer restoreAdmin()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	// Payment service reports ineligible for trial
	ps := &payment.PaymentService{}
	ps.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return false, nil
		},
		nil,
	)

	h := Handler{
		translation:    tm,
		paymentService: ps,
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 50,
			Chat: models.Chat{
				ID: 99999,
			},
			From: &models.User{
				ID:           99999,
				Username:     "adminuser",
				LanguageCode: "en",
			},
			Text: "hi",
		},
	}

	h.CustomerTextMessageHandler(context.Background(), b, update)

	if len(*captured) != 0 {
		t.Fatalf("captured calls = %d, want 0 (admin message ignored when ineligible)", len(*captured))
	}
}

func TestCustomerTextMessageHandler_AdminAutoActivatesTrialOnGreeting(t *testing.T) {
	restoreAdmin := config.SetAdminTelegramIdForTesting(99999)
	defer restoreAdmin()
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	subURL := "https://example.com/sub/admin-trial-key"
	ps := &payment.PaymentService{}
	ps.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return true, nil
		},
		func(ctx context.Context, telegramID int64) (string, error) {
			return subURL, nil
		},
	)

	h := Handler{
		translation:    tm,
		paymentService: ps,
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 51,
			Chat: models.Chat{
				ID: 99999,
			},
			From: &models.User{
				ID:           99999,
				Username:     "adminuser",
				LanguageCode: "my",
			},
			Text: "hi",
		},
	}

	h.CustomerTextMessageHandler(context.Background(), b, update)

	if len(*captured) != 1 {
		t.Fatalf("captured calls = %d, want 1 trial delivery message", len(*captured))
	}

	text, _ := (*captured)[0].Body["text"].(string)
	if !strings.Contains(text, "<code>"+subURL+"</code>") {
		t.Fatalf("message text = %q, want code block with %q", text, subURL)
	}
}

func TestResolveEffectiveLanguage(t *testing.T) {
	tests := []struct {
		name         string
		customer     *database.Customer
		telegramLang string
		want         string
	}{
		{
			name:         "telegram en defaults to my",
			customer:     nil,
			telegramLang: "en",
			want:         "my",
		},
		{
			name:         "telegram empty defaults to my",
			customer:     nil,
			telegramLang: "",
			want:         "my",
		},
		{
			name:         "telegram ru preserved as ru",
			customer:     nil,
			telegramLang: "ru",
			want:         "ru",
		},
		{
			name:         "telegram ru-RU preserved as ru",
			customer:     nil,
			telegramLang: "ru-RU",
			want:         "ru",
		},
		{
			name:         "customer language en defaults to my",
			customer:     &database.Customer{Language: "en"},
			telegramLang: "en",
			want:         "my",
		},
		{
			name:         "customer language en with empty telegram defaults to my",
			customer:     &database.Customer{Language: "en"},
			telegramLang: "",
			want:         "my",
		},
		{
			name:         "customer language my returns my",
			customer:     &database.Customer{Language: "my"},
			telegramLang: "en",
			want:         "my",
		},
		{
			name:         "customer language ru returns ru",
			customer:     &database.Customer{Language: "ru"},
			telegramLang: "en",
			want:         "ru",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEffectiveLanguage(tt.customer, tt.telegramLang)
			if got != tt.want {
				t.Fatalf("resolveEffectiveLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStartCommandHandler_DefaultsTelegramEnToBurmese(t *testing.T) {
	restoreTrial := config.SetTrialConfigForTesting(7, 10)
	defer restoreTrial()

	b, captured := newTestBot(t)
	tm := loadHandlerTestTranslations(t)

	subURL := "https://example.com/sub/trial-key"
	ps := &payment.PaymentService{}
	ps.SetTestTrialHooks(
		func(ctx context.Context, telegramID int64) (bool, error) {
			return true, nil
		},
		func(ctx context.Context, telegramID int64) (string, error) {
			return subURL, nil
		},
	)

	h := Handler{
		translation:    tm,
		paymentService: ps,
	}

	update := &models.Update{
		Message: &models.Message{
			ID: 52,
			Chat: models.Chat{
				ID: 12345,
			},
			From: &models.User{
				ID:           12345,
				Username:     "myanmar_user",
				LanguageCode: "en", // Client sends "en"
			},
			Text: "/start",
		},
	}

	h.StartCommandHandler(context.Background(), b, update)

	if len(*captured) != 1 {
		t.Fatalf("captured calls = %d, want 1 trial delivery message", len(*captured))
	}

	text, _ := (*captured)[0].Body["text"].(string)
	// Must contain Burmese trial text rather than English
	if !strings.Contains(text, "အခမဲ့ စမ်းသပ်အသုံးပြုခွင့်") {
		t.Fatalf("message text = %q, want Burmese trial success text", text)
	}
}
