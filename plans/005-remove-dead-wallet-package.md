# Plan 005: Remove the unused duplicate wallet service package

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report - do not improvise. When done, update the status row for this plan in `plans/README.md` unless a reviewer dispatched you and told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 8e80b0b..HEAD -- internal/wallet internal/service/wallet cmd/app/main.go internal/api/handlers.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S-M
- **Risk**: LOW
- **Depends on**: `plans/001-referral-service-purchase-only.md`
- **Category**: tech-debt
- **Planned at**: commit `8e80b0b`, 2026-06-20

## Why This Matters

There are two wallet service packages. The app wires `internal/service/wallet`, while `internal/wallet` appears unused and untested. Keeping both implementations around raises maintenance risk in money paths because future changes can land in the wrong package or copy stale behavior.

## Current State

- `internal/service/wallet/wallet.go` - active wallet service used by the app and API.
- `internal/wallet/service.go` - older-looking wallet implementation with direct SQL and no observed imports.
- `cmd/app/main.go` - imports and constructs the active wallet service.
- `internal/api/handlers.go` - aliases the active wallet service package as `walletsvc`.

Current excerpts:

```go
// cmd/app/main.go:25
"remnawave-tg-shop-bot/internal/service/wallet"
```

```go
// cmd/app/main.go:252
walletService := wallet.NewWalletService(paymentService, customerRepository, purchaseRepository, remnawaveClient, b, tm, subKeyRepo, walletTxRepo)
```

```go
// internal/api/handlers.go:14
walletsvc "remnawave-tg-shop-bot/internal/service/wallet"
```

```go
// internal/wallet/service.go:14
type Service struct {
    pool         *pgxpool.Pool
    customerRepo *database.CustomerRepository
    walletTxRepo *database.WalletTransactionRepository
    purchaseRepo *database.PurchaseRepository
}
```

```go
// internal/service/wallet/wallet.go:28
func NewWalletService(
    paymentService *payment.PaymentService,
    customerRepo *database.CustomerRepository,
    ...
) *WalletService {
```

Repo conventions:

- Active services live under `internal/service/<name>`, for example `internal/service/autorenew`, `internal/service/backup`, and `internal/service/healthcheck`.
- Recent history favors narrow cleanup commits like `fix: ...` and `feat: ...`.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Import check | `rg "internal/wallet|NewService\\(" internal cmd --glob '*.go'` | no references to `internal/wallet`; ignore unrelated `NewService` matches after manual review |
| Go package list | `go list ./...` | exit 0 and no `remnawave-tg-shop-bot/internal/wallet` package |
| Full Go tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Build | `go build ./cmd/app` | exit 0 |

## Scope

**In scope**:
- Delete `internal/wallet/service.go`.
- Delete `internal/wallet/` if it becomes empty.
- Update any references only if the initial import check finds real usage.

**Out of scope**:
- Do not refactor `internal/service/wallet` behavior.
- Do not change wallet top-up minimum amount, balance deduction, auto-renew, or payment logic.
- Do not remove `internal/service/invoicechecker` in this plan; it belongs to a separate disabled CryptoPay cleanup decision.

## Git Workflow

- Branch: `codex/005-remove-dead-wallet-package`
- Commit message: `chore: remove unused wallet package`
- Do not push or open a PR unless instructed.

## Steps

### Step 1: Confirm `internal/wallet` is still unused

Run:

```bash
rg "remnawave-tg-shop-bot/internal/wallet|internal/wallet" .
```

Expected result: no source imports of `internal/wallet`. References in this plan file or generated reports do not count.

Also run:

```bash
go list ./...
```

Expected result before deletion: it may include `remnawave-tg-shop-bot/internal/wallet`. This confirms it is just a standalone package, not necessarily used.

**Verify**: If any production source imports `internal/wallet`, stop.

### Step 2: Remove the dead package

Delete `internal/wallet/service.go`. If `internal/wallet/` is empty afterward, remove the directory.

Do not edit the active `internal/service/wallet/wallet.go`.

**Verify**: `test ! -e internal/wallet/service.go` -> exit 0.

### Step 3: Verify package graph and tests

Run:

```bash
go list ./...
```

Expected result: exit 0 and no `remnawave-tg-shop-bot/internal/wallet` package in the output.

Run:

```bash
go test ./...
go vet ./...
go build ./cmd/app
```

Expected result: all exit 0.

## Test Plan

No new tests are required for deleting a proven-unused package. The verification is the package graph plus full tests/build.

## Done Criteria

- [ ] `internal/wallet/service.go` no longer exists.
- [ ] `go list ./...` exits 0 and does not list `remnawave-tg-shop-bot/internal/wallet`.
- [ ] `go test ./...` exits 0.
- [ ] `go vet ./...` exits 0.
- [ ] `go build ./cmd/app` exits 0.
- [ ] No active wallet behavior files were modified.
- [ ] `plans/README.md` status row updated.

## STOP Conditions

Stop and report back if:

- Any live code imports `remnawave-tg-shop-bot/internal/wallet`.
- Removing the package requires changing active wallet behavior.
- The package contains new files not reflected in this plan.

## Maintenance Notes

After this cleanup, wallet-related changes should happen in `internal/service/wallet` and payment settlement code only. Reviewers should reject reintroducing a parallel wallet service package without a clear migration plan.
