# Project Agent Memory — Wavy Best Shop (remnawave-tg-shop-bot)

This file is the project's committed home for project-intrinsic agent knowledge: architecture, build, test, safety rules, and production deployment operations for Wavy Best Shop.

---

## 1. Architecture & Subsystems

- **Go Backend Core**: Single Go module `remnawave-tg-shop-bot` (Go 1.25+ / 1.27+). Main entrypoint: `cmd/app/main.go`.
- **Telegram Mini App (Frontend)**: Isolated React/TypeScript Vite application in `web-app/` (Node 20+). Entrypoint: `web-app/src/main.tsx`.
- **Static Asset Serving**: In production, the Go HTTP server serves the pre-compiled Mini App SPA directly from `./web-app/dist` (relative to process working directory). Frontend must be built before local static serving works.
- **Database Layer**: PostgreSQL 17 managed via Docker Compose (`db`). Schema migrations reside in `db/migrations` and auto-run sequentially on backend startup.
- **Edge Reverse Proxy**: Caddy server (`Caddyfile`) handles TLS termination and proxies public HTTPS traffic to the bot API (`:8080`) and Mini App.
- **Remnawave Panel Integration**: Outbound API client (`internal/remnawave`) synchronizes users, subscription plans, inbound server nodes, traffic usage quotas, and expiration dates with the Remnawave management panel (`https://panel.wavypremium.xyz`).
- **Vision OCR Verification**: `internal/gemini` handles payment receipt screenshot analysis via Gemini Vision API with automatic OpenRouter failover for slip OCR parsing and validation.

### Internal Package Map

| Package | Responsibility |
| :--- | :--- |
| `cmd/app` | Main application lifecycle, daemon initialization, shutdown signals |
| `internal/handler` | Telegram bot command handlers, inline keyboards, `/admin` control menu, user callbacks |
| `internal/payment` | Subscription purchases, payment slip processing, idempotency checks, test mode |
| `internal/wallet` | User wallet balance top-ups, deductions, referral credits, balance invariants |
| `internal/api` | REST API routes, Telegram WebApp authentication, static SPA serving, healthchecks |
| `internal/database` | PostgreSQL connection pooling, migration runner, transaction helpers |
| `internal/config` | Configuration loading from `.env` and environment variables |
| `internal/remnawave` | Remnawave panel API client (user provisioning, traffic limits, node lists) |
| `internal/gemini` | Payment slip screenshot OCR extraction & verification with OpenRouter failover |
| `internal/reporting` | Financial revenue calculations, CSV exports, `/revenue` command, admin finance card |
| `internal/service/*` | Background workers (healthcheck, automated backup scheduler, auto-renewal, notifications) |
| `internal/promo` | Promotional code redemption and tracking |

---

## 2. Verification & Testing Commands (CI Truth)

Always execute verification commands before declaring changes complete or opening pull requests:

### Backend Verification (Repo Root)
```bash
# Run all unit and integration tests
go test ./...

# Static analysis and vetting
go vet ./...

# Binary build verification
go build ./cmd/app
```

### Focused Backend Tests
```bash
# Payment logic tests
go test ./internal/payment/ -count=1 -v

# Wallet balance and transaction tests
go test ./internal/wallet/ -count=1 -v

# API endpoint and authentication tests
go test ./internal/api/ -count=1 -v

# Test mode magic transaction ID validation
go test ./internal/payment/ -run TestGetTestTransactionID -count=1
```

### Frontend Verification (`web-app/`)
```bash
cd web-app

# Clean dependency installation
npm ci

# Run Vitest test suites
npm test

# Production bundle compilation and TypeScript typecheck
npm run build
```

---

## 3. Hard Safety Invariants & Rules

1. **Money & Wallet Safety**:
   - Preserve absolute idempotency and uniqueness on all transaction IDs (`transaction_id`).
   - Wallet top-ups, payment approvals, and referral rewards must execute within atomic database transactions with row-level locks. Never mutate balances without an audit ledger entry.
2. **Crypto Pay Status**:
   - Crypto Pay is currently disabled in this runtime. Do not re-enable or expose crypto payment routes without an explicit product directive.
3. **Database Restore Safety**:
   - Live database restoration inside the running bot is disabled to prevent split-brain state and corruption. Backups can be listed and inspected via `/admin`, but restores must be performed offline using `setup.sh` per `BACKUP-RUNBOOK.md`.
4. **Test Mode Protocol**:
   - Magic Test Transaction ID: `01004063070995016447`.
   - Test mode bypasses payment OCR strictly when enabled by an admin via `/admin` → Operations → Enable Test Mode (or `/test enable`). Disable immediately after testing.
5. **No Blind Env Creation**:
   - Always reference `.env.sample` as the canonical source of environment variables. Never invent unconfigured configuration keys.

---

## 4. Production VPS Deployment & Topology

Hands-off deploy script: `~/.hermes/skills/wavy-vps-deploy/scripts/deploy_latest.sh`

| Parameter | Production Value |
| :--- | :--- |
| **Local Workspace** | `/Users/macbookair/coding_projects/Wavy_Best_Shop` |
| **Production VPS Host** | `root@143.20.154.244` (or SSH alias `nl`) |
| **Remote Application Path** | `/opt/remnawave-shop` |
| **Public Mini App URL** | `https://shop.wavypremium.xyz/` |
| **Rebuild Container Target** | **`bot` service only** — never recreate or drop `db` during standard deploys |
| **Health Check Endpoints** | `/livez` (liveness), `/readyz` (readiness), `/healthcheck` (JSON status) |

### Deployment Execution Rules

1. **Deploy `origin/main` Only**:
   - Ensure local branch is clean and fully pushed to `origin/main`.
   - Local preflight: `git fetch origin main && git diff --quiet origin/main && go test ./...`.
2. **Remote Safe Pull & Rebuild**:
   - On VPS: pull latest `origin/main` cleanly without dirty tracked files.
   - Rebuild and restart the bot container:
     ```bash
     docker compose build bot && docker compose up -d bot
     ```
3. **Post-Deploy Health Verification**:
   - Container Status: `docker compose ps` (verify `bot` is `Up` and healthy).
   - Internal Readiness: `curl -s -f http://127.0.0.1:8080/readyz || echo "UNREADY"`.
   - Public Mini App: `curl -s -o /dev/null -w "%{http_code}\n" https://shop.wavypremium.xyz/`.
   - Plans Endpoint: `curl -s http://127.0.0.1:8080/api/plans`.
   - Telegram Polling: Inspect `docker compose logs --tail=40 bot` for clean polling starts with zero panic or fatal errors.

---

## 5. Operations & Documentation Pointers

- **`readme.md`**: Project overview, environment setup, initial installation guide.
- **`HOWTOUSE.md`**: Day-to-day admin operations, plan management syntax, test mode workflows, dirty migration recovery.
- **`BACKUP-RUNBOOK.md`**: Automated backup schedules, manual snapshot generation, offline disaster recovery.
- **`docs/MINI_APP.md`**: Telegram Mini App design tokens, theme integration, routing, and Admin Finance cards.
- **`.env.sample`**: Complete reference for all database, Telegram, Remnawave, and payment provider keys.
