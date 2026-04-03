## Why

When mobile screenshot verification fails because the OpenRouter API key is invalid or unauthorized, the app currently collapses that outage into the same generic "could not verify screenshot" result shown for bad screenshots. Buyers get the wrong guidance, and operators do not get a direct alert that the OpenRouter credential is broken.

## What Changes

- Detect OpenRouter provider auth failures in the screenshot verification flow.
- Return a specific temporary-unavailable verification outcome instead of the generic screenshot-invalid guidance for that failure class.
- Send a deduplicated Telegram admin alert when OpenRouter auth is broken, with enough detail to check the credential quickly.
- Add tests for error classification and alert throttling behavior.

## Capabilities

### New Capabilities

- `mobile-payment-provider-auth-alerts`: Alert operators and surface a clearer user failure mode when screenshot verification is unavailable because the OpenRouter credential is not working.

### Modified Capabilities

- `mobile-payment-vision-fallback`: Extend the provider-fallback flow so OpenRouter auth failures are treated as an operational outage, not as a user screenshot problem.

## Impact

- Screenshot verification logic in `internal/payment/payment.go`
- Provider error classification helpers in `internal/gemini/`
- Telegram admin alerting via the existing bot wiring
- Bot translation strings in `translations/en.json` and `translations/my.json`
- Regression coverage in payment tests
