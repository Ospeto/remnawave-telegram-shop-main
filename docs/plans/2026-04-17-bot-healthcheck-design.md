# Bot Healthcheck Design

Date: 2026-04-17
Project: Wavy_Best_Shop

## Goal

Add a bot-side synthetic healthcheck that an admin can trigger from the Telegram dashboard to verify the real production workflow end to end, not just process liveness.

## Problem

Current health endpoints only prove that the HTTP process, database connection, Remnawave ping path, and screenshot-provider readiness are up. They do not prove that the application can still fulfill a purchase through the real service layer.

That leaves a gap where:

- the app is "healthy" according to `/readyz`
- but purchase fulfillment or cleanup logic is broken
- and operators only learn that after a real customer is affected

## Requirements

The bot healthcheck must:

- be triggerable from the admin dashboard
- run a synthetic canary instead of a read-only probe
- validate database, Remnawave, and screenshot-provider readiness
- validate the real fulfillment workflow through the payment service
- avoid touching real customer balances or promo usage
- clean up the Remnawave user and local state it creates
- report step-by-step pass/fail output back to the admin in Telegram

## Chosen Approach

Use a dedicated `HealthcheckService` that runs two linked checks:

1. `Dependency canary`
   - Runs the existing analyzer readiness flow
   - Reports provider readiness and hard-fails if the screenshot-verification path is degraded

2. `Workflow canary`
   - Finds or creates a dedicated synthetic customer record
   - Creates a zero-amount synthetic purchase through the payment service
   - Forces fulfillment through the same `ProcessPurchaseById` path used in production
   - Verifies that a local subscription key row and Remnawave user are both created
   - Cleans the Remnawave user up, marks the local key deleted, and clears the customer’s active subscription fields

This approach gives a real end-to-end signal while keeping business impact near zero.

## Why This Approach

This is preferred over test mode because test mode bypasses strict verification and would hide real provider or fulfillment failures.

It is preferred over a read-only dashboard action because the request specifically requires a live workflow proof.

It is preferred over a full screenshot-upload bot canary because that would require synthetic image generation or a fragile fixture path and would still not prove much more than provider readiness already does.

## Data Model / Runtime Impact

The canary intentionally leaves a small audit trail:

- one synthetic customer row reused across runs
- zero-amount synthetic purchase records
- subscription keys marked `deleted` after cleanup

This is acceptable because:

- it does not affect revenue reporting
- it avoids destructive delete cascades in production data
- it preserves operator auditability

The live Remnawave user will be deleted after the check, so the remote system does not accumulate active test users.

## Admin UX

Add:

- admin dashboard button: `Run E2E Check`
- fallback admin command: `/healthbot run`

The response should be a single Telegram message with:

- overall verdict
- pass/fail per step
- duration
- any cleanup warnings

## Testing Strategy

Add focused unit tests for:

- dashboard rendering and command routing
- healthcheck service success path
- cleanup when Remnawave delete succeeds
- failure reporting when analyzer readiness is degraded
- failure reporting when workflow fulfillment does not create a subscription key

## Rollout Notes

This feature is operator-only and should be safe to deploy without schema changes.

The feature is intentionally conservative:

- one run at a time
- admin-only
- explicit synthetic labels in records
- cleanup best effort but visible in the report
