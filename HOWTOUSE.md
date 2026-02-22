# User Manual & Operations Guide

This guide covers how to operate the Remnawave Telegram Shop Bot after installation.

## Admin Commands

To use these commands, your Telegram ID must be set in `ADMIN_TELEGRAM_ID` in the `.env` file.

| Command | Description |
| :--- | :--- |
| `/admin` | Opens the main admin dashboard. |
| `/broadcast` | Send a message to all users. Supports text, photos, and buttons. |
| `/stats` | View sales statistics (daily revenue, user count). |
| `/id` | Get your Telegram ID (useful for configuring whitelist/blocklist). |
| `/test` | Toggle "Test Mode" (allows using Magic Transaction IDs). |

### Using Test Mode
1.  Run `/test` (Admin only). The bot will reply "Test Mode ENABLED".
2.  Use the Magic Transaction ID: `01004063070995016447`.
3.  Go to **Wallet** -> Top up -> Select Mobile Banking.
4.  Upload *any* screenshot.
5.  Wait for verification (it will auto-approve).
6.  The system will bypass duplicate checks for this ID by appending a timestamp.

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

### CryptoPay
-   **Setup**: Enable in `setup.sh`, provide `CRYPTO_PAY_TOKEN`.
-   **Usage**: Fully automated. User clicks "Pay", sends crypto, bot detects tx, plan activates immediately.

### Mobile Banking (Manual / AI)
-   **Setup**: Enable in `setup.sh`, set `MOBILE_BANKING_PHONE` and `GEMINI_API_KEY`.
-   **Usage**:
    1.  User selects plan & payment method.
    2.  Bot sends instructions ("Transfer money to 09...").
    3.  User transfers money and **sends screenshot** to the bot.
    4.  **AI Verification**: Gemini reads the screenshot. Checks amount, phone number, and transaction ID.
    5.  **Success**: Bot activates plan automatically.
    6.  **Failure**: Bot asks user to try again or contact support.

### Wallet System
-   Users can "Top Up" their internal balance using Crypto or Mobile Banking.
-   Purchases made using Wallet Balance are instant.

---

## Migration & Backups

You can move your entire shop to a new server without losing data.

### 1. Create Backup (Old Server)
1.  Run `./setup.sh`.
2.  Select **Option 9 (Backup)**.
3.  Wait for success message.
4.  Download the file from `remnawave-telegram-shop-main/backups/backup_YYYYMMDD.tar.gz`.

### 2. Restore (New Server)
1.  Install Docker & Git on new server.
2.  Clone this repository.
3.  Create a `backups` folder and upload your `.tar.gz` file there.
4.  Run `./setup.sh`.
5.  Select **Option 10 (Restore)**.
6.  Choose your backup file.
7.  The script will stop services, restore DB/Certs, and restart everything.

---

## Troubleshooting

### Blank Screen in Mini App
-   **Cause**: Invalid SSL, mixed content (HTTP vs HTTPS), or `MINI_APP_URL` mismatch.
-   **Fix**: Ensure `DOMAIN_NAME` in `.env` matches your actual domain and points to your server's IP. Caddy handles SSL automatically.

### "Dirty Database Version" Error
-   **Cause**: A migration failed mid-way (e.g., bot crashed during update).
-   **Fix**:
    1.  Stop bot: `docker-compose stop bot`
    2.  Reset DB version (example to version 15):
        `docker exec remnawave-telegram-shop-db psql -U postgres -d postgres -c "UPDATE schema_migrations SET dirty = false, version = 15;"`
    3.  Restart: `docker-compose up -d`

### Duplicate Transaction ID Error
-   **Cause**: Testing with the same receipt twice.
-   **Fix**: Use Test Mode (`/test`), which bypasses this check safely.
