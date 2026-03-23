package cryptopay

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCreateInvoiceHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	client := NewCryptoPayClient("https://example.invalid", "token")
	client.httpClient.Timeout = 5 * time.Second
	client.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.CreateInvoice(ctx, &InvoiceRequest{Fiat: "MMK", Amount: "100"})
	if err == nil {
		t.Fatal("expected CreateInvoice to fail when context is canceled")
	}

	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context-related error, got %v", err)
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("CreateInvoice ignored context cancellation and took too long: %s", elapsed)
	}
}
