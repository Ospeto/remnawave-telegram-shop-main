# Bot Healthcheck Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an admin-triggered synthetic healthcheck canary that proves the production fulfillment workflow works end to end.

**Architecture:** Introduce a dedicated healthcheck service that runs dependency readiness plus a synthetic zero-amount purchase fulfillment canary, then wire it into the admin dashboard and fallback commands. Cleanup happens by deleting the temporary Remnawave user and marking local synthetic state inactive instead of deleting audit records.

**Tech Stack:** Go, Telegram bot handlers, PostgreSQL repositories, Remnawave API client, existing payment service.

---

### Task 1: Add the failing tests for the synthetic healthcheck service

**Files:**
- Create: `internal/service/healthcheck/service_test.go`
- Modify: `internal/handler/admin_quick_actions_test.go`

**Step 1: Write the failing tests**

- Add a service test for a successful canary run.
- Add a service test for analyzer readiness failure.
- Add a handler/dashboard test that expects a new `Run E2E Check` action and `/healthbot run` mapping.

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/service/healthcheck ./internal/handler -run 'Test.*Health' -count=1
```

**Step 3: Implement the minimal production code to satisfy the tests**

- Add the new service package and placeholder handler/dashboard hooks.

**Step 4: Run tests to verify they pass**

Run the same command again.

### Task 2: Add Remnawave cleanup support and payment-service canary creation

**Files:**
- Modify: `internal/remnawave/client.go`
- Modify: `internal/payment/payment.go`
- Modify: `internal/payment/payment_test.go`

**Step 1: Write the failing tests**

- Add a payment-service test for synthetic canary purchase metadata/behavior.
- Add a remnawave client test around the new delete path if practical at unit level.

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/payment ./internal/remnawave -run 'Test.*Canary|Test.*Delete' -count=1
```

**Step 3: Write minimal implementation**

- Add a payment-service helper that creates a zero-amount synthetic purchase and fulfills it.
- Add a Remnawave client method to delete a user by UUID.

**Step 4: Run tests to verify they pass**

Run the same command again.

### Task 3: Implement the healthcheck service workflow and cleanup

**Files:**
- Create: `internal/service/healthcheck/service.go`
- Modify: `internal/database/subscription_key.go`

**Step 1: Write the failing tests**

- Expand the service tests to assert:
  - synthetic customer creation/reuse
  - key existence after fulfillment
  - Remnawave delete + local cleanup
  - cleanup warnings if delete fails

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/service/healthcheck -count=1
```

**Step 3: Write minimal implementation**

- Add the service report model.
- Implement one-at-a-time execution.
- Implement synthetic customer resolution, workflow canary, and cleanup.
- Add repository support to mark or look up keys as needed.

**Step 4: Run tests to verify they pass**

Run the same command again.

### Task 4: Wire the healthcheck into the admin bot UX

**Files:**
- Modify: `internal/handler/handler.go`
- Modify: `internal/handler/admin_dashboard.go`
- Modify: `internal/handler/admin_registry.go`
- Modify: `cmd/app/main.go`
- Create: `internal/handler/healthcheck_commands.go`

**Step 1: Write the failing tests**

- Add handler tests for:
  - admin dashboard button presence
  - fallback command routing
  - healthcheck command output formatting on success/failure

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/handler -run 'Test.*Health' -count=1
```

**Step 3: Write minimal implementation**

- Inject the healthcheck service into the handler.
- Add `Run E2E Check` button.
- Add `/healthbot run` admin command.
- Format the report for Telegram.

**Step 4: Run tests to verify they pass**

Run the same command again.

### Task 5: Update docs and verify the full feature

**Files:**
- Modify: `readme.md`
- Modify: `HOWTOUSE.md`

**Step 1: Add documentation**

- Document the new admin action and fallback command.
- Explain that this is a synthetic workflow canary, not just a liveness check.

**Step 2: Run verification**

Run:

```bash
gofmt -w internal/remnawave/client.go internal/payment/payment.go internal/payment/payment_test.go internal/database/subscription_key.go internal/service/healthcheck/service.go internal/service/healthcheck/service_test.go internal/handler/handler.go internal/handler/admin_dashboard.go internal/handler/admin_registry.go internal/handler/healthcheck_commands.go cmd/app/main.go
go test ./internal/service/healthcheck ./internal/handler ./internal/payment ./internal/remnawave -count=1
go test ./... -count=1
go vet ./...
```

**Step 3: Review the diff**

- Run the review skill on the completed changes and resolve any significant findings before commit.

**Step 4: Commit**

```bash
git add docs/plans/2026-04-17-bot-healthcheck-design.md docs/plans/2026-04-17-bot-healthcheck.md internal/remnawave/client.go internal/payment/payment.go internal/payment/payment_test.go internal/database/subscription_key.go internal/service/healthcheck/service.go internal/service/healthcheck/service_test.go internal/handler/handler.go internal/handler/admin_dashboard.go internal/handler/admin_registry.go internal/handler/healthcheck_commands.go cmd/app/main.go readme.md HOWTOUSE.md
git commit -m "Add synthetic bot healthcheck canary"
```
