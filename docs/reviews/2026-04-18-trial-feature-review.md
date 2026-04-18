# Trial Feature Review

Date: 2026-04-18
Branch: `main`

## Initial Findings

1. High: concurrent trial activations could stack extra free days
2. High: Telegram callback flow and Mini App API used different eligibility rules
3. High: trial usage was not durably recorded and could be forgotten after cleanup
4. Medium: targeted trial coverage was missing in backend and frontend

## Fixes Applied

1. Added durable customer-level trial tracking with `trial_used_at`
2. Added migration `000031_add_trial_tracking` and backfilled existing subscription history
3. Moved trial activation to a locked backend flow using `FindByTelegramIdForUpdateTx`
4. Centralized trial eligibility in `PaymentService`
5. Updated Telegram callback flow to rely on the same eligibility path as Mini App activation
6. Prevented Remnawave trial retries from extending an existing user again
7. Added backend regression tests for `/api/me`, `/api/trial`, and payment trial helpers
8. Added frontend regression coverage for the Home trial conflict state

## Verification

```bash
go test ./...
npm test -- --run src/pages/Home.test.tsx src/pages/Plans.test.tsx src/pages/Checkout.test.tsx
npm run build
```

## Second Review Pass

No significant issues found in the updated trial flow.

Residual note:
- The activation path intentionally holds a row lock while creating the trial user remotely. That is a reasonable tradeoff here because it closes the double-activation hole and the operation is rare.
