## Context

The current mobile payment verification flow calls Gemini directly from `VerifyMobilePayment`. Any Gemini API failure returns a generic verification failure and stops the flow. This creates operational fragility around API key rotation, temporary provider outages, and rate limits.

Payment verification is fraud-sensitive: fallback must increase reliability without weakening business correctness checks (recipient, amount, duplicate transaction ID, forbidden note rules).

## Goals / Non-Goals

**Goals:**
- Introduce a provider-neutral screenshot analyzer interface.
- Keep a single prompt and output contract across providers.
- Add bounded retry/failover for provider failures only.
- Support OpenRouter fallback with configurable key/model.
- Preserve current fail-closed business validation behavior.

**Non-Goals:**
- Rewriting purchase validation rules.
- Auto-approving payments when both providers fail.
- Broad multi-provider model routing strategy at launch.

## Decisions

1. Add an orchestrator in `internal/gemini` that can call multiple provider adapters (`gemini`, `openrouter`) using one prompt/parser contract.
Rationale: keeps `PaymentService` unchanged at the business-rule boundary and isolates inference reliability concerns.

2. Classify provider errors before retry/failover.
Rationale: fallback is allowed only for provider failures (transport/timeouts/auth/rate-limit/5xx/malformed response), not semantic negatives (`is_valid=false`, amount mismatch, recipient mismatch).

3. Keep attempts bounded to preserve latency.
Rationale: verification happens in an interactive Telegram flow with a finite request timeout; unlimited retries increase cost and unpredictability.

4. Make fallback optional and backward compatible.
Rationale: existing Gemini-only deployments should keep working without OpenRouter configuration.

## Risks / Trade-offs

- [Provider output inconsistency] -> Enforce one JSON contract and centralized parser logic.
- [Fraud posture drift from model-shopping] -> Disallow fallback after semantic negative results.
- [Latency growth] -> Bound attempts and apply per-request timeout budget.
- [Operational opacity] -> Add structured logs with provider/attempt/error class metadata.
- [Config mistakes] -> Keep explicit env keys with safe defaults and clear setup prompts.

## Migration Plan

1. Add provider-neutral analyzer + fallback config paths behind optional OpenRouter env values.
2. Wire `PaymentService` to new analyzer and keep current business validation logic.
3. Update setup/docs/env templates for OpenRouter + retry settings.
4. Run targeted tests.
5. Deploy with Gemini primary and OpenRouter fallback enabled only when configured.

Rollback:
- Disable fallback by clearing `OPENROUTER_API_KEY` or setting `VISION_PROVIDER_FALLBACK` empty.
- Keep Gemini-only behavior unchanged.

## Open Questions

- Whether to expose advanced OpenRouter provider routing options in first release or keep a single pinned model only.
- Whether health endpoint should include detailed per-provider status or high-level fallback readiness only.
