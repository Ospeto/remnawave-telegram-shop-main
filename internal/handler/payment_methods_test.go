package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

type decodedKeyboard struct {
	InlineKeyboard [][]struct {
		Text         string `json:"text"`
		CallbackData string `json:"callback_data"`
		URL          string `json:"url"`
		WebApp       *struct {
			URL string `json:"url"`
		} `json:"web_app"`
	} `json:"inline_keyboard"`
}

func decodeKeyboard(t *testing.T, raw string) decodedKeyboard {
	t.Helper()
	var markup decodedKeyboard
	if err := json.Unmarshal([]byte(raw), &markup); err != nil {
		t.Fatalf("reply_markup = %q, want an inline keyboard: %v", raw, err)
	}
	return markup
}

// newPhotoUpdate builds the photo message a customer sends after paying: one
// receipt photo, no text.
func newPhotoUpdate() *models.Update {
	return &models.Update{
		Message: &models.Message{
			ID:   60,
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, LanguageCode: "en"},
			Photo: []models.PhotoSize{
				{FileID: "small", FileUniqueID: "small", Width: 90, Height: 90},
				{FileID: "receipt", FileUniqueID: "receipt", Width: 1280, Height: 720},
			},
		},
	}
}

// TestCancelPendingSuccessKeyboardLabelsMatchTheirAction pins the Unit 2 defect:
// the two buttons carried each other's action, so "✖️ Cancel payment" took the
// customer to the start menu and no cancellation ever happened.
func TestCancelPendingSuccessKeyboardLabelsMatchTheirAction(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}
	h.testFindCustomer = func(ctx context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 10, TelegramID: telegramID, Language: "en"}, nil
	}
	h.testFindPurchase = func(ctx context.Context, purchaseID int64) (*database.Purchase, error) {
		return &database.Purchase{ID: purchaseID, CustomerID: 10, Amount: 15000}, nil
	}
	h.testCancelPurchase = func(ctx context.Context, purchaseID, customerID int64) (bool, error) {
		return true, nil
	}

	b, captured := newTestBot(t)
	h.CancelPendingPurchaseCallbackHandler(context.Background(), b, newCancelCallbackUpdate("cancel_pending_purchase?id=77&confirm=1"))

	if len(*captured) != 1 {
		t.Fatalf("telegram calls = %d, want 1", len(*captured))
	}
	raw, _ := (*captured)[0].Body["reply_markup"].(string)
	markup := decodeKeyboard(t, raw)

	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("rows = %d, want cancel + back", len(markup.InlineKeyboard))
	}

	// The label, not the position, decides what a customer expects. Whichever row
	// carries the cancel label must cancel; whichever carries back must buy.
	seenCancel, seenBack := false, false
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			switch {
			case button.Text == h.translation.GetText("en", "cancel_pending_button"),
				button.Text == h.translation.GetText("my", "cancel_pending_button"):
				seenCancel = true
				if !strings.HasPrefix(button.CallbackData, CallbackCancelPendingPurchase) {
					t.Fatalf("cancel button callback = %q, want %s?...", button.CallbackData, CallbackCancelPendingPurchase)
				}
				if !strings.Contains(button.CallbackData, "id=77") {
					t.Fatalf("cancel callback = %q, want the purchase id", button.CallbackData)
				}
			case button.Text == h.translation.GetText("en", "back_button"),
				button.Text == h.translation.GetText("my", "back_button"):
				seenBack = true
				if button.CallbackData != CallbackBuy {
					t.Fatalf("back button callback = %q, want %q", button.CallbackData, CallbackBuy)
				}
			}
		}
	}

	if !seenCancel {
		t.Fatal("no button carried the cancel label")
	}
	if !seenBack {
		t.Fatal("no button carried the back label")
	}
}

