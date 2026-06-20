# Plan 006: Correct docs for the removed wallet auto-renew endpoint

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report - do not improvise. When done, update the status row for this plan in `plans/README.md` unless a reviewer dispatched you and told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 8e80b0b..HEAD -- readme.md HOWTOUSE.md internal/api/server.go internal/api/handlers.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: docs
- **Planned at**: commit `8e80b0b`, 2026-06-20

## Why This Matters

The README route table says `/api/wallet/autorenew` provides wallet-based auto-renew settings, but the handler always returns `410 Gone` and directs clients to per-key auto-renew. Stale API docs are worse than missing docs because they send future clients and agents down the wrong path.

## Current State

- `readme.md` - route table lists `/api/wallet/autorenew` as active.
- `internal/api/server.go` - still registers the legacy endpoint.
- `internal/api/handlers.go` - returns `410 Gone` for that endpoint.
- `/api/keys/autorenew` is the active per-key endpoint.

Current excerpts:

```markdown
<!-- readme.md:433 -->
| `/api/wallet` | Wallet summary |
| `/api/wallet/history` | Wallet transaction history |
| `/api/wallet/autorenew` | Wallet-based auto-renew settings |
| `/api/referrals` | Referral information |
| `/api/keys/autorenew` | Per-key auto-renew settings |
```

```go
// internal/api/server.go:91
// Wallet endpoints
mux.HandleFunc("/api/wallet", withAuth(handler.GetWallet))
mux.HandleFunc("/api/wallet/history", withAuth(handler.GetWalletHistory))
mux.HandleFunc("/api/wallet/autorenew", withAuth(handler.UpdateAutoRenew))
```

```go
// internal/api/handlers.go:2083
func (h *APIHandler) UpdateAutoRenew(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    http.Error(w, "Customer-wide auto-renew has been removed. Please enable auto-renew on a specific key from the home screen.", http.StatusGone)
}
```

Repo conventions:

- README route table is concise and operator-focused.
- Runtime keeps a legacy route to return a clear deprecation error instead of silent 404.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Doc grep | `rg -n "/api/wallet/autorenew|Wallet-based auto-renew" readme.md HOWTOUSE.md` | no misleading "active settings" wording remains |
| Go tests | `go test ./internal/api` | exit 0 |

## Scope

**In scope**:
- `readme.md`
- `HOWTOUSE.md` only if it repeats the stale endpoint wording
- Optionally add or update an API test that locks the `410 Gone` behavior

**Out of scope**:
- Do not remove the legacy route.
- Do not change `/api/keys/autorenew` behavior.
- Do not reintroduce customer-wide auto-renew.

## Git Workflow

- Branch: `codex/006-document-legacy-wallet-autorenew`
- Commit message: `docs: clarify legacy wallet autorenew endpoint`
- Do not push or open a PR unless instructed.

## Steps

### Step 1: Update the route table

In `readme.md`, change the `/api/wallet/autorenew` row to one of these clear forms:

```markdown
| `/api/wallet/autorenew` | Legacy endpoint; returns `410 Gone`. Use `/api/keys/autorenew`. |
```

or remove the row and add a short note below the table:

```markdown
Legacy `/api/wallet/autorenew` returns `410 Gone`; per-key auto-renew is handled by `/api/keys/autorenew`.
```

Keep `/api/keys/autorenew` listed as the active endpoint.

**Verify**: `rg -n "Wallet-based auto-renew settings" readme.md HOWTOUSE.md` -> no matches.

### Step 2: Check secondary docs

Search all Markdown docs for `/api/wallet/autorenew`. If other docs mention it as active, update them with the same legacy wording.

**Verify**: `rg -n "/api/wallet/autorenew" *.md docs openspec 2>/dev/null` -> every remaining match clearly says legacy, removed, or `410 Gone`.

### Step 3: Add a tiny regression test if missing

If no API test already asserts the legacy endpoint returns `http.StatusGone` for POST, add one in `internal/api` using `httptest` and direct `APIHandler.UpdateAutoRenew`.

Expected behavior:

- `GET` returns 405.
- `POST` returns 410.

**Verify**: `go test ./internal/api` -> exit 0.

## Test Plan

- Documentation grep proves stale wording is gone.
- Optional focused API test locks current deprecation behavior.

## Done Criteria

- [ ] README no longer describes `/api/wallet/autorenew` as active wallet-based auto-renew settings.
- [ ] Remaining docs either omit the endpoint or clearly mark it as legacy/410.
- [ ] `/api/keys/autorenew` remains documented as the active endpoint.
- [ ] `go test ./internal/api` exits 0 if a test was added.
- [ ] `plans/README.md` status row updated.

## STOP Conditions

Stop and report back if:

- Product requirements say customer-wide auto-renew should be restored instead of documented as removed.
- Docs reveal a second active client still using `/api/wallet/autorenew` successfully.

## Maintenance Notes

If the legacy route is removed in a future breaking API cleanup, update this documentation again and add a changelog note for clients pinned to the old endpoint.
