# Remnawave Telegram Shop Bot

A self-hosted Telegram shop for selling Remnawave-backed digital subscriptions through a Telegram bot and Mini App.

It combines:

- a Go backend for bot logic, payments, delivery, and operations
- a React Mini App for plan browsing and checkout
- PostgreSQL for state and transaction history
- an optional bundled Caddy profile for HTTPS termination

This repository is intended for operators who want a production-capable Telegram sales flow without depending on a hosted storefront.

## What It Does

- sells subscription plans defined in environment configuration
- provisions or extends access through the Remnawave API
- supports multiple payment paths:
  - Crypto Pay
  - mobile banking with screenshot verification
  - internal wallet top-ups and wallet purchases
- supports promo codes, referral rewards, trials, and auto-renew toggles
- exposes a Telegram Mini App UI for customer self-service
- includes scheduled database backups and a Telegram admin dashboard for operator work

## Production Notes

Recent hardening in this codebase includes:

- idempotent purchase creation for API and Telegram callback flows
- atomic wallet charging and wallet ledger writes
- purchase processing state before final `paid` transition
- promo usage reservation at purchase creation to prevent oversell
- screenshot verification throttling for repeated uploads
- panic recovery and timeout guards around scheduled jobs
- HTTP server read, write, and idle timeouts
- stricter replay protection for Telegram Mini App auth
- safer forwarded-header trust defaults

If you are deploying for real users, read the `Configuration`, `Payments`, `Backups`, and `Operations` sections carefully.

## Admin Operations

The primary admin entrypoint is `/admin`.

The dashboard is organized into:

- `Overview`: revenue, transactions, provider status, backup health
- `Payments`: provider settings, referral bonus, receiver status
- `Backups`: run backups, inspect status, manage schedule, restore guidance
- `Operations`: sync, notifications, test mode, fallback commands

Most older slash commands still exist as hidden emergency fallbacks, but they are no longer the main operator UX.

## Architecture

### Runtime Components

- `bot`: Go application
  - runs Telegram long polling
  - serves the Mini App build and JSON API
  - runs DB migrations on startup
  - executes scheduled jobs for invoices, auto-renew, reports, backups, and subscription checks
- `db`: PostgreSQL
- `caddy`: optional reverse proxy with automatic HTTPS when you enable the `edge` profile

### Request Flow

1. A user opens the bot or Mini App from Telegram.
2. The Mini App calls the backend API under the same origin.
3. The backend authenticates Mini App requests using Telegram `initData`.
4. Purchases are created and processed through payment services.
5. Subscription creation or extension is executed through Remnawave.
6. Purchase state, wallet entries, referrals, and audit data are stored in PostgreSQL.

### Repository Layout

```text
.
├── cmd/
│   ├── app/                  # Main application entrypoint
│   └── reset_db/             # DB reset utility
├── db/
│   └── migrations/           # SQL migrations
├── internal/
│   ├── api/                  # HTTP API and Mini App auth
│   ├── cache/                # Runtime caches
│   ├── config/               # Environment loading and validation
│   ├── cryptopay/            # Crypto Pay client logic
│   ├── database/             # Repositories and DB helpers
│   ├── gemini/               # Screenshot-analysis providers and analyzer
│   ├── handler/              # Telegram command, dashboard, and callback handlers
│   ├── notification/         # Subscription notifications
│   ├── payment/              # Purchase creation and fulfillment logic
│   ├── remnawave/            # Remnawave API integration
│   └── service/              # Backup, auto-renew, invoice checker, wallet, etc.
├── translations/             # Localization files mounted into the container
├── web-app/                  # React Mini App
├── Caddyfile
├── Dockerfile
├── docker-compose.yaml
├── setup.sh                  # Interactive setup and maintenance wizard
└── .env.sample
```

## Stack

- Go `1.25.3`
- Node.js `20` for the frontend build stage
- React `18`
- TypeScript
- Vite
- PostgreSQL `17` in the default compose stack
- Caddy `2`

