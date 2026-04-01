package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type AnalyzerOptions struct {
	Primary        Provider
	Fallback       Provider
	RetryAttempts  int
	MaxAttempts    int
	AttemptTimeout time.Duration
}

type fallbackAnalyzer struct {
	primary        Provider
	fallback       Provider
	retryAttempts  int
	maxAttempts    int
	attemptTimeout time.Duration
}

const defaultAnalyzerAttemptTimeout = 20 * time.Second

func NewAnalyzer(options AnalyzerOptions) Analyzer {
	if options.Primary == nil {
		return nil
	}

	maxAttempts := options.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
		if options.Fallback != nil {
			maxAttempts++
		}
		if options.RetryAttempts > 0 {
			maxAttempts += options.RetryAttempts
			if options.Fallback != nil {
				maxAttempts += options.RetryAttempts
			}
		}
	}

	if maxAttempts < 1 {
		maxAttempts = 1
	}

	if options.RetryAttempts < 0 {
		options.RetryAttempts = 0
	}

	attemptTimeout := options.AttemptTimeout
	if attemptTimeout <= 0 {
		attemptTimeout = defaultAnalyzerAttemptTimeout
	}

	return &fallbackAnalyzer{
		primary:        options.Primary,
		fallback:       options.Fallback,
		retryAttempts:  options.RetryAttempts,
		maxAttempts:    maxAttempts,
		attemptTimeout: attemptTimeout,
	}
}

func (a *fallbackAnalyzer) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string, providers []ConfiguredProvider) (*PaymentInfo, error) {
	order := []Provider{a.primary}
	if a.fallback != nil && a.fallback.Name() != a.primary.Name() {
		order = append(order, a.fallback)
	}

	attemptsByProvider := make(map[string]int, len(order))
	currentProviderIdx := 0
	var lastErr error

	for totalAttempt := 1; totalAttempt <= a.maxAttempts; totalAttempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		provider := order[currentProviderIdx]
		providerName := provider.Name()
		attemptsByProvider[providerName]++
		providerAttempt := attemptsByProvider[providerName]

		slog.Info("Vision analyzer attempt",
			"provider", providerName,
			"attempt", totalAttempt,
			"provider_attempt", providerAttempt,
			"max_attempts", a.maxAttempts,
		)

		attemptCtx, cancel := a.attemptContext(ctx, totalAttempt)
		info, err := provider.AnalyzePaymentScreenshot(attemptCtx, imageBytes, mimeType, providers)
		cancel()
		if err == nil {
			outcome := "accepted"
			if info != nil && !info.IsValid {
				outcome = "semantic_negative"
			}
			slog.Info("Vision analyzer attempt complete",
				"provider", providerName,
				"attempt", totalAttempt,
				"outcome", outcome,
			)
			return info, nil
		}

		providerErr, ok := AsProviderError(err)
		if !ok {
			providerErr = &ProviderError{
				Provider: providerName,
				Class:    ErrorClassUnknown,
				Err:      err,
				Message:  err.Error(),
			}
		}

		lastErr = providerErr
		slog.Warn("Vision analyzer attempt failed",
			"provider", providerName,
			"attempt", totalAttempt,
			"provider_attempt", providerAttempt,
			"error_class", providerErr.Class,
			"status_code", providerErr.StatusCode,
			"retryable", providerErr.AllowsRetry(),
			"failover_eligible", providerErr.AllowsFailover(),
			"error", providerErr.Error(),
		)

		if totalAttempt >= a.maxAttempts {
			break
		}

		if providerErr.AllowsRetry() && providerAttempt <= a.retryAttempts && a.canRetryCurrentProvider(currentProviderIdx, totalAttempt, len(order)) {
			continue
		}

		if providerErr.AllowsFailover() && currentProviderIdx+1 < len(order) {
			nextProvider := order[currentProviderIdx+1]
			slog.Warn("Vision analyzer fallback triggered",
				"from_provider", providerName,
				"to_provider", nextProvider.Name(),
				"attempt", totalAttempt,
				"error_class", providerErr.Class,
			)
			currentProviderIdx++
			continue
		}

		break
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("vision analyzer failed without a provider error")
	}

	slog.Error("Vision analyzer exhausted attempts",
		"primary_provider", a.primary.Name(),
		"fallback_provider", fallbackProviderName(a.fallback),
		"max_attempts", a.maxAttempts,
		"error", lastErr,
	)
	return nil, lastErr
}

func (a *fallbackAnalyzer) attemptContext(parent context.Context, totalAttempt int) (context.Context, context.CancelFunc) {
	timeout := a.attemptTimeout
	if timeout <= 0 {
		return parent, func() {}
	}

	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return parent, func() {}
		}

		attemptsLeft := a.maxAttempts - totalAttempt + 1
		if attemptsLeft < 1 {
			attemptsLeft = 1
		}
		slice := remaining / time.Duration(attemptsLeft)
		if slice <= 0 {
			slice = remaining
		}
		if slice < timeout {
			timeout = slice
		}
	}

	return context.WithTimeout(parent, timeout)
}

func (a *fallbackAnalyzer) canRetryCurrentProvider(currentProviderIdx, totalAttempt, providerCount int) bool {
	// Reserve at least one remaining attempt for fallback when a fallback provider exists.
	if currentProviderIdx+1 < providerCount {
		return totalAttempt < a.maxAttempts-1
	}
	return true
}

func (a *fallbackAnalyzer) Readiness(ctx context.Context) AnalyzerReadiness {
	providers := map[string]string{}
	status := "ok"

	primaryName := a.primary.Name()
	if err := a.primary.Ping(ctx); err != nil {
		providers[primaryName] = "error: " + err.Error()
		status = "degraded"
	} else {
		providers[primaryName] = "ok"
	}

	fallbackName := fallbackProviderName(a.fallback)
	if a.fallback != nil {
		if err := a.fallback.Ping(ctx); err != nil {
			providers[a.fallback.Name()] = "error: " + err.Error()
			if status == "ok" {
				status = "degraded"
			}
		} else {
			providers[a.fallback.Name()] = "ok"
		}
	}

	return AnalyzerReadiness{
		Status:    status,
		Primary:   primaryName,
		Fallback:  fallbackName,
		Providers: providers,
	}
}

func fallbackProviderName(provider Provider) string {
	if provider == nil {
		return ""
	}
	return provider.Name()
}
