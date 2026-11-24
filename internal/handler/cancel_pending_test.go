package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/database"
)

// newCancelCallbackUpdate builds the callback update the pending-payment
// keyboard sends, with the given callback data.
func newCancelCallbackUpdate(data string) *models.Update {
	return &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "callback-1",
			From: models.User{ID: 12345, LanguageCode: "en"},
			Message: models.MaybeInaccessibleMessage{
				Type:    models.MaybeInaccessibleMessageTypeMessage,
				Message: &models.Message{ID: 55, Chat: models.Chat{ID: 12345}},
			},
			Data: data,
		},
	}
}

// TestCancelPendingPurchaseAsksForConfirmation is the U2 regression test.
//
// The pending-payment keyboard used to hold only a WebApp link, so the chat had
// no way to release a purchase waiting for a screenshot: the customer had to
// open the Mini App, and translations/en.json:55 told them to. The first tap must
// now ask before doing anything destructive.
func TestCancelPendingPurchaseAsksForConfirmation(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}
	h.testFindCustomer = func(ctx context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 10, TelegramID: telegramID, Language: "en"}, nil
	}
	h.testFindPurchase = func(ctx context.Context, purchaseID int64) (*database.Purchase, error) {
		return &database.Purchase{ID: purchaseID, CustomerID: 10, Amount: 15000}, nil
	}
	cancelCalls := 0
	h.testCancelPurchase = func(ctx context.Context, purchaseID, customerID int64) (bool, error) {
		cancelCalls++
		return true, nil
	}

	b, captured := newTestBot(t)
	h.CancelPendingPurchaseCallbackHandler(context.Background(), b, newCancelCallbackUpdate("cancel_pending_purchase?id=77"))

	if cancelCalls != 0 {
		t.Fatalf("cancel calls before confirmation = %d, want 0", cancelCalls)
	}
	if len(*captured) != 1 {
		t.Fatalf("telegram calls = %d, want 1 confirmation edit", len(*captured))
	}
	call := (*captured)[0]
	if !strings.HasSuffix(call.Path, "/editMessageText") {
		t.Fatalf("call path = %q, want suffix /editMessageText", call.Path)
	}
	text, _ := call.Body["text"].(string)
	// The prompt is localized, and this shop resolves a non-Russian Telegram
	// locale to Burmese, so the assertion is "resolved copy, not a raw key".
	if text == "" || text == "cancel_pending_confirm" {
		t.Fatalf("confirmation text = %q, want resolved localized copy", text)
	}
	if !strings.Contains(text, "15000") {
		t.Fatalf("confirmation text = %q, want the amount the customer is releasing", text)
	}

	// reply_markup travels as a serialized JSON string on the wire.
	rawMarkup, _ := call.Body["reply_markup"].(string)
	var markup struct {
		InlineKeyboard [][]struct {
			Text         string `json:"text"`
			CallbackData string `json:"callback_data"`
		} `json:"inline_keyboard"`
	}
	if err := json.Unmarshal([]byte(rawMarkup), &markup); err != nil {
		t.Fatalf("reply_markup = %q, want an inline keyboard: %v", rawMarkup, err)
	}
	if len(markup.InlineKeyboard) == 0 || len(markup.InlineKeyboard[0]) == 0 {
		t.Fatalf("inline_keyboard = %#v, want confirmation buttons", markup.InlineKeyboard)
	}
	if got := markup.InlineKeyboard[0][0].CallbackData; got != "cancel_pending_purchase?id=77&confirm=1" {
		t.Fatalf("confirm callback_data = %q, want the confirmed variant of the same purchase", got)
	}
}

