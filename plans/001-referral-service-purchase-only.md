# Plan 001: Restrict referral conversion bonuses to service purchases

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report - do not improvise. When done, update the status row for this plan in `plans/README.md` unless a reviewer dispatched you and told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 8e80b0b..HEAD -- internal/payment/payment.go internal/payment/payment_test.go internal/database/purchase.go translations/en.json`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `8e80b0b`, 2026-06-20

## Why This Matters

Referral bonuses move wallet balance. Today a referred customer can trigger `processReferralBonus` after a wallet top-up because wallet top-ups are fulfilled through the same successful purchase path as service purchases. Product copy and comments describe the trigger as a friend making their first purchase, meaning a service conversion, not merely adding stored balance. This plan makes that business rule explicit and tests it before changing behavior.

## Current State

- `internal/payment/payment.go` - successful purchase fulfillment and referral bonus grant.
- `internal/payment/payment_test.go` - existing payment unit tests; has wallet top-up helper tests but no referral-exclusion test.
- `internal/database/purchase.go` - contains an unused helper whose comment says referral eligibility is based on any paid invoice type.
- `translations/en.json` - user-facing referral copy.

Current excerpts:

```go
// internal/payment/payment.go:1012
if purchase.InvoiceType == database.InvoiceTypeWalletTopUp {
    // WALLET TOP-UP - balance credit and transaction log must be atomic.
    if err := settleWalletTopUp(...); err != nil {
        ...
    }
} else if purchase.ExtendKeyID != nil {
    ...
}
```

```go
// internal/payment/payment.go:1118
if err := s.purchaseRepository.MarkAsPaid(context.WithoutCancel(ctx), purchase.ID); err != nil {
    ...
}
...
// internal/payment/payment.go:1148
// Referral bonus - completely non-fatal
s.processReferralBonus(ctx, notifyCustomer)
```

```go
// internal/payment/payment.go:1171
// processReferralBonus grants a 1,000 MMK wallet bonus to both the referrer and
// the referee (new buyer) when the referee completes their first purchase.
```

```go
// translations/en.json:32
"referral_bonus_granted": "... A friend you referred just made their first purchase."
```

Repo conventions:

- Tests use table-driven subtests and `t.Fatalf`, as in `internal/payment/payment_test.go:233-252`.
- Money-flow helpers are small package-level functions where useful, as shown by `settleWalletTopUp` and its direct tests at `internal/payment/payment_test.go:888-948`.
- Commit messages in recent history use conventional prefixes like `fix: ...` and `feat: ...`.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Targeted tests | `go test ./internal/payment` | exit 0 |
| Full Go tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Build | `go build ./cmd/app` | exit 0 |

## Scope

**In scope**:
- `internal/payment/payment.go`
- `internal/payment/payment_test.go`
- `internal/database/purchase.go` only if you update the stale referral-eligibility comment or helper to match the new rule

**Out of scope**:
- Do not change wallet top-up settlement, receipt verification, purchase status transitions, or wallet balance math.
- Do not change the referral bonus amount or admin `/setreferralbonus` behavior.
- Do not change user-facing referral copy unless it is necessary to keep wording consistent with the service-purchase rule.

## Git Workflow

- Branch: `codex/001-referral-service-purchase-only`
- Commit message: `fix: restrict referral bonuses to service purchases`
- Do not push or open a PR unless instructed.

## Steps

### Step 1: Add a characterization helper for referral conversion invoice types

In `internal/payment/payment.go`, add a small unexported helper near the other payment helpers:

```go
func triggersReferralConversion(invoiceType database.InvoiceType) bool {
    switch invoiceType {
    case database.InvoiceTypeMobileBanking, database.InvoiceTypeWalletPayment, database.InvoiceTypeCrypto:
        return true
    case database.InvoiceTypeWalletTopUp:
        return false
    default:
        return false
    }
}
```

Do not invent new invoice type constants. At planning time the Go constants are `InvoiceTypeCrypto`, `InvoiceTypeMobileBanking`, `InvoiceTypeWalletTopUp`, and `InvoiceTypeWalletPayment`.

Add a table-driven unit test in `internal/payment/payment_test.go` that asserts:

- `mobile_banking` triggers referral conversion.
- `wallet_payment` triggers referral conversion.
- `wallet_topup` does not trigger referral conversion.
- Unknown invoice type does not trigger referral conversion.

**Verify**: `go test ./internal/payment` -> exit 0 and the new test passes.

### Step 2: Gate referral bonus execution after successful fulfillment

In `ProcessSuccessfulPayment` or the live function that contains `s.processReferralBonus(ctx, notifyCustomer)`, wrap that call:

```go
if triggersReferralConversion(purchase.InvoiceType) {
    s.processReferralBonus(ctx, notifyCustomer)
}
```

Keep notification behavior unchanged unless a test proves wallet top-ups are incorrectly receiving subscription activation messages too. If that turns out to be true, treat it as a STOP condition because it broadens the behavior change beyond referral conversion.

**Verify**: `go test ./internal/payment` -> exit 0.

### Step 3: Fix stale comments if needed

If `internal/database/purchase.go:367-370` still claims referral eligibility is based on any paid invoice type and that helper remains unused, update the comment to avoid misleading future maintainers. Do not delete that helper in this plan unless the compiler or tests force it; dead-code cleanup belongs in a separate pass.

**Verify**: `go test ./internal/database ./internal/payment` -> exit 0.

## Test Plan

- Add `TestTriggersReferralConversion` in `internal/payment/payment_test.go`, modeled after `TestSupportsScreenshotVerification`.
- Existing wallet top-up settlement tests must continue passing.
- Full verification: `go test ./...`, `go vet ./...`, and `go build ./cmd/app`.

## Done Criteria

- [ ] `go test ./internal/payment` exits 0.
- [ ] `go test ./...` exits 0.
- [ ] `go vet ./...` exits 0.
- [ ] `go build ./cmd/app` exits 0.
- [ ] A test proves `database.InvoiceTypeWalletTopUp` does not trigger referral conversion.
- [ ] Wallet-funded service purchases still trigger referral conversion.
- [ ] `plans/README.md` status row updated.

## STOP Conditions

Stop and report back if:

- The code around `s.processReferralBonus(ctx, notifyCustomer)` no longer matches the current-state excerpt.
- The only way to test the change requires adding broad mocks for Telegram, Remnawave, or a live database.
- You discover product requirements that intentionally award referral bonuses for wallet top-ups.
- The fix appears to require changing wallet settlement or purchase status semantics.

## Maintenance Notes

When adding new invoice types later, reviewers must decide whether they represent service access or stored-value movement and update `triggersReferralConversion` tests at the same time.
