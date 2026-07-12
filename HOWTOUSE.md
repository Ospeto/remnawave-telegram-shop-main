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

---

## Structured finance reporting

1. Open the Mini App as admin → **Finance** card → `/admin/finance`.
2. Use Daily/Weekly/Monthly/Yearly tabs or a custom Yangon date range.
3. Headline cards show the **selected period** (not the full history window); the chart shows dense history. Export CSV from the page (same totals as on-screen JSON metrics).
4. Telegram `/revenue` and scheduled daily/weekly/monthly jobs use the same `FinanceService` definitions (Net Income, Gross, Refunds, Cash).

### Recording a service refund

Service refunds are **not** inferred from purchase status and are **not** wallet cleanup refunds.

```http
POST /api/admin/financial-adjustments
Authorization: <admin mini-app session>
Content-Type: application/json

{
  "adjustment_type": "refund",
  "amount": 1000.00,
  "currency": "MMK",
  "purchase_id": 123,
  "effective_at": "2026-07-12T10:00:00+06:30",
  "reason": "customer request",
  "external_ref": "bank-txn-1",
  "idempotency_key": "refund:123:bank-txn-1"
}
```

- Replay with the same `idempotency_key` and the same payload returns the existing row (HTTP 200). Same key with a different amount/currency/type/effective_at/purchase_id returns HTTP 409.
- This endpoint writes only the finance ledger. It does **not** change purchase fulfillment, Remnawave state, or wallet balances.
- Wallet cleanup refunds (`wallet_transaction.type = refund`) are operational wallet corrections and must not be entered as service refunds.
- Historical refunds are not auto-backfilled; enter explicit adjustments after reconciliation.

---

## Reseller wholesale pricing

Admin-approved Telegram customers pay fixed per-plan wholesale prices in the same Mini App (and bot sell path). Pricing is server-authoritative; promos do not stack with wholesale.

### Approve a reseller

1. Open the Mini App as admin → **Resellers** card → `/admin/resellers`.
2. Enter the customer’s Telegram ID and enable reseller (or disable to revoke).
3. API equivalent:

```http
PATCH /api/admin/customers/{telegram_id}/reseller
Authorization: <admin mini-app session>
Content-Type: application/json

{ "is_reseller": true }
```

- List: `GET /api/admin/resellers`.
- Only admins can set/clear `is_reseller`. Removing the flag does not rewrite past purchases; new purchases use the current flag. A pending purchase keeps the amount frozen at create time.

### Set plan wholesale prices

1. Admin Mini App → **Plans** → `/admin/plans`.
2. Set optional **wholesale price** per plan (integer MMK).
3. Rules:
   - If set: must be **> 0** and **≤ retail price** (save rejected otherwise).
   - Clear the field to remove wholesale (reseller then pays retail for that plan).
   - Reseller buying a plan with **no** wholesale configured: **falls back to retail** (sale is not blocked).

### What resellers see and pay

- Same Mini App purchase flow; effective price only (no retail strikethrough in v1). “Reseller price” badge when tier is wholesale.
- Promo codes: UI hidden; server rejects promo on purchase create and promo validate (cannot combine with reseller pricing).
- Payment methods: **mobile banking + wallet** only. Crypto Pay stays disabled.
- Keys remain on the **reseller’s** Telegram account (offline resale/sharing). No gift/assign to another user in v1.

### Purchase audit & finance

- Each new purchase stores `pricing_tier` = `retail` | `wholesale` from `ResolvePlanPrice`.
- Finance / revenue still uses **paid amounts** (gross, refunds, net). No v1 Finance page split by retail vs wholesale; the tier field is for later reporting.
- **No historical backfill** as wholesale — migration defaults existing purchases to `retail`.

### Ops checklist

| Task | Where |
|------|--------|
| Mark/unmark reseller | `/admin/resellers` or `PATCH …/reseller` |
| Set/clear wholesale price | `/admin/plans` (`wholesale_price`) |
| Confirm effective prices | Log in as reseller → Plans / Checkout |
| Confirm promo blocked | Reseller + promo → HTTP 400 |
| Confirm keys on buyer | Fulfillment unchanged (buyer account only) |

---

## Reseller postpaid credit & sales ledger

Approved resellers can optionally buy **on account** (postpaid) in the Mini App when they have remaining credit. AR is a **separate ledger from wallet** — wallet stays prepaid and non-negative. Bot postpaid and mobile-banking settlement are **not** in v1.

