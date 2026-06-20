# Plan 002: Return a minimal user DTO from `/api/me`

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report - do not improvise. When done, update the status row for this plan in `plans/README.md` unless a reviewer dispatched you and told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 8e80b0b..HEAD -- internal/api/handlers.go internal/api/admin_promos_test.go web-app/src/lib/types.ts web-app/src/pages`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `8e80b0b`, 2026-06-20

## Why This Matters

`/api/me` is the frontend's main authenticated bootstrap endpoint. It currently serializes `*database.Customer` directly, which exposes fields the frontend does not need and lets the database model define the public API shape. A small explicit DTO makes data minimization intentional and prevents future database fields from leaking into client responses.

## Current State

- `internal/api/handlers.go` - defines `ValidationResponse` and builds `/api/me` response.
- `internal/database/customer.go` - database model includes internal fields and has no JSON contract tags.
- `web-app/src/lib/types.ts` - frontend expects a tiny snake_case user object.
- `web-app/src/pages/*test.tsx` - frontend tests mock `/api/me` with only `id` and `telegram_id`.

Current excerpts:

```go
// internal/api/handlers.go:46
type ValidationResponse struct {
    User *database.Customer `json:"user"`
    Keys []KeyResponse `json:"keys"`
    IsActive bool `json:"is_active"`
    IsAdmin bool `json:"is_admin"`
    ...
}
```

```go
// internal/database/customer.go:25
type Customer struct {
    ID int64 `db:"id"`
    TelegramID int64 `db:"telegram_id"`
    ExpireAt *time.Time `db:"expire_at"`
    CreatedAt time.Time `db:"created_at"`
    SubscriptionLink *string `db:"subscription_link"`
    Language string `db:"language"`
    Balance float64 `db:"balance"`
    AutoRenew bool `db:"auto_renew"`
    ...
}
```

```go
// internal/api/handlers.go:1183
resp := ValidationResponse{
    User: customer,
    Keys: keys,
    IsActive: isActive,
    IsAdmin: h.isAdminUser(telegramID),
    ...
}
```

```ts
// web-app/src/lib/types.ts:33
export interface UserData {
    user: {
        id: number;
        telegram_id: number;
    };
    keys: SubscriptionKey[];
    ...
}
```

Repo conventions:

- Response structs live near the top of `internal/api/handlers.go` under `// --- Response types ---`.
- Existing API tests use direct JSON marshal assertions, e.g. `TestValidationResponseJSONIncludesIsAdmin` in `internal/api/admin_promos_test.go:19-28`.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| API tests | `go test ./internal/api` | exit 0 |
| Frontend tests | `cd web-app && npm test` | exit 0 |
| Full Go tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

**In scope**:
- `internal/api/handlers.go`
- `internal/api/admin_promos_test.go` or another existing `internal/api/*_test.go`
- `web-app/src/lib/types.ts` only if backend response intentionally changes beyond the existing frontend shape
- Frontend tests only if their mocks need to be aligned with the finalized DTO

**Out of scope**:
- Do not change `database.Customer` field names or database scan behavior.
- Do not change subscription key response shape.
- Do not add balance, subscription URL, language, or auto-renew fields to the new user DTO.

## Git Workflow

- Branch: `codex/002-api-me-minimal-user-dto`
- Commit message: `fix: return minimal user dto from api me`
- Do not push or open a PR unless instructed.

## Steps

### Step 1: Introduce an explicit API user DTO

In `internal/api/handlers.go`, add a response struct near `KeyResponse`:

```go
type UserResponse struct {
    ID         int64 `json:"id"`
    TelegramID int64 `json:"telegram_id"`
}
```

Add a small helper:

```go
func userResponse(customer *database.Customer) *UserResponse {
    if customer == nil {
        return nil
    }
    return &UserResponse{ID: customer.ID, TelegramID: customer.TelegramID}
}
```

Change `ValidationResponse.User` from `*database.Customer` to `*UserResponse`.

**Verify**: `go test ./internal/api` -> may fail until Step 2 is complete, but should not show unrelated compile errors.

### Step 2: Use the DTO in `/api/me`

In the `/api/me` response assembly, change:

```go
User: customer,
```

to:

```go
User: userResponse(customer),
```

Do not alter how `ExpireAt`, referral fields, keys, or admin status are computed.

**Verify**: `go test ./internal/api` -> exit 0.

### Step 3: Add JSON contract tests

Add a focused test next to `TestValidationResponseJSONIncludesIsAdmin` that marshals a `ValidationResponse` with `UserResponse{ID: 1, TelegramID: 42}` and asserts:

- The JSON includes `"user":{"id":1,"telegram_id":42}` or equivalent unmarshaled values.
- The JSON does not include `subscription_link`, `balance`, `auto_renew`, `language`, or `created_at`.

Use `json.Unmarshal` into `map[string]any` for robust checks instead of brittle full-string comparisons.

**Verify**: `go test ./internal/api` -> exit 0 and the new test fails if raw `database.Customer` is restored.

### Step 4: Confirm frontend contract still matches

The frontend already expects:

```ts
user: {
    id: number;
    telegram_id: number;
}
```

Run frontend tests. If any mock expected extra fields from the old backend behavior, update that mock to the minimal DTO instead of expanding `UserData`.

**Verify**: `cd web-app && npm test` -> exit 0.

## Test Plan

- New backend JSON contract test for `ValidationResponse.User`.
- Existing frontend `/api/me` tests should remain green because they already use the intended minimal shape.
- Full verification: `go test ./...`, `go vet ./...`, `cd web-app && npm test`.

## Done Criteria

- [ ] `ValidationResponse.User` is not `*database.Customer`.
- [ ] `/api/me` returns only `id` and `telegram_id` inside `user`.
- [ ] Backend test proves internal customer fields are absent from the JSON response.
- [ ] `go test ./...` exits 0.
- [ ] `go vet ./...` exits 0.
- [ ] `cd web-app && npm test` exits 0.
- [ ] `plans/README.md` status row updated.

## STOP Conditions

Stop and report back if:

- A real frontend screen depends on leaked `database.Customer` fields instead of separate documented API fields.
- The live `ValidationResponse` no longer resembles the excerpt.
- The change appears to require database model edits or migration work.

## Maintenance Notes

Future `/api/me` additions should be explicit fields on `ValidationResponse` or small DTOs, not a raw persistence model. Reviewers should reject future direct serialization of database structs on public API responses unless the struct is explicitly designed as an API contract.
