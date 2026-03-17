package receiptai

import (
	"context"
	"errors"
	"testing"
)

type stubAnalyzer struct {
	name         string
	healthErr    error
	analyzeErr   error
	result       *PaymentInfo
	analyzeCalls int
	healthCalls  int
}

func (s *stubAnalyzer) ProviderName() string {
	return s.name
}

func (s *stubAnalyzer) CheckHealth(ctx context.Context) error {
	s.healthCalls++
	return s.healthErr
}

func (s *stubAnalyzer) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string) (*PaymentInfo, error) {
	s.analyzeCalls++
	if s.analyzeErr != nil {
		return nil, s.analyzeErr
	}
	return s.result, nil
}

func TestFailoverAnalyzerPrimarySuccess(t *testing.T) {
	primary := &stubAnalyzer{name: "Gemini", result: &PaymentInfo{Provider: "kpay"}}
	fallback := &stubAnalyzer{name: "OpenRouter", result: &PaymentInfo{Provider: "wavepay"}}
	analyzer, ok := NewFailoverAnalyzer(primary, fallback).(*FailoverAnalyzer)
	if !ok {
		t.Fatal("expected failover analyzer")
	}
	analyzer.perProviderTimeout = 0

	info, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("img"), "image/png")
	if err != nil {
		t.Fatalf("AnalyzePaymentScreenshot returned error: %v", err)
	}
	if info.Provider != "kpay" {
		t.Fatalf("expected primary result, got %+v", info)
	}
	if primary.analyzeCalls != 1 {
		t.Fatalf("expected primary to be called once, got %d", primary.analyzeCalls)
	}
	if fallback.analyzeCalls != 0 {
		t.Fatalf("expected fallback to be skipped, got %d calls", fallback.analyzeCalls)
	}
}

func TestFailoverAnalyzerFallsBackOnPrimaryError(t *testing.T) {
	primary := &stubAnalyzer{name: "Gemini", analyzeErr: errors.New("gemini down")}
	fallback := &stubAnalyzer{name: "OpenRouter", result: &PaymentInfo{Provider: "wavepay"}}
	analyzer, ok := NewFailoverAnalyzer(primary, fallback).(*FailoverAnalyzer)
	if !ok {
		t.Fatal("expected failover analyzer")
	}
	analyzer.perProviderTimeout = 0

	info, err := analyzer.AnalyzePaymentScreenshot(context.Background(), []byte("img"), "image/png")
	if err != nil {
		t.Fatalf("AnalyzePaymentScreenshot returned error: %v", err)
	}
	if info.Provider != "wavepay" {
		t.Fatalf("expected fallback result, got %+v", info)
	}
	if primary.analyzeCalls != 1 || fallback.analyzeCalls != 1 {
		t.Fatalf("expected both analyzers to run once, got primary=%d fallback=%d", primary.analyzeCalls, fallback.analyzeCalls)
	}
}

func TestFailoverAnalyzerHealthReport(t *testing.T) {
	primary := &stubAnalyzer{name: "Gemini"}
	fallback := &stubAnalyzer{name: "OpenRouter", healthErr: errors.New("openrouter down")}
	analyzer, ok := NewFailoverAnalyzer(primary, fallback).(*FailoverAnalyzer)
	if !ok {
		t.Fatal("expected failover analyzer")
	}
	analyzer.perProviderTimeout = 0

	report := analyzer.HealthReport(context.Background())
	if len(report) != 2 {
		t.Fatalf("expected 2 health entries, got %d", len(report))
	}
	if report[0].Role != "Primary" || report[0].Name != "Gemini" || report[0].Err != nil {
		t.Fatalf("unexpected primary report: %+v", report[0])
	}
	if report[1].Role != "Fallback" || report[1].Name != "OpenRouter" || report[1].Err == nil {
		t.Fatalf("unexpected fallback report: %+v", report[1])
	}
}
