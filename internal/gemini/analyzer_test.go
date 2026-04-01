package gemini

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	name    string
	results []*PaymentInfo
	errors  []error
	analyze func(context.Context, []byte, string, []ConfiguredProvider) (*PaymentInfo, error)
	calls   int
}

func (f *fakeProvider) Name() string {
	return f.name
}

func (f *fakeProvider) AnalyzePaymentScreenshot(ctx context.Context, image []byte, mime string, providers []ConfiguredProvider) (*PaymentInfo, error) {
	if f.analyze != nil {
		f.calls++
		return f.analyze(ctx, image, mime, providers)
	}

	idx := f.calls
	f.calls++

	if idx < len(f.errors) && f.errors[idx] != nil {
		return nil, f.errors[idx]
	}
	if idx < len(f.results) {
		return f.results[idx], nil
	}
	return nil, errors.New("unexpected call")
}

func (f *fakeProvider) Ping(context.Context) error {
	return nil
}

func TestAnalyzerSemanticNegativeStopsFallback(t *testing.T) {
	primary := &fakeProvider{
		name: "gemini",
		results: []*PaymentInfo{
			{
				Provider:      "kpay",
				TransactionID: "TX-1",
				IsValid:       false,
			},
		},
	}
	fallback := &fakeProvider{
		name: "openrouter",
		results: []*PaymentInfo{
			{
				Provider:      "wavepay",
				TransactionID: "TX-2",
				IsValid:       true,
			},
		},
	}

	analyzer := NewAnalyzer(AnalyzerOptions{
		Primary:       primary,
		Fallback:      fallback,
		RetryAttempts: 1,
		MaxAttempts:   3,
	})

	info, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("image"), "image/png", nil)
	if err != nil {
		t.Fatalf("AnalyzePaymentScreenshot() error = %v", err)
	}
	if info == nil || info.IsValid {
		t.Fatalf("AnalyzePaymentScreenshot() info = %+v, want semantic negative result", info)
	}
	if primary.calls != 1 {
		t.Fatalf("primary calls = %d, want 1", primary.calls)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}
}

func TestAnalyzerRetriesPrimaryBeforeFallback(t *testing.T) {
	primary := &fakeProvider{
		name: "gemini",
		errors: []error{
			&ProviderError{Provider: "gemini", Class: ErrorClassTimeout, Message: "timeout"},
		},
		results: []*PaymentInfo{
			nil,
			{
				Provider:      "kpay",
				TransactionID: "TX-1",
				IsValid:       true,
			},
		},
	}
	fallback := &fakeProvider{name: "openrouter"}

	analyzer := NewAnalyzer(AnalyzerOptions{
		Primary:       primary,
		Fallback:      fallback,
		RetryAttempts: 1,
		MaxAttempts:   3,
	})

	info, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("image"), "image/png", nil)
	if err != nil {
		t.Fatalf("AnalyzePaymentScreenshot() error = %v", err)
	}
	if info == nil || !info.IsValid {
		t.Fatalf("AnalyzePaymentScreenshot() info = %+v, want valid primary retry result", info)
	}
	if primary.calls != 2 {
		t.Fatalf("primary calls = %d, want 2", primary.calls)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback calls = %d, want 0", fallback.calls)
	}
}

