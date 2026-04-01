## 1. Mini-App Recovery Fixes

- [x] 1.1 Update wallet top-up routing and checkout loading so top-up no longer depends on a negative plan index.
- [x] 1.2 Clear stale home-screen error state before retrying `/api/me` so successful retries recover normally.

## 2. Runtime Safety Fixes

- [x] 2.1 Replace the broken compose exec healthcheck with a process-safe probe against the running HTTP service.
- [x] 2.2 Change scheduled backup bookkeeping to mark the daily run complete only after successful local backup creation.
- [x] 2.3 Reject runtime restore execution from the live bot process and update operator-facing restore messaging/docs accordingly.

## 3. Verification

- [x] 3.1 Run backend and frontend verification commands for the change surface.
- [x] 3.2 Reconcile task checkboxes and confirm the change is ready for archive or follow-up review.
