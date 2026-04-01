## Context

The current codebase already fixed the mini-app retry bug and now has frontend regression tests in the working tree, but production readiness is still blocked by backend and runtime concerns. The app process does not listen for `SIGTERM`, release checks are not enforced in CI, and the money-moving payment and auto-renew paths still rely mostly on helper-level tests.

This change crosses runtime bootstrap, repository automation, and backend service packages. The main constraint is to improve release confidence without introducing a large architectural rewrite or a dependency on an external database service in routine test runs.

## Goals / Non-Goals

**Goals:**
- Make the main process follow a graceful shutdown path on normal container termination signals.
- Add an enforced CI workflow that runs the existing backend and frontend verification commands.
- Add meaningful automated coverage for wallet top-up settlement and auto-renew job behavior.

**Non-Goals:**
- Rework the overall payment architecture or repository layout.
- Add full end-to-end infrastructure provisioning for tests.
- Change business rules for plans, pricing, referrals, or backup flows.

## Decisions

### Listen for `SIGTERM` in the main process
The app will expand `signal.NotifyContext` to include `syscall.SIGTERM` in addition to `os.Interrupt`.

Why:
- This matches normal container and orchestrator shutdown behavior.
- It is the smallest safe change that activates the shutdown path already present after `b.Start(ctx)`.

Alternative considered:
- Wrapping the process in an external init/supervisor to translate signals.
  Rejected because the app should handle standard runtime signals itself.

### Add a single repository CI workflow as the release gate
The repo will add one GitHub Actions workflow that runs:
- `go test ./...`
- `go vet ./...`
- `go build ./cmd/app`
- `npm test`
- `npm run build`

Why:
- These are already the checks used for local release validation.
- A single workflow keeps the initial gate easy to reason about and maintain.

Alternative considered:
- Splitting backend and frontend into separate workflows immediately.
  Rejected for now because the goal is enforcement, not pipeline optimization.

### Add test seams in service packages instead of database-backed integration tests
`payment` and `autorenew` will gain small package-local interfaces or injectable collaborators so the critical behavior can be tested with focused fakes.

Why:
- The current service code depends directly on concrete repository types, which makes meaningful behavior tests difficult.
- Small interfaces let tests verify business behavior without introducing a live database requirement.
- This keeps the refactor local to the service packages rather than spreading mocks throughout the codebase.

Alternative considered:
- Adding PostgreSQL-backed integration tests.
  Rejected for this change because it would increase setup complexity and still leave CI/environment dependency questions to solve.

## Risks / Trade-offs

- [Risk] Small interface seams could broaden refactor scope unexpectedly.
  → Mitigation: keep interfaces package-local and include only methods already used by the tested code paths.
- [Risk] CI enforcement may surface unrelated flaky behavior.
  → Mitigation: use existing deterministic commands first; avoid adding nonessential jobs in this change.
- [Risk] Behavioral tests may still miss external Remnawave edge cases.
  → Mitigation: focus these tests on local business invariants and preserve room for later staged end-to-end validation.

## Migration Plan

1. Land the shutdown signal change and CI workflow.
2. Land the new backend behavioral tests and ensure they pass in the same workflow.
3. Use the existing verification commands locally and in CI before release.

Rollback:
- The CI workflow can be reverted independently if needed.
- The signal-handling change is isolated to process bootstrap.
- Test-seam refactors are internal-only and can be reverted without API migration.

## Open Questions

- Whether a later follow-up should add staged end-to-end billing validation in addition to these new unit/behavior tests.
