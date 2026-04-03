## Context

The mobile-payment flow already classifies provider failures into retry/failover-friendly error classes, including `auth`. However, `VerifyMobilePayment` currently treats every analyzer error the same way and returns a generic screenshot-verification failure. That is correct for malformed receipts and ambiguous model output, but it is misleading for OpenRouter auth failures because:

- the buyer cannot fix the problem by re-uploading a better screenshot
- the operator needs a direct signal that `OPENROUTER_API_KEY` is broken

The app already has two useful surfaces:

- a Telegram bot capable of notifying the configured admin
- a screenshot upload response that can return a specific failure message to the buyer

## Goals / Non-Goals

**Goals**
- Alert the operator when OpenRouter auth is failing during screenshot verification.
- Avoid alert spam when many uploads fail against the same broken key.
- Return a user-facing failure that reflects temporary verification unavailability rather than screenshot invalidity.
- Preserve the existing fail-closed business validation for amount, recipient, duplicate transaction, and forbidden-note checks.

**Non-Goals**
- Changing how non-auth screenshot failures are messaged.
- Building a general alerting framework for every provider failure class.
- Persisting alert cooldown state in the database.
- Exposing provider internals such as `OPENROUTER_API_KEY` directly to buyers.

## Decisions

1. Only OpenRouter auth failures trigger the new operator alert path.
Rationale: this change is solving the explicit OpenRouter API-key failure mode. Other provider failures already surface in health/readiness and logs, and broadening the scope would change support behavior more than requested.

2. Buyers receive a generic temporary-unavailable message, not provider internals.
Rationale: the buyer needs the correct action ("try again later or contact support"), not backend credential details.

3. Admin alerts are deduplicated in-memory with a fixed cooldown.
Rationale: a broken key can cause every upload to fail. A simple per-provider cooldown prevents spam while still alerting quickly after the first failure and again after restarts or prolonged outages.

4. The Telegram bot remains the operator alert channel.
Rationale: the repo already uses admin Telegram notifications, and this is the fastest human-visible signal available without adding new infrastructure.

## Proposed Flow

```text
Screenshot upload
      |
      v
AnalyzePaymentScreenshot
      |
      +--> non-auth error / semantic failure
      |         |
      |         v
      |   existing failure handling
      |
      +--> OpenRouter auth error
                |
                +--> send deduped admin Telegram alert
                |
                +--> return "verification temporarily unavailable"
```

## Implementation Notes

- Add helper logic in `internal/payment/payment.go` to:
  - detect whether an analyzer error is an OpenRouter auth failure
  - build the user-facing temporary-unavailable result
  - throttle admin alerts by provider key
- Use a fixed cooldown, stored in-memory on `PaymentService`.
- Cover both provider names currently used by the app:
  - `openrouter`
  - `openrouter-fallback`
- Add a dedicated translation key for the Telegram bot failure response.

## Risks / Trade-offs

- Alert state is process-local, so restarts clear the cooldown.
  Acceptable because the first failure after restart should alert again.

- The web API still returns raw message text rather than a translation key-driven localized response.
  Acceptable for this change because the buyer-facing message remains accurate and the Telegram bot path stays localized.

- Focusing only on OpenRouter auth means Gemini auth still falls back to existing behavior.
  Acceptable because this change targets the OpenRouter credential outage explicitly requested by the user.