## Prerequisites

You need:

- Docker and Docker Compose plugin, or legacy `docker-compose`
- a Telegram bot token from [@BotFather](https://t.me/BotFather)
- your Telegram numeric ID for admin access
- a Remnawave panel URL and API token
- a public domain name if you want the Mini App exposed over HTTPS

Optional, depending on the payment features you enable:

- Crypto Pay token
- OpenRouter API key
- Gemini API key

## Quick Start

### Recommended: Setup Wizard

The repository includes an interactive installer and maintenance script.

```bash
git clone https://github.com/Ospeto/remnawave-telegram-shop-main.git
cd remnawave-telegram-shop-main
chmod +x setup.sh
./setup.sh
```

Use `Fresh Install` from the menu and fill in the required values.

The wizard can:

- create or update `.env`
- detect Docker Compose style
- build and start the stack
- edit payment and backup settings later
- create full-system backups
- restore from a previously created backup bundle

### Manual Docker Deployment

1. Copy the sample environment.

```bash
cp .env.sample .env
```

2. Edit `.env` with your real values.

At minimum, configure:

- `TELEGRAM_TOKEN`
- `ADMIN_TELEGRAM_ID`
- `REMNAWAVE_URL`
- `REMNAWAVE_TOKEN`
- `PLANS`
- `MINI_APP_URL`

If you want the repository to manage HTTPS for you with bundled Caddy, also set:

- `DOMAIN_NAME`
- `ACME_EMAIL`

3. Start the application stack.

```bash
docker compose up -d --build bot db
```

If you use legacy Compose:

```bash
docker-compose up -d --build bot db
```

4. Optional: start the bundled HTTPS proxy.

```bash
docker compose up -d --build caddy
```

If you use legacy Compose:

```bash
docker-compose up -d --build caddy
```

5. Follow logs if needed.

```bash
docker compose logs -f bot
docker compose logs -f db
```

If you started bundled Caddy:

```bash
docker compose logs -f caddy
```

## Telegram Setup

### Create and Configure the Bot

1. Create a bot with [@BotFather](https://t.me/BotFather).
2. Put the token in `TELEGRAM_TOKEN`.
3. Put your own numeric Telegram ID in `ADMIN_TELEGRAM_ID`.

### Configure the Mini App / Menu Button

1. Open your bot in BotFather.
2. Go to `Bot Settings`.
3. Configure the menu button with your Mini App URL.
4. Set the URL to the same HTTPS origin you deploy for the app, for example:

```text
https://shop.example.com
```

Also set:

- `DOMAIN_NAME=shop.example.com`
- `MINI_APP_URL=https://shop.example.com`

If `MINI_APP_URL` is left empty, the Mini App link is effectively disabled.

## Configuration

The full template lives in [.env.sample](.env.sample).

### Core Required Variables

| Variable | Purpose |
| --- | --- |
| `TELEGRAM_TOKEN` | Bot token from BotFather |
| `ADMIN_TELEGRAM_ID` | Telegram user ID allowed to run admin commands |
| `REMNAWAVE_URL` | Remnawave panel base URL |
| `REMNAWAVE_TOKEN` | Remnawave API token |
| `PLANS` | Comma-separated plan definitions: `Label|Days|Price|TrafficGB` |
| `DATABASE_URL` | PostgreSQL connection string |

### Domain and Web App

| Variable | Purpose |
| --- | --- |
| `DOMAIN_NAME` | Public domain used by the optional bundled Caddy proxy |
| `ACME_EMAIL` | Email for the optional bundled Caddy / Let's Encrypt flow |
| `MINI_APP_URL` | Full HTTPS URL for the Telegram Mini App |
| `HEALTH_CHECK_PORT` | Internal HTTP listen port, default `8080` |
| `IS_WEB_APP_LINK` | Whether to present the web app as the primary user-facing link |

### Remnawave

| Variable | Purpose |
| --- | --- |
| `REMNAWAVE_MODE` | `remote` or `local` |
| `REMNAWAVE_TAG` | Optional tag attached to generated subscriptions |
| `TRIAL_REMNAWAVE_TAG` | Optional separate tag for trial users |
| `REMNAWAVE_HEADERS` | Optional extra API headers in `key:value;key:value` format |
| `SQUAD_UUIDS` | Comma-separated Remnawave squad UUIDs |
| `EXTERNAL_SQUAD_UUID` | Optional external squad UUID |

### Payments

#### Crypto Pay

| Variable | Purpose |
| --- | --- |
| `CRYPTO_PAY_ENABLED` | Legacy flag. Keep `false` unless you are reintroducing crypto checkout intentionally |
| `CRYPTO_PAY_TOKEN` | Legacy Crypto Pay API token |
| `CRYPTO_PAY_URL` | Legacy Crypto Pay API base URL |

#### Mobile Banking

| Variable | Purpose |
| --- | --- |
| `MOBILE_BANKING_ENABLED` | Enable or disable mobile banking |
| `MOBILE_BANKING_PHONE` | Default display phone number |
| `OPENROUTER_API_KEY` | OpenRouter API key for screenshot verification |
| `OPENROUTER_MODEL` | Primary OpenRouter model |
| `OPENROUTER_FALLBACK_MODEL` | Optional fallback model via OpenRouter |
| `VISION_PROVIDER_FALLBACK` | Fallback provider, for example `openrouter` or `gemini` |
| `VISION_RETRY_ATTEMPTS` | Retries per provider before failover |
| `VISION_RETRY_MAX_ATTEMPTS` | Total attempts across primary and fallback |
| `VISION_ACCEPT_CONFIDENCE_THRESHOLD` | Minimum confidence to accept a screenshot analysis result. Lower this to reduce false negatives. |
| `VISION_REJECT_CONFIDENCE_THRESHOLD` | Minimum confidence before an invalid result is treated as a hard reject. Raise this to reduce false negatives. |

Advanced note:

- screenshot verification can use OpenRouter and Gemini-backed providers
- the sample file is OpenRouter-first because that is the default production path in this repository
- if you want Gemini available, configure `GEMINI_API_KEY` and optionally `GEMINI_MODEL`
- low-confidence or blurry results now trigger a clearer-screenshot response instead of a generic reject
- analyzer logs include explicit `decision` values so operators can see retry, fallback, reject, and ask-clearer paths

#### Wallet, Referrals, Trials

| Variable | Purpose |
| --- | --- |
| `REFERRAL_DAYS` | Reward or extension window for referrals |
| `TRIAL_DAYS` | Trial duration in days |
| `TRIAL_TRAFFIC_LIMIT` | Trial traffic cap |
| `TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY` | Trial reset strategy |

### Optional External Links

These show up in the user experience if set:

- `SERVER_STATUS_URL`
- `SUPPORT_URL`
- `FEEDBACK_URL`
- `CHANNEL_URL`
- `TOS_URL`

### Access Control

| Variable | Purpose |
| --- | --- |
| `BLOCKED_TELEGRAM_IDS` | Comma-separated blocked user IDs |
| `WHITELISTED_TELEGRAM_IDS` | Comma-separated allowlisted user IDs |

### Backups

| Variable | Purpose |
| --- | --- |
| `BACKUP_ENABLED` | Enable scheduled DB backups |
| `BACKUP_SCHEDULE_CRON` | Daily cron expression |
| `BACKUP_TIMEZONE` | Backup timezone |
| `BACKUP_DIR` | In-container backup path, default `/backups` |
| `BACKUP_RETENTION_DAYS` | Retention window |
| `BACKUP_MAX_LOCAL_FILES` | Max number of local backup files |
| `BACKUP_SEND_TO_TELEGRAM` | Send successful backups to admin chat |
| `BACKUP_RESTORE_ENABLED` | Enables restore-related config, but live runtime restore remains disabled |
| `BACKUP_CONFIRM_TTL_MINUTES` | Restore confirmation token lifetime |
| `BACKUP_JOB_TIMEOUT_SECONDS` | Backup job timeout |
| `BACKUP_RESTORE_TIMEOUT_SECONDS` | Restore timeout |

### Reverse Proxy Header Trust

By default, the app does not trust private-network `X-Forwarded-For` values unless you opt in.

Set this only when you know your proxy chain is correct:

```bash
TRUST_PRIVATE_PROXY_HEADERS=true
```

This matters when you put another private reverse proxy in front of the bot or Caddy.

## Payments and Checkout Flows

### Crypto Pay

- crypto checkout is currently disabled in this runtime
- keep the config unset unless you plan to restore that payment rail in a future change

### Mobile Banking

- customer selects a provider
- customer transfers money manually
- customer uploads a screenshot
- screenshot analysis validates the payment details
- purchase is fulfilled if verification succeeds

The current runtime includes:

- retry and fallback support for screenshot analyzers
- throttling for repeated screenshot uploads on the same purchase
- readiness failure when screenshot verification providers are degraded
- safer handling of overlapping verification attempts
- screenshot uploads are limited to receipt-based purchases only

### Wallet

- customer can top up wallet balance
- wallet purchases reuse internal balance for fast checkout
- wallet debit and wallet ledger insertion are handled atomically

### Promo Codes and Referrals

- promo validation is exposed through the Mini App API
- promo usage is reserved when the purchase is created
- referrals and wallet data are available through the API and Mini App

## Reliability and Safety Behavior

This codebase is opinionated about not losing money or silently mis-marking purchases.

Important runtime behavior:

- DB migrations run automatically on startup from [`db/migrations`](db/migrations)
- purchase creation supports idempotency through the `Idempotency-Key` request header
- wallet purchases are transactionally protected
- purchases move through a processing state before being marked paid
- scheduled jobs use timeout wrappers and panic recovery
- the HTTP server uses read, write, header, and idle timeouts
- Telegram Mini App auth is validated server-side and bound to a replay guard

If you are building your own client instead of using the bundled Mini App, send a unique `Idempotency-Key` for purchase creation.

## API Overview

The Go app serves the Mini App and these API routes:

| Route | Purpose |
| --- | --- |
| `/api/me` | Current authenticated customer |
| `/api/plans` | Available plans |
| `/api/purchase` | Create a purchase |
| `/api/upload_screenshot` | Upload screenshot for mobile banking verification |
| `/api/purchase/status` | Poll purchase state |
| `/api/promo/validate` | Validate promo code |
| `/api/trial` | Activate a trial |
| `/api/wallet` | Wallet summary |
| `/api/wallet/history` | Wallet transaction history |
| `/api/wallet/autorenew` | Wallet-based auto-renew settings |
| `/api/referrals` | Referral information |
| `/api/keys/autorenew` | Per-key auto-renew settings |
| `/api/revenue` | Admin revenue summary |
| `/redirect` | Simple redirect helper |

Notes:

- Mini App auth uses Telegram `initData`
- CORS allows `Authorization`, `Content-Type`, and `Idempotency-Key`
- the frontend is served from `web-app/dist` with SPA fallback

## Admin Commands

The admin Telegram account can operate the bot from chat.

Primary entrypoint:

- `/admin`

The dashboard is organized into four sections:

- `Overview`: revenue, recent transactions, provider status, backup health
- `Payments`: provider phone/name updates, provider disabling, referral bonus
- `Backups`: backup now, status, list, schedule controls, restore guidance
- `Operations`: user sync, subscription notifications, test mode, fallback help

Most older admin slash commands still work as hidden emergency fallbacks, but they are no longer the primary operator UX. Use the dashboard first.

Fallback commands that remain available include:

- `/backup now|status|list|enable|disable|schedule HH:MM`
- `/restore list`
- `/sync`
- `/test enable|disable`
- `/notify <telegram_id>`
- `/setreferralbonus <amount>`
- `/setphone <provider> <number>`
- `/setname <provider> <name>`
- `/disablephone <provider>`
- `/disablename <provider>`

Important restore note:

- live runtime restore is intentionally disabled for safety
- use `/restore list` to identify the backup you want
- stop the app and perform the restore offline/manual

## Backups and Restore

There are two backup paths in this repository.

For VPS-to-VPS migration and rollback sequencing, follow:

- [docs/PRODUCTION_MIGRATION_RUNBOOK.md](docs/PRODUCTION_MIGRATION_RUNBOOK.md)

### 1. Bot-Managed DB Backups

The running bot can create scheduled or on-demand PostgreSQL backups into the `/backups` volume.

The compose stack mounts:

- `bot_backups:/backups`

Use admin commands to trigger or inspect these backups.

### 2. Full-System Backup via `setup.sh`

The setup wizard can create and restore a broader archive that includes:

- database dump
- `.env`
- `translations`
- Caddy data

Use the menu options in `setup.sh` for this workflow when doing environment migration or disaster recovery.

### Restore Policy

For safety, live runtime restore through the bot process is disabled.

Recommended restore workflow:

1. stop the app
2. run `/restore list` to identify the backup you want
3. perform the restore offline/manual
4. restart the app
5. verify health endpoints and bot behavior

## Health Checks and Operations

### Health Endpoints

- `/livez`: liveness probe
- `/readyz`: readiness probe for database, Remnawave, and screenshot verification providers
- `/healthcheck`: dependency-aware JSON health report with the same readiness decision

The Docker healthcheck uses `/readyz` on `127.0.0.1:${HEALTH_CHECK_PORT}`.

### Logs

```bash
docker compose logs -f bot
docker compose logs -f db
```

If you started bundled Caddy:

```bash
docker compose logs -f caddy
```

### Upgrades

The app runs SQL migrations automatically on startup, so a normal upgrade is:

```bash
git pull
docker compose up -d --build bot db
```

If you use bundled Caddy, refresh it separately after the app comes back:

```bash
docker compose up -d caddy
```

If you are doing a production update, verify:

- `.env` changes are applied
- the Mini App URL is still correct
- payment provider credentials remain valid
- backup schedules still match your expectations

## Local Development

### Backend

The Go app expects the built frontend at `web-app/dist` when serving the Mini App.

```bash
cp .env.sample .env
cd web-app
npm ci
npm run build
cd ..
go run ./cmd/app
```

### Frontend

For frontend-only iteration:

```bash
cd web-app
npm ci
npm run dev
```

For end-to-end behavior, Docker Compose is still the recommended path because the Mini App depends on the Go backend and database.

## Testing

Backend:

```bash
go test ./...
go test -race ./...
```

Frontend:

```bash
cd web-app
npm test -- --run
npm run build
```

## Troubleshooting

### Mini App loads blank or fails to authenticate

Check:

- `MINI_APP_URL`
- `DOMAIN_NAME`
- HTTPS termination in Caddy
- BotFather menu button URL

### Mobile banking verification fails immediately

Check:

- `MOBILE_BANKING_ENABLED=true`
- `MOBILE_BANKING_PHONE`
- `OPENROUTER_API_KEY` or `GEMINI_API_KEY`
- model configuration and fallback settings

### Backup command runs but file is not delivered

Check:

- `BACKUP_SEND_TO_TELEGRAM=true`
- `ADMIN_TELEGRAM_ID`
- bot logs
- local `/backups` volume contents

### Real client IPs are not showing correctly

If you run another private reverse proxy in front of the app, set:

```bash
TRUST_PRIVATE_PROXY_HEADERS=true
```

Do not enable that blindly on untrusted networks.

### Bot image does not have the latest backup tooling

Rebuild the container:

```bash
docker compose up -d --build bot
```

## Deployment Files

- [docker-compose.yaml](docker-compose.yaml)
- [Dockerfile](Dockerfile)
- [Caddyfile](Caddyfile)
- [setup.sh](setup.sh)
- [.env.sample](.env.sample)

## License

MIT
