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
-   Users can "Top Up" their internal balance using Crypto or Mobile Banking.
-   Purchases made using Wallet Balance are instant.

---

## Migration & Backups

You can move your entire shop to a new server without losing data.

### Runtime Restore Policy

The running bot can help you inspect backups, but it will not perform a live restore.

Use `/admin` -> `Backups` for day-to-day backup work, and use `/restore list` only to identify the file you want before doing the restore offline/manual.

### 1. Create Backup (Old Server)
1.  Open `/admin` -> `Backups` -> `Run Backup Now`, or run `./setup.sh` and choose **Option 9 (Backup)**.
2.  Wait for success message.
3.  Download the file from `remnawave-telegram-shop-main/backups/backup_YYYYMMDD.tar.gz`.

### 2. Restore (New Server)
1.  Install Docker & Git on new server.
2.  Clone this repository.
3.  Create a `backups` folder and upload your `.tar.gz` file there.
4.  Use `/restore list` in the old bot if you need to confirm the exact backup filename.
5.  Run `./setup.sh`.
6.  Select **Option 10 (Restore)**.
7.  Choose your backup file.
8.  The script will stop services, restore DB/Certs, and restart everything.

---

## Troubleshooting

### Blank Screen in Mini App
-   **Cause**: Invalid SSL, mixed content (HTTP vs HTTPS), or `MINI_APP_URL` mismatch.
-   **Fix**: Ensure `MINI_APP_URL` matches your real HTTPS origin. If you use the bundled proxy, set `DOMAIN_NAME` to that host and start `caddy`; if you use your own reverse proxy, verify TLS there instead.

### "Dirty Database Version" Error
-   **Cause**: A migration failed mid-way (e.g., bot crashed during update).
-   **Fix**:
    1.  Stop bot: `docker-compose stop bot`
    2.  Reset DB version (example to version 15):
        `docker exec remnawave-telegram-shop-db psql -U postgres -d postgres -c "UPDATE schema_migrations SET dirty = false, version = 15;"`
    3.  Restart: `docker-compose up -d bot db`

### Duplicate Transaction ID Error
-   **Cause**: Testing with the same receipt twice.
-   **Fix**: Use Test Mode (`/test`), which bypasses this check safely.