// TestSellScreenExplainsASwitchedOffCheckout covers the second Unit 2 defect: with
// the in-chat chain off, the method screen used to hold only a Back button and no
// sentence explaining why the list was empty.
func TestSellScreenExplainsASwitchedOffCheckout(t *testing.T) {
	restoreMini := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restoreMini()
	restoreBanking := config.SetMobileBankingEnabledForTesting(false)
	defer restoreBanking()

	h := Handler{translation: loadHandlerTestTranslations(t)}
	b, captured := newTestBot(t)
	h.SellCallbackHandler(context.Background(), b, newCancelCallbackUpdate("sell?plan=2"))

	if len(*captured) != 1 {
		t.Fatalf("telegram calls = %d, want 1", len(*captured))
	}
	call := (*captured)[0]
	if !strings.HasSuffix(call.Path, "/editMessageText") {
		t.Fatalf("call path = %q, want editMessageText so the screen can explain itself", call.Path)
	}

	text, _ := call.Body["text"].(string)
	if text == "" || text == "payment_methods_unavailable_miniapp" {
		t.Fatalf("text = %q, want resolved explanation copy", text)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("switched-off screen rendered empty text")
	}

	raw, _ := call.Body["reply_markup"].(string)
	markup := decodeKeyboard(t, raw)

	// The empty-method screen must offer the surface that can take the order.
	var webAppURL string
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.WebApp != nil {
				webAppURL = button.WebApp.URL
			}
		}
	}
	if webAppURL != "https://shop.example.com/plans" {
		t.Fatalf("web app button URL = %q, want the Mini App plans screen", webAppURL)
	}
}

// TestSellScreenStillShowsTheMethodWhenEnabled guards the enabled path.
func TestSellScreenStillShowsTheMethodWhenEnabled(t *testing.T) {
	restoreBanking := config.SetMobileBankingEnabledForTesting(true)
	defer restoreBanking()

	h := Handler{translation: loadHandlerTestTranslations(t)}
	keyboard := h.buildPaymentMethodKeyboard("en", "2")

	if len(keyboard) != 2 {
		t.Fatalf("rows = %d, want method + back", len(keyboard))
	}
	if keyboard[0][0].CallbackData != "payment?plan=2&invoiceType=mobile_banking" {
		t.Fatalf("method callback = %q", keyboard[0][0].CallbackData)
	}
	if keyboard[1][0].CallbackData != CallbackBuy {
		t.Fatalf("back callback = %q", keyboard[1][0].CallbackData)
	}
}

// TestSwitchedOffCheckoutOffersSupportWithoutMiniApp keeps the last door open: no
// Mini App configured and no in-chat chain means support is the only way forward.
func TestSwitchedOffCheckoutOffersSupportWithoutMiniApp(t *testing.T) {
	restoreMini := config.SetMiniAppURLForTesting("")
	defer restoreMini()
	restoreBanking := config.SetMobileBankingEnabledForTesting(false)
	defer restoreBanking()

	h := Handler{translation: loadHandlerTestTranslations(t)}
	keyboard := h.buildPaymentMethodKeyboard("en", "2")

	if len(keyboard) != 2 {
		t.Fatalf("rows = %d, want support + back", len(keyboard))
	}
	if keyboard[0][0].URL != "https://t.me/ospeto" {
		t.Fatalf("support url = %q, want the default support link", keyboard[0][0].URL)
	}
}

// TestReceiptPhotoWhileCheckoutOffGetsAnAnswer is the mirror defect: the photo
// handler was registered only under the flag, so in the switched-off state a
// receipt photo matched no handler and got no reply at all.
func TestReceiptPhotoWhileCheckoutOffGetsAnAnswer(t *testing.T) {
	restoreMini := config.SetMiniAppURLForTesting("https://shop.example.com")
	defer restoreMini()
	restoreBanking := config.SetMobileBankingEnabledForTesting(false)
	defer restoreBanking()

	h := Handler{translation: loadHandlerTestTranslations(t)}
	b, captured := newTestBot(t)
	h.MobilePayScreenshotHandler(context.Background(), b, newPhotoUpdate())

	if len(*captured) != 1 {
		t.Fatalf("telegram calls = %d, want one honest reply instead of silence", len(*captured))
	}
	call := (*captured)[0]
	if !strings.HasSuffix(call.Path, "/sendMessage") {
		t.Fatalf("call path = %q, want sendMessage", call.Path)
	}
	text, _ := call.Body["text"].(string)
	if text == "" || text == "payment_methods_unavailable_miniapp" {
		t.Fatalf("reply text = %q, want resolved copy", text)
	}

	raw, _ := call.Body["reply_markup"].(string)
	markup := decodeKeyboard(t, raw)
	if len(markup.InlineKeyboard) != 1 || markup.InlineKeyboard[0][0].WebApp == nil {
		t.Fatalf("reply keyboard = %#v, want a Mini App button", markup.InlineKeyboard)
	}
}
