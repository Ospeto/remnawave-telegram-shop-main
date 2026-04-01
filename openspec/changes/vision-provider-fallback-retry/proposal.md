## Why

Mobile screenshot verification currently depends on a single Gemini API key and single provider path. Transient provider failures (timeouts, rate limits, key issues) cause immediate payment verification failures even when a secondary provider could succeed safely.

## What Changes

- Add a provider-neutral vision verification layer for payment screenshots.
- Add OpenRouter as fallback provider when primary Gemini attempts fail for retriable/provider errors.
- Add bounded retry/failover policy with explicit error classification.
- Keep existing business validation fail-closed (amount/recipient/duplicate/note checks unchanged).
- Add configuration for OpenRouter keys/model and retry settings.
- Update setup/docs so operators can configure fallback behavior safely.

## Capabilities

### New Capabilities

- `mobile-payment-vision-fallback`: Reliable screenshot analysis with controlled retry and provider fallback for provider failures.

### Modified Capabilities

- None.

## Impact

- Core payment verification flow in `internal/payment/payment.go`.
- Gemini integration and shared prompt/parser logic in `internal/gemini/`.
- App config parsing and startup wiring in `internal/config/config.go` and `cmd/app/main.go`.
- Operator configuration UX in `.env.sample`, `setup.sh`, and `readme.md`.