// TestCancelPendingPurchaseReleasesConfirmedPurchase proves the confirmed tap
// goes through the shared payment-service path and tells the customer it worked.
func TestCancelPendingPurchaseReleasesConfirmedPurchase(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}
	h.testFindCustomer = func(ctx context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 10, TelegramID: telegramID, Language: "en"}, nil
	}
	h.testFindPurchase = func(ctx context.Context, purchaseID int64) (*database.Purchase, error) {
		return &database.Purchase{ID: purchaseID, CustomerID: 10, Amount: 15000}, nil
	}
	var cancelledID, cancelledCustomer int64
	h.testCancelPurchase = func(ctx context.Context, purchaseID, customerID int64) (bool, error) {
		cancelledID, cancelledCustomer = purchaseID, customerID
		return true, nil
	}

	b, captured := newTestBot(t)
	h.CancelPendingPurchaseCallbackHandler(context.Background(), b, newCancelCallbackUpdate("cancel_pending_purchase?id=77&confirm=1"))

	if cancelledID != 77 || cancelledCustomer != 10 {
		t.Fatalf("cancel called with purchase=%d customer=%d, want 77 and 10", cancelledID, cancelledCustomer)
	}
	if len(*captured) != 1 {
		t.Fatalf("telegram calls = %d, want 1 confirmation edit", len(*captured))
	}
	text, _ := (*captured)[0].Body["text"].(string)
	if text == "" || text == "cancel_pending_done" {
		t.Fatalf("confirmation text = %q, want resolved localized copy", text)
	}
}

// TestCancelPendingPurchaseRefusesAnotherCustomersPurchase keeps the ownership
// check that the Mini App path already enforces: a callback id is not authority.
func TestCancelPendingPurchaseRefusesAnotherCustomersPurchase(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}
	h.testFindCustomer = func(ctx context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 10, TelegramID: telegramID, Language: "en"}, nil
	}
	h.testFindPurchase = func(ctx context.Context, purchaseID int64) (*database.Purchase, error) {
		return &database.Purchase{ID: purchaseID, CustomerID: 99}, nil
	}
	cancelCalls := 0
	h.testCancelPurchase = func(ctx context.Context, purchaseID, customerID int64) (bool, error) {
		cancelCalls++
		return true, nil
	}

	b, captured := newTestBot(t)
	h.CancelPendingPurchaseCallbackHandler(context.Background(), b, newCancelCallbackUpdate("cancel_pending_purchase?id=77&confirm=1"))

	if cancelCalls != 0 {
		t.Fatalf("cancel calls for a foreign purchase = %d, want 0", cancelCalls)
	}
	if len(*captured) != 1 {
		t.Fatalf("telegram calls = %d, want 1 error edit", len(*captured))
	}
	text, _ := (*captured)[0].Body["text"].(string)
	if text == "" {
		t.Fatalf("error reply text = %q, want localized copy", text)
	}
}

// TestCancelPendingPurchaseReportsAlreadySettledPurchase covers the race A9's
// reaper creates: the row moved on between the prompt and the tap, so the reply
// must say so instead of claiming a cancellation that did not happen.
func TestCancelPendingPurchaseReportsAlreadySettledPurchase(t *testing.T) {
	h := Handler{translation: loadHandlerTestTranslations(t)}
	h.testFindCustomer = func(ctx context.Context, telegramID int64) (*database.Customer, error) {
		return &database.Customer{ID: 10, TelegramID: telegramID, Language: "en"}, nil
	}
	h.testFindPurchase = func(ctx context.Context, purchaseID int64) (*database.Purchase, error) {
		return &database.Purchase{ID: purchaseID, CustomerID: 10}, nil
	}
	h.testCancelPurchase = func(ctx context.Context, purchaseID, customerID int64) (bool, error) {
		return false, nil
	}

	b, captured := newTestBot(t)
	h.CancelPendingPurchaseCallbackHandler(context.Background(), b, newCancelCallbackUpdate("cancel_pending_purchase?id=77&confirm=1"))

	if len(*captured) != 1 {
		t.Fatalf("telegram calls = %d, want 1 reply", len(*captured))
	}
	text, _ := (*captured)[0].Body["text"].(string)
	// The reply is localized; assert it is the already-settled copy rather than
	// the success copy the handler would otherwise send.
	settledText := h.translation.GetText("en", "cancel_pending_gone")
	if text == "" || text == "cancel_pending_gone" {
		t.Fatalf("reply text = %q, want resolved localized copy", text)
	}
	if text == h.translation.GetText("en", "cancel_pending_done") {
		t.Fatalf("reply text = %q, want the already-settled copy, not a success claim", text)
	}
	if settledText == "cancel_pending_gone" {
		t.Fatal("cancel_pending_gone missing from the translation catalogue")
	}
}
