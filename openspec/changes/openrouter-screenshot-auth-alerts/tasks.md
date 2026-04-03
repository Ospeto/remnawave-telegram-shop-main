## 1. OpenRouter Auth Failure Detection And Alerting

- [x] 1.1 Detect OpenRouter auth failures from analyzer errors and map them to a dedicated verification outcome.
- [x] 1.2 Add deduplicated Telegram admin alerts for OpenRouter auth failures with a fixed cooldown.
- [x] 1.3 Add unit tests for OpenRouter auth error classification, alert text shaping, and cooldown behavior.

## 2. User-Facing Failure Messaging

- [x] 2.1 Add a dedicated translation key for the Telegram bot screenshot-failure response when verification is temporarily unavailable.
- [x] 2.2 Return the new temporary-unavailable message from screenshot verification without weakening existing business validation paths.
- [x] 2.3 Verify the web upload path and Telegram photo-upload path both surface the new failure reason correctly.
