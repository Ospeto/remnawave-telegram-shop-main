## ADDED Requirements

### Requirement: Wallet top-up checkout path
The mini-app SHALL allow wallet top-up purchases without depending on a valid subscription plan index.

#### Scenario: User starts wallet top-up from wallet screen
- **WHEN** a user opens the wallet screen and selects top-up
- **THEN** the mini-app routes the user through plans and checkout using top-up-specific state rather than an invalid negative `plan_index`

#### Scenario: Wallet top-up checkout loads successfully
- **WHEN** checkout is opened for a wallet top-up flow
- **THEN** the page loads the top-up amount and purchase actions without showing an invalid-plan error

### Requirement: Recoverable home reload after transient API failure
The mini-app home screen SHALL recover from a transient `/api/me` load failure without requiring a full page reload.

#### Scenario: Retry after failed initial load
- **WHEN** the initial `/api/me` request fails and the user presses retry
- **THEN** the mini-app clears stale error state before reloading data

#### Scenario: Successful retry shows recovered content
- **WHEN** a retry request succeeds after an earlier load error
- **THEN** the home screen renders the refreshed user data instead of remaining on the previous error view
