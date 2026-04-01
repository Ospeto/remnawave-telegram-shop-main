## ADDED Requirements

### Requirement: Repository enforces release verification commands
The repository SHALL provide an automated CI gate that runs the standard backend and frontend verification commands for production-ready changes.

#### Scenario: Change is validated in CI
- **WHEN** repository CI runs for the default release workflow
- **THEN** it executes backend test, vet, and build commands plus frontend test and build commands

### Requirement: Critical revenue paths have automated behavioral coverage
The codebase SHALL include automated tests for critical revenue-path behaviors that move money or service state.

#### Scenario: Wallet top-up settlement is validated
- **WHEN** automated tests run for the payment service
- **THEN** they verify wallet top-up settlement updates purchase state, balance effects, and wallet transaction logging behavior

#### Scenario: Auto-renew job behavior is validated
- **WHEN** automated tests run for the auto-renew job
- **THEN** they verify renewal and insufficient-funds paths without relying on manual release-only validation
