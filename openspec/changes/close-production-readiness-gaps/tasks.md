## 1. Runtime Safety

- [x] 1.1 Update the main process bootstrap to handle `SIGTERM` alongside interrupt-based shutdown.
- [x] 1.2 Add or update verification around graceful shutdown signal handling if a focused test seam is practical.

## 2. Release Gates

- [x] 2.1 Add a GitHub Actions workflow that runs backend test, vet, and build commands plus frontend test and build commands.
- [x] 2.2 Verify the workflow matches the current local release commands and does not depend on unrelated untracked files.

## 3. Revenue-Path Validation

- [x] 3.1 Add focused payment-service behavioral tests for wallet top-up settlement and atomic logging behavior.
- [x] 3.2 Add focused auto-renew job tests for successful renewal and insufficient-funds handling paths.

## 4. Verification

- [x] 4.1 Run backend and frontend verification commands after the runtime, CI, and test changes.
- [x] 4.2 Reconcile the task checklist and confirm the change is ready for final production-readiness review.
