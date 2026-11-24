package handler

import (
	"context"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// TestPanicRecoveryMiddlewareContainsHandlerPanic is the A1 regression test: a
// panicking handler must not take the whole bot process down for every
// customer.
func TestPanicRecoveryMiddlewareContainsHandlerPanic(t *testing.T) {
	called := false
	handler := func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		called = true
		panic("panel client exploded")
	}

	wrapped := PanicRecoveryMiddleware(handler)

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("PanicRecoveryMiddleware propagated panic: %v", recovered)
			}
		}()
		wrapped(context.Background(), nil, &models.Update{ID: 42})
	}()

	if !called {
		t.Fatal("PanicRecoveryMiddleware did not invoke the wrapped handler")
	}
}

// TestPanicRecoveryMiddlewarePassesThroughNormalUpdates guards against a
// recovery wrapper that swallows ordinary handler work.
func TestPanicRecoveryMiddlewarePassesThroughNormalUpdates(t *testing.T) {
	calls := 0
	handler := func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		calls++
	}

	wrapped := PanicRecoveryMiddleware(handler)
	wrapped(context.Background(), nil, &models.Update{ID: 7})

	if calls != 1 {
		t.Fatalf("wrapped handler calls = %d, want 1", calls)
	}
}
