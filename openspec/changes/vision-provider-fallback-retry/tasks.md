## 1. Analyzer Abstraction And Providers

- [x] 1.1 Refactor `internal/gemini` to expose a provider-neutral analyzer interface and keep a shared prompt/output contract.
- [x] 1.2 Add OpenRouter provider adapter for image+text analysis with normalized response parsing.
- [x] 1.3 Add retry/failover orchestrator with bounded attempts and error classification.

## 2. Payment Flow Integration

- [x] 2.1 Update `internal/payment/payment.go` to use the new analyzer abstraction instead of direct Gemini-only calls.
- [x] 2.2 Ensure semantic negative results do not trigger fallback and existing business validation remains unchanged.
- [x] 2.3 Add structured logs for provider attempt, error class, fallback trigger, and final outcome.

## 3. Configuration And Wiring

- [x] 3.1 Extend `internal/config/config.go` with OpenRouter + retry/fallback settings and backward-compatible defaults.
- [x] 3.2 Update `cmd/app/main.go` wiring so mobile banking initializes analyzer with primary/fallback configuration.
- [x] 3.3 Keep health check behavior compatible while reporting analyzer provider readiness.

## 4. Operator UX And Validation

- [x] 4.1 Update `.env.sample` and `setup.sh` prompts/env writes for OpenRouter fallback and retry settings.
- [x] 4.2 Update `readme.md` mobile-banking configuration docs for fallback behavior.
- [x] 4.3 Add/adjust tests for retry decision logic and provider fallback behavior.
