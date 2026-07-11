# User Manual & Operations Guide

This guide covers day-to-day operation after installation.

## Admin Commands

To use these commands, your Telegram ID must be set in `ADMIN_TELEGRAM_ID` in the `.env` file.

The primary admin entrypoint is `/admin`.

The dashboard is organized into four sections:

- `Overview`: revenue, recent transactions, provider status, backup health
- `Payments`: update provider phone/name, disable providers, adjust referral bonus
- `Backups`: run backups, inspect status, manage schedule, view restore guidance
- `Operations`: sync users, run the synthetic E2E canary, send notifications, toggle test mode, review fallbacks

Most older admin slash commands still exist as hidden emergency fallbacks, but they are no longer the primary way to operate the bot.

Hidden fallbacks:

- `/backup now|status|list|enable|disable|schedule HH:MM`
- `/restore list`
- `/sync`
- `/test enable|disable`
- `/healthbot run`
- `/notify <telegram_id>`
- `/setreferralbonus <amount>`
- `/setphone <provider> <number>`
- `/setname <provider> <name>`
- `/disablephone <provider>`
- `/disablename <provider>`

### Using Test Mode
1.  Open `/admin` -> `Operations` -> `Enable Test Mode`.
2.  Use the Magic Transaction ID: `01004063070995016447`.
3.  Go to **Wallet** -> Top up -> Select Mobile Banking.
4.  Upload *any* screenshot.
5.  Wait for verification; it will auto-approve.
6.  Turn test mode off with `/test disable` when you are done.

---

## Managing Plans

Plans are configured in the `.env` file or via `setup.sh` (Option 3).

**Format:**
`PLANS="Label|Days|Price|Limit,Label2|Days2|Price2|Limit2"`

### Examples

**Unlimited 30 Days for 10,000 MMK:**
`Unlimited|30|10000|0`

**20GB Limited 30 Days for 5,000 MMK:**
`Budget|30|5000|20`

### Applying Changes
After editing plans, you must restart the bot:
1.  Run `./setup.sh`
2.  Select **Option 4** (Restart Services).

---

## Payment Methods

### Mobile Banking (Manual / AI)
-   **Setup**: Enable in `setup.sh`, set `MOBILE_BANKING_PHONE` and `OPENROUTER_API_KEY`. For a Gemini fallback, also set `GEMINI_API_KEY` and choose `VISION_PROVIDER_FALLBACK=gemini`. If you prefer a second OpenRouter model instead, set `OPENROUTER_FALLBACK_MODEL` and choose `VISION_PROVIDER_FALLBACK=openrouter`.
-   **Usage**:
    1.  User selects plan & payment method.
    2.  Bot sends instructions ("Transfer money to 09...").
    3.  User transfers money and **sends screenshot** to the bot.
    4.  **AI Verification**: OpenRouter reads the screenshot first. If configured, the bot fails over either to Gemini or to a second OpenRouter model such as `google/gemini-3.1-flash-lite-preview`.
    5.  **Success**: Bot activates plan automatically.
    6.  **Failure**: Bot asks user to try again or contact support.

### Key Monitoring
-   `/readyz` now fails when database, Remnawave, or screenshot-verification providers are unhealthy.
-   `/healthcheck` returns the same readiness decision in JSON, so expired OpenRouter or Gemini keys show up as `503` instead of staying green.
-   `/admin` -> `Operations` -> `Run E2E Check` or `/healthbot run` triggers a synthetic bot canary that checks analyzer readiness and a disposable fulfillment flow.

### Wallet System
-   Users can "Top Up" their internal balance using Mobile Banking (screenshot verification).
-   Purchases made using Wallet Balance are instant.

---

## Migration & Backups

You can move your shop to a new server without losing data. There are **two** backup artifact types — restore both offline with `setup.sh` (live bot restore is disabled).

| Type | Filename | Created by | What it contains | `setup.sh` restore |
|------|----------|------------|------------------|--------------------|
| Full bundle | `backup_*.tar.gz` | `setup.sh` option **10** | DB + `.env` + translations + Caddy certs (when present) | Option **11** — full restore |
| Bot DB-only | `db_*.sql.gz` | `/admin` → Backups or `/backup now` | PostgreSQL dump only (gzip SQL) | Option **11** — DB only (after copy to host `./backups/`) |