### Enable reseller + set credit limit

1. Approve the customer as reseller (same as wholesale): Mini App → **Resellers** → `/admin/resellers`, or `PATCH /api/admin/customers/{telegram_id}/reseller` with `{ "is_reseller": true }`.
2. Set a **credit limit** on that reseller (required for postpaid; default is no credit):
   - UI: `/admin/resellers` → edit credit limit for the row.
   - API:

```http
PATCH /api/admin/customers/{telegram_id}/credit
Authorization: <admin mini-app session>
Content-Type: application/json

{ "credit_limit": 100000 }
```

3. Env default for **new** reseller accounts:

| Env | Default | Meaning |
|-----|---------|---------|
| `RESELLER_DEFAULT_CREDIT_LIMIT` | `0` | Starting credit limit when an account is first ensured. **`0` = no postpaid credit until admin sets a limit.** |

- List fields: `GET /api/admin/resellers` includes `credit_limit`, `balance_owed`, `remaining_credit` per reseller.
- Remaining credit = `credit_limit − balance_owed`. Order amount must be `≤ remaining credit` (no partial fulfill).
- Clearing `is_reseller` while balance is owed: **new postpaid blocked**; settlement still allowed; past ledger kept.

### Postpaid checkout

- Available only to `is_reseller` customers in the **Mini App** Checkout when remaining credit covers the order (or limit &gt; 0 with clear messaging). Prepaid (mobile banking / wallet) remains available.
- Flow: credit check → create service purchase at `ResolvePlanPrice` amount → **fulfill immediately** → AR ledger **sale** increases `balance_owed`.
- **No cash and no wallet movement** on postpaid create.
- Promo codes stay blocked for resellers (UI + server HTTP 400).
- Keys still fulfill to the **buyer** (reseller’s Telegram account).

### Settlement (pay down AR)

Two rails; both reduce `balance_owed` on the AR ledger:

| Who | Endpoint | Effect |
|-----|----------|--------|
| Reseller self-pay | `POST /api/reseller/settlements` | Debits **wallet** for the settlement amount (wallet must cover it; **no negative wallet**). Body: `{ "amount", "payment_method": "wallet", "idempotency_key"? }`. |
| Admin offline | `POST /api/admin/customers/{telegram_id}/settlements` | **Ledger-only** — records cash received offline; **does not** debit wallet. Body: `{ "amount", "note"?, "idempotency_key"? }`. |

- Reseller surfaces: `/reseller/account` (balance, limit, remaining, ledger, pay-balance CTA).
- Admin surfaces: `/admin/resellers` (limit / owed / remaining, set limit, record settlement, open ledger).
- Admin ledger: `GET /api/admin/customers/{telegram_id}/ledger`. Reseller own ledger: `GET /api/reseller/ledger`.

### Finance reporting

| Metric | When counted |
|--------|----------------|
| **Service revenue (gross)** | Postpaid **sale / fulfill date** (paid service purchase amount) |
| **Cash collected** | Settlement **`effective_at`** (self-pay or admin offline) |

- Do **not** treat postpaid create as cash. Settling later must not double-count revenue.
- Same `FinanceService` / `/admin/finance` / `/revenue` definitions as other sales.

### Money-safety invariants

- **AR ledger ≠ wallet.** Postpaid owed balance is not a negative wallet.
- Wallet never goes negative; self-settlement fails if wallet balance is insufficient.
- Admin settlement is **ledger-only** (no purchase/wallet mutation).
- **No AR historical backfill** of past prepaid wholesale purchases into the credit ledger.
- Promo still blocked on all reseller purchase paths (including postpaid).

### Ops checklist

| Task | Where |
|------|--------|
| Enable reseller | `/admin/resellers` or `PATCH …/reseller` |
| Set credit limit | `/admin/resellers` or `PATCH …/credit` |
| Default limit for new accounts | `RESELLER_DEFAULT_CREDIT_LIMIT` (default `0`) |
| Postpaid buy | Reseller Mini App Checkout → Postpaid |
| Reseller pay-down | `/reseller/account` or `POST /api/reseller/settlements` |
| Admin offline settlement | `/admin/resellers` or `POST …/settlements` |
| View ledger | Reseller: `/reseller/account`; Admin: ledger on resellers page |
| Confirm revenue vs cash | Finance: sale date = revenue; settlement date = cash |
