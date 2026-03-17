package receiptai

import (
	"context"
	"fmt"
	"time"
)

type ProviderStatus struct {
	Role string
	Name string
	Err  error
}

type HealthReporter interface {
	HealthReport(ctx context.Context) []ProviderStatus
}

type FailoverAnalyzer struct {
	primary            Analyzer
	fallback           Analyzer
	perProviderTimeout time.Duration
}

func NewFailoverAnalyzer(primary Analyzer, fallback Analyzer) Analyzer {
	switch {
	case primary != nil && fallback != nil:
		return &FailoverAnalyzer{
			primary:            primary,
			fallback:           fallback,
			perProviderTimeout: 15 * time.Second,
		}
	case primary != nil:
		return primary
	default:
		return fallback
	}
}

func (a *FailoverAnalyzer) ProviderName() string {
	if a == nil {
		return "Receipt AI"
	}
	if a.primary != nil && a.fallback != nil {
		return fmt.Sprintf("%s (fallback: %s)", a.primary.ProviderName(), a.fallback.ProviderName())
	}
	if a.primary != nil {
		return a.primary.ProviderName()
	}
	if a.fallback != nil {
		return a.fallback.ProviderName()
	}
	return "Receipt AI"
}

func (a *FailoverAnalyzer) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string) (*PaymentInfo, error) {
	if a.primary == nil && a.fallback == nil {
		return nil, fmt.Errorf("receipt analyzer is not configured")
	}
	if a.primary == nil {
		return a.runAnalyze(ctx, a.fallback, imageBytes, mimeType)
	}

	info, primaryErr := a.runAnalyze(ctx, a.primary, imageBytes, mimeType)
	if primaryErr == nil {
		return info, nil
	}
	if a.fallback == nil {
		return nil, fmt.Errorf("%s failed: %w", a.primary.ProviderName(), primaryErr)
	}

	info, fallbackErr := a.runAnalyze(ctx, a.fallback, imageBytes, mimeType)
	if fallbackErr == nil {
		return info, nil
	}

	return nil, fmt.Errorf("%s failed: %w; %s fallback failed: %w", a.primary.ProviderName(), primaryErr, a.fallback.ProviderName(), fallbackErr)
}

func (a *FailoverAnalyzer) CheckHealth(ctx context.Context) error {
	if a.primary == nil && a.fallback == nil {
		return fmt.Errorf("receipt analyzer is not configured")
	}
	if a.primary == nil {
		return a.runHealthCheck(ctx, a.fallback)
	}

	primaryErr := a.runHealthCheck(ctx, a.primary)
	if primaryErr == nil || a.fallback == nil {
		return primaryErr
	}

	fallbackErr := a.runHealthCheck(ctx, a.fallback)
	if fallbackErr == nil {
		return nil
	}

	return fmt.Errorf("%s failed: %w; %s fallback failed: %w", a.primary.ProviderName(), primaryErr, a.fallback.ProviderName(), fallbackErr)
}

func (a *FailoverAnalyzer) HealthReport(ctx context.Context) []ProviderStatus {
	if a == nil {
		return nil
	}

	var report []ProviderStatus
	if a.primary != nil {
		report = append(report, ProviderStatus{
			Role: "Primary",
			Name: a.primary.ProviderName(),
			Err:  a.runHealthCheck(ctx, a.primary),
		})
	}
	if a.fallback != nil {
		report = append(report, ProviderStatus{
			Role: "Fallback",
			Name: a.fallback.ProviderName(),
			Err:  a.runHealthCheck(ctx, a.fallback),
		})
	}
	return report
}

func (a *FailoverAnalyzer) childContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.perProviderTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, a.perProviderTimeout)
}

func (a *FailoverAnalyzer) runAnalyze(ctx context.Context, analyzer Analyzer, imageBytes []byte, mimeType string) (*PaymentInfo, error) {
	childCtx, cancel := a.childContext(ctx)
	defer cancel()
	return analyzer.AnalyzePaymentScreenshot(childCtx, imageBytes, mimeType)
}

func (a *FailoverAnalyzer) runHealthCheck(ctx context.Context, analyzer Analyzer) error {
	childCtx, cancel := a.childContext(ctx)
	defer cancel()
	return analyzer.CheckHealth(childCtx)
}
