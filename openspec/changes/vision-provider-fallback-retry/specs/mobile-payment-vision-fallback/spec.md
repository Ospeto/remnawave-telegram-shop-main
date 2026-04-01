## ADDED Requirements

### Requirement: Provider-neutral screenshot analysis
The system SHALL analyze mobile payment screenshots through a provider-neutral analyzer interface so payment verification logic is not coupled to one LLM provider.

#### Scenario: Payment verification calls analyzer abstraction
- **WHEN** `VerifyMobilePayment` receives an image to validate
- **THEN** it uses the provider-neutral analyzer interface and receives one normalized `PaymentInfo` result or an analyzer error

### Requirement: Controlled fallback for provider failures
The system SHALL support Gemini as primary analyzer and OpenRouter as fallback when configured, and SHALL only fallback for retriable/provider failures.

#### Scenario: Primary timeout triggers fallback
- **WHEN** Gemini analysis fails with timeout/transport/rate-limit/server error
- **THEN** the analyzer performs bounded retry/failover according to configuration and attempts OpenRouter fallback

#### Scenario: Primary semantic negative does not trigger fallback
- **WHEN** Gemini returns a valid parsed result with `is_valid=false`
- **THEN** the analyzer returns that result and does not call fallback provider

### Requirement: Bounded retry and attempt budget
The system SHALL enforce a maximum number of analyzer attempts per verification request.

#### Scenario: Max attempts reached
- **WHEN** retries and fallback attempts reach configured maximum
- **THEN** analyzer returns failure without additional provider calls

### Requirement: Backward-compatible configuration
The system SHALL continue Gemini-only behavior when OpenRouter fallback is not configured.

#### Scenario: Gemini-only deployment
- **WHEN** mobile banking is enabled and OpenRouter key/model are absent
- **THEN** verification continues using Gemini-only analyzer flow

### Requirement: Operator-visible fallback metadata
The system SHALL log structured analyzer attempt metadata without exposing secrets or raw screenshot data.

#### Scenario: Fallback attempt is executed
- **WHEN** fallback is triggered for a verification request
- **THEN** logs include provider name, attempt number, error class, and outcome without logging API keys or raw image content