Details and volume copy commands: [BACKUP-RUNBOOK.md](BACKUP-RUNBOOK.md). VPS cutover: [docs/PRODUCTION_MIGRATION_RUNBOOK.md](docs/PRODUCTION_MIGRATION_RUNBOOK.md).

### Runtime Restore Policy

The running bot can help you inspect backups, but it will not perform a live restore.

Use `/admin` -> `Backups` for day-to-day backup work, and use `/restore list` only to identify the file you want before doing the restore offline via `setup.sh`.

### 1. Create Backup (Old Server)

**Preferred for full migration (DB + config + certs):**

1.  Run `./setup.sh` and choose **Option 10 (Backup)**.
2.  Download `./backups/backup_YYYYMMDD_HHMMSS.tar.gz` from the host.

**Bot-managed DB-only (scheduled / admin backup):**

1.  Open `/admin` -> `Backups` -> `Run Backup Now` (or `/backup now`).
2.  Wait for success. Artifact is `db_YYYYMMDD_HHMMSS.sql.gz` on the `bot_backups` volume (`/backups` in the container), **not** under host `./backups/` unless you copy it.
3.  Confirm the name with `/restore list` or:

```bash
docker run --rm -v bot_backups:/backups alpine:3.22 ls -lah /backups
```

### 2. Restore (New Server)

1.  Install Docker & Git on new server.
2.  Clone this repository.
3.  Create a host `backups` folder and place the artifact there:
    - Full bundle: copy `backup_*.tar.gz` into `./backups/`.
    - Bot DB-only: copy `db_*.sql.gz` from the old `bot_backups` volume into `./backups/` (see BACKUP-RUNBOOK). Ensure a valid `.env` already exists (bot dumps do not include config).
4.  Run `./setup.sh`.
5.  Select **Option 11 (Restore)**.
6.  Choose your backup file (menu labels **full bundle** vs **bot DB-only**).
7.  Confirm with `yes`. The script stops services, restores via the offline compose path, and restarts bot+db.
    - Full bundle: may restore DB + `.env` + translations + Caddy data.
    - Bot DB-only: restores PostgreSQL only (gunzip → `psql`); leaves `.env`/certs/translations unchanged.

---

## Troubleshooting

### Blank Screen in Mini App
-   **Cause**: Invalid SSL, mixed content (HTTP vs HTTPS), or `MINI_APP_URL` mismatch.
-   **Fix**:
    1.  Ensure `MINI_APP_URL` is a **public HTTPS** origin and matches BotFather’s menu button URL.
    2.  If you use the **bundled Caddy edge** profile: set `DOMAIN_NAME` to that host and start Caddy with
        `docker compose --profile edge up -d caddy`.
    3.  If you terminate TLS with your own reverse proxy, verify certificates and that traffic reaches the bot on the expected internal port (default `8080`).

### "Dirty Database Version" Error
-   **Cause**: A migration failed mid-way (e.g., bot crashed during update). Do **not** blindly force a version number.
-   **Safer recovery procedure**:
    1.  Stop the bot so it does not keep retrying migrations:
        `docker compose stop bot`
    2.  Inspect bot logs for the failed migration name/version:
        `docker compose logs bot`
    3.  Read the current migration state (**read-only**):
        `docker exec remnawave-telegram-shop-db psql -U postgres -d postgres -c "SELECT version, dirty FROM schema_migrations;"`
    4.  Identify the failed migration under `db/migrations/` and check whether any of its SQL already applied partially.
    5.  If you are unsure about partial state, **restore from a known-good backup** offline via `setup.sh` (see [BACKUP-RUNBOOK.md](BACKUP-RUNBOOK.md)) rather than guessing.
    6.  Only after you know the actual last-known-good version and have repaired or rolled back partial changes, clear the dirty flag / set that version, then restart:
        `docker compose up -d bot db`
    7.  Confirm the bot starts cleanly and migrations complete without a new dirty state.

### Duplicate Transaction ID Error
-   **Cause**: Testing with the same receipt twice.
-   **Fix**: Use Test Mode (`/test`), which bypasses this check safely.