func TestAnalyzerFallsBackOnProviderFailure(t *testing.T) {
	primary := &fakeProvider{
		name: "gemini",
		errors: []error{
			&ProviderError{Provider: "gemini", Class: ErrorClassServer, StatusCode: 503, Message: "upstream unavailable"},
		},
	}
	fallback := &fakeProvider{
		name: "openrouter",
		results: []*PaymentInfo{
			{
				Provider:      "wavepay",
				TransactionID: "TX-2",
				IsValid:       true,
			},
		},
	}

	analyzer := NewAnalyzer(AnalyzerOptions{
		Primary:       primary,
		Fallback:      fallback,
		RetryAttempts: 0,
		MaxAttempts:   2,
	})

	info, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("image"), "image/png", nil)
	if err != nil {
		t.Fatalf("AnalyzePaymentScreenshot() error = %v", err)
	}
	if info == nil || info.Provider != "wavepay" {
		t.Fatalf("AnalyzePaymentScreenshot() info = %+v, want fallback result", info)
	}
	if primary.calls != 1 {
		t.Fatalf("primary calls = %d, want 1", primary.calls)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestAnalyzerAuthErrorFailsOverWithoutRetryingPrimary(t *testing.T) {
	primary := &fakeProvider{
		name: "gemini",
		errors: []error{
			&ProviderError{Provider: "gemini", Class: ErrorClassAuth, StatusCode: 401, Message: "unauthorized"},
		},
	}
	fallback := &fakeProvider{
		name: "openrouter",
		results: []*PaymentInfo{
			{
				Provider:      "wavepay",
				TransactionID: "TX-3",
				IsValid:       true,
			},
		},
	}

	analyzer := NewAnalyzer(AnalyzerOptions{
		Primary:       primary,
		Fallback:      fallback,
		RetryAttempts: 2,
		MaxAttempts:   3,
	})

	info, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("image"), "image/png", nil)
	if err != nil {
		t.Fatalf("AnalyzePaymentScreenshot() error = %v", err)
	}
	if info == nil || info.Provider != "wavepay" {
		t.Fatalf("AnalyzePaymentScreenshot() info = %+v, want fallback result", info)
	}
	if primary.calls != 1 {
		t.Fatalf("primary calls = %d, want 1 because auth errors should not retry", primary.calls)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestAnalyzerReservesAttemptForFallbackWhenBudgetIsTight(t *testing.T) {
	primary := &fakeProvider{
		name: "gemini",
		errors: []error{
			&ProviderError{Provider: "gemini", Class: ErrorClassTimeout, Message: "timeout"},
		},
	}
	fallback := &fakeProvider{
		name: "openrouter",
		results: []*PaymentInfo{
			{
				Provider:      "wavepay",
				TransactionID: "TX-4",
				IsValid:       true,
			},
		},
	}

	analyzer := NewAnalyzer(AnalyzerOptions{
		Primary:       primary,
		Fallback:      fallback,
		RetryAttempts: 5,
		MaxAttempts:   2,
	})

	info, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("image"), "image/png", nil)
	if err != nil {
		t.Fatalf("AnalyzePaymentScreenshot() error = %v", err)
	}
	if info == nil || !info.IsValid || info.Provider != "wavepay" {
		t.Fatalf("AnalyzePaymentScreenshot() info = %+v, want fallback success", info)
	}
	if primary.calls != 1 {
		t.Fatalf("primary calls = %d, want 1 (no retry when fallback slot must be preserved)", primary.calls)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestAnalyzerAppliesAttemptTimeoutWithoutParentDeadline(t *testing.T) {
	primary := &fakeProvider{
		name: "gemini",
		analyze: func(ctx context.Context, _ []byte, _ string, _ []ConfiguredProvider) (*PaymentInfo, error) {
			<-ctx.Done()
			return nil, &ProviderError{
				Provider: "gemini",
				Class:    ErrorClassTimeout,
				Message:  "attempt deadline reached",
				Err:      ctx.Err(),
			}
		},
	}

	analyzer := NewAnalyzer(AnalyzerOptions{
		Primary:        primary,
		RetryAttempts:  0,
		MaxAttempts:    1,
		AttemptTimeout: 40 * time.Millisecond,
	})

	start := time.Now()
	_, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("image"), "image/png", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("AnalyzePaymentScreenshot() expected error, got nil")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("AnalyzePaymentScreenshot() elapsed %v, want <= 250ms", elapsed)
	}
	if primary.calls != 1 {
		t.Fatalf("primary calls = %d, want 1", primary.calls)
	}

	providerErr, ok := AsProviderError(err)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Class != ErrorClassTimeout {
		t.Fatalf("providerErr.Class = %s, want %s", providerErr.Class, ErrorClassTimeout)
	}
}
