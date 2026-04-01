## Why

The current branch compiles and the mini-app now has regression tests, but production signoff is still blocked by deploy-time shutdown risk and weak verification on money-moving backend paths. These gaps need to be closed now because they directly affect release safety and confidence.

## What Changes

- Add graceful process-lifecycle handling so the app follows a clean shutdown path on normal container stop signals.
- Add enforced repository-level release checks so backend and frontend verification stop being manual-only.
- Add automated backend coverage for critical revenue-path behaviors, specifically wallet top-up settlement and auto-renew job behavior.

## Capabilities

### New Capabilities
- `runtime-process-lifecycle`: Defines how the running service must react to termination signals and shutdown in containerized production.
- `release-validation-gates`: Defines the automated verification gates required before production-ready changes can be merged or released.

### Modified Capabilities
- None.

## Impact

- Affected code: `/cmd/app`, `/internal/payment`, `/internal/service/autorenew`, `/web-app`, and repository CI configuration under `/.github/workflows`.
- Affected systems: container runtime behavior, merge/release verification, and backend financial-path test coverage.
- Dependencies: GitHub Actions workflow support and existing Go/Node test commands.
