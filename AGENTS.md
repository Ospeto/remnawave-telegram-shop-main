# AGENTS.md — Wavy Best Shop / remnawave-tg-shop-bot

Repo-specific guidance for OpenCode sessions. Prefer this over generic Go/React advice.

## Architecture

- **Single Go module**: `remnawave-tg-shop-bot` (Go **1.25.3**). Entrypoint: `cmd/app`.
- **Frontend**: React/Vite Mini App isolated under `web-app/` (Node **20**). Entrypoint: `web-app/src/main.tsx`.
- API serves the SPA from **`./web-app/dist`** (relative to process CWD). Build frontend before local backend SPA serving works.
- **Postgres 17** (docker-compose). Migrations auto-run on startup from `db/migrations`.
- Caddy is an **optional** edge profile (`Caddyfile`); Mini App URL must be **public HTTPS**.

### Internal packages (short map)

| Package | Role |
|---------|------|
| `internal/handler` | Telegram bot handlers / admin |
| `internal/payment` | Purchases, verification, test-mode bypass |
| `internal/wallet` | Internal balance top-up / spend |
| `internal/api` | HTTP API + SPA static serve |
| `internal/database` | DB pool, migrations |
| `internal/config` | Env loading (`.env` unless disabled) |
| `internal/remnawave` | Remnawave panel client |
| `internal/service/*` | healthcheck, backup, autorenew, etc. |
| `internal/gemini` | Vision / screenshot verification |
| `internal/promo`, `notification`, `sync`, `reporting`, `translation`, `cache` | Supporting domains |

## Verification commands (CI truth)

Backend (repo root):

```bash
go test ./...
go vet ./...
go build ./cmd/app
```

Frontend (`web-app/`):

```bash
npm ci
npm test          # vitest run
npm run build     # tsc && vite build
```

CI (`.github/workflows/ci.yml`): install frontend deps → Go test/vet/build → frontend test/build. **No separate lint/formatter/pre-commit** found.

### Focused tests

```bash
# One Go package
go test ./internal/payment/ -count=1

# One Go test by name
go test ./internal/payment/ -run TestGetTestTransactionID -count=1

# One frontend file
cd web-app && npm test -- src/pages/Home.test.tsx
```

## Local run vs Docker

| Mode | Notes |
|------|--------|
| **Docker** | Primary deploy path (`docker-compose`, `setup.sh`). Sets `DISABLE_ENV_FILE=true` — env comes from compose, not `.env` file load. |
| **Local backend** | Needs `.env` from `.env.sample`. CWD must see `./web-app/dist` and `./translations`. Migrations path `./db/migrations`. |
| **Frontend dev** | `cd web-app && npm run dev` (Vite only). Production Mini App is the built `dist` served by Go. |

Health endpoints: `/livez` (liveness), `/readyz` (DB + Remnawave + vision readiness), `/healthcheck` (same readiness as JSON). Docker healthcheck hits `/readyz`.

## Env / runtime gotchas

- Start from **`.env.sample`**; do not invent keys.
- **`DISABLE_ENV_FILE=true`**: skips godotenv (Docker). Local runs should leave this unset/false.
- **`MINI_APP_URL`**: public HTTPS origin; empty effectively disables Mini App link. Blank Mini App screen → SSL / mixed content / URL mismatch (see `HOWTOUSE.md`).
- Caddy optional; if used, `DOMAIN_NAME` must match Mini App host.
- Admin primary UI: `/admin`. Ops docs: `HOWTOUSE.md`, backups: `BACKUP-RUNBOOK.md`.

## Testing / ops gotchas

- **Test mode magic txn ID**: `01004063070995016447` (only when test mode enabled via admin/`/test`). Defined in `internal/payment` as unexported `testTransactionID`.
- **Dirty migration recovery**: stop bot, clear dirty flag / set version, restart — procedure in `HOWTOUSE.md` (“Dirty Database Version”).
- Synthetic E2E: `/healthbot run` or admin Operations → Run E2E Check.
- Duplicate txn errors when reusing receipts outside test mode — use test mode for safe bypass.

## Hard constraints (do not casually reverse)

1. **Crypto Pay is currently disabled** in this runtime — do not re-enable without explicit product decision and full payment-path review.
2. **Live restore is disabled** in the running bot — inspect/list only; restore offline via `setup.sh` / runbook. Do not “fix” runtime restore for convenience.
3. **Money-safety paths** (`internal/payment`, `internal/wallet`, purchase/fulfillment state): preserve **idempotency**, transaction-ID uniqueness, and wallet balance invariants. Prefer additive tests over speculative refactors.

## Production deploy (VPS)

Hands-off deploy skill: `~/.hermes/skills/wavy-vps-deploy/SKILL.md`  
Script: `~/.hermes/skills/wavy-vps-deploy/scripts/deploy_latest.sh`

| Fact | Value |
|------|--------|
| Local repo | `/Users/macbookair/coding_projects/Wavy_Best_Shop` |
| VPS | `root@92.112.127.10` |
| App path | `/opt/remnawave-shop` |
| Public Mini App | `https://shop.wavypremium.xyz/` |
| Rebuild service | **`bot` only** — never recreate `db` for normal deploys |

### Deploy rules

1. Deploy **`origin/main` only** unless the operator explicitly names another commit.
2. Local preflight: `git fetch origin main`, `HEAD == origin/main`, no tracked dirty files, `go test ./...`.
3. VPS must also be clean on tracked files before reset to `origin/main`.
4. Never print/copy SSH keys, `.env`, `TELEGRAM_TOKEN`, or other secrets.
5. Preserve payment/wallet safety — no manual DB edits during deploy.
6. After deploy, verify: VPS commit before/after, `docker compose ps`, `http://127.0.0.1:8080/readyz`, public Mini App HTTP, `/api/plans` smoke, Telegram `getMe` + `getWebhookInfo` **from the VPS**, recent bot logs for getUpdates/error/panic/fatal.
7. If Telegram API fails but Mini App/`readyz` are OK, report that clearly (commands may be broken while Mini App still works). Known quirk: this host has had intermittent Telegram egress timeouts.

### Finance reporting (shipped)

- Admin Mini App: `/admin/finance` (admin-only Home card).
- Shared assembly: `internal/reporting.FinanceService.GetReport` (API JSON/CSV, Telegram `/revenue`, cron).
- Refund ledger: migration `000033_financial_adjustment`; admin POST adjustment is ledger-only (not purchase/wallet mutation).
- Timezone: Asia/Yangon; weeks Mon–Sun; Net Income = gross service revenue − refunds on effective date.

## Docs worth opening first

- `readme.md` — setup, env, architecture overview
- `HOWTOUSE.md` — admin ops, test mode, dirty migrations, Mini App troubleshooting, structured finance reporting
- `BACKUP-RUNBOOK.md` — backup/restore safety
- `docs/MINI_APP.md` — Mini App build/dev details + Admin Finance
- `docs/plans/2026-07-12-financial-reporting-design.md` — finance reporting design
- `.env.sample` — canonical env surface
