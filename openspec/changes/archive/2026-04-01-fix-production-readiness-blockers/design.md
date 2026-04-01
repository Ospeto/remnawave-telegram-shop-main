## Context

This change fixes five concrete production-readiness findings spread across the React mini-app and the Go runtime/deployment path. The frontend defects are localized state/routing issues, while the runtime issues affect container health, backup retry semantics, and the safety of destructive restore operations.

The codebase already exposes an HTTP health endpoint, a scheduled backup service, and Telegram admin commands for backup/restore. The goal here is not to redesign those systems, but to make the current release path safe and predictable with the smallest coherent set of changes.

## Goals / Non-Goals

**Goals:**
- Make wallet top-up flow through checkout without invalid plan routing.
- Ensure home-screen retry clears stale error state and recovers normally.
- Use a probe path that checks the running service instead of starting a second app process.
- Ensure scheduled backups are retried after failures instead of being prematurely marked complete.
- Prevent destructive runtime restores from executing in the live bot process.

**Non-Goals:**
- Reworking the broader payment UX, promo math, or router architecture.
- Building a maintenance-mode restore workflow in this change.
- Adding full observability, off-host backups, or full end-to-end test coverage for every payment path.

## Decisions

### Use top-up-specific routing state instead of a sentinel plan index
Wallet top-up is not a subscription plan purchase, so checkout should not require a valid `planIndex`. The fix will treat `walletTopup=true` and `amount` as the authoritative state for that branch.

Alternative considered:
- Keep using `-1` as a sentinel and special-case it deeper in checkout. Rejected because it keeps invalid route state as part of the public contract and is easy to regress again.

### Clear stale error state before retrying home data load
The home screen already has a retry action; the failure is state hygiene, not missing behavior. The fix will reset the error before issuing another request.

Alternative considered:
- Force a full page reload on retry. Rejected because it is heavier, less predictable inside Telegram WebApp, and unnecessary for the defect.

### Add a lightweight liveness endpoint for container healthchecks
The deployment will probe a lightweight HTTP liveness endpoint on the running process rather than executing the application binary again. This keeps container liveness separate from downstream dependency readiness.

Alternative considered:
- Reuse the existing `/healthcheck` endpoint. Rejected because it checks downstream dependencies and is better treated as readiness/diagnostic state than container liveness.

### Mark scheduled backup completion only after successful local backup creation
The current bug is ordering, not scheduling capability. The scheduler will persist `last scheduled date` only after the local backup succeeds so failures remain retryable.

Alternative considered:
- Add a second “attempted” timestamp and reconcile multiple states. Rejected because the review finding can be fixed with simpler success-only bookkeeping.

### Disable runtime restore execution in the live bot process
The running process cannot safely guarantee an offline restore boundary. The minimal production-safe fix is to reject runtime restore commands and direct operators to offline/manual restore.

Alternative considered:
- Add partial safeguards around the existing live restore flow. Rejected because they do not remove the core risk of applying destructive SQL against a live database.

## Risks / Trade-offs

- [Restore command behavior changes for operators] → Update command responses and runbook text to make offline/manual restore explicit.
- [Liveness and readiness semantics may still evolve later] → Keep this change focused on adding a cheap liveness probe while preserving the richer health endpoint for diagnostics.
- [Top-up flow logic may rely on plan-derived UI in a few places] → Keep top-up branch explicitly separated and verify wallet top-up in the mini-app after build/test.

## Migration Plan

1. Update mini-app routing/state handling for wallet top-up and retry recovery.
2. Update deployment healthcheck to target the running HTTP service.
3. Update backup scheduling bookkeeping to record success only after local backup creation.
4. Disable runtime restore command execution and surface an explicit operator-facing error.
5. Verify with `go test ./...`, `go build ./cmd/app`, and `npm run build`.
6. Deploy with the updated compose healthcheck and keep runtime restore disabled.

Rollback:
- Frontend fixes can be reverted independently if needed.
- Healthcheck and backup bookkeeping can be reverted independently.
- Runtime restore rejection should only be rolled back together with a safer restore design, not by re-enabling the live destructive path.

## Open Questions

- None for implementation. The requested fixes are specific enough to apply directly.
