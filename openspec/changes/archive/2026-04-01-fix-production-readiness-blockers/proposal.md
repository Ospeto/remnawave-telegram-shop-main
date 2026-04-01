## Why

The current production-readiness review uncovered release-blocking failures in both the Telegram mini-app and the runtime deployment path. These issues can block wallet funding, leave users stuck after transient API failures, keep containers permanently unhealthy, and make backup/restore behavior unsafe under real production conditions.

## What Changes

- Fix the mini-app wallet top-up flow so users can complete top-ups without relying on an invalid plan index.
- Make the mini-app home screen retry path recover cleanly after transient `/api/me` failures.
- Replace the broken container probe path with a healthcheck strategy that does not boot a second full app process.
- Change scheduled backup bookkeeping so a failed run is retried instead of being marked complete for the day.
- Guard restore behavior so destructive restore operations are not allowed in unsafe live-production conditions.

## Capabilities

### New Capabilities
- `mini-app-recovery-flows`: Defines reliable mini-app behavior for wallet top-up entry and retry recovery after transient load failures.
- `runtime-operational-safety`: Defines safe runtime behavior for healthchecks, backup scheduling, and restore gating in production deployments.

### Modified Capabilities

- None.

## Impact

- Affected frontend code in `web-app/src/pages/Home.tsx`, `web-app/src/pages/Plans.tsx`, `web-app/src/pages/Checkout.tsx`, and routing/bootstrap files as needed.
- Affected deployment/runtime code in `docker-compose.yaml`, backup service logic, and application startup/health behavior.
- Changes operational behavior for backup retry handling and restore enablement expectations.
