# Remnawave Telegram Shop Bot

A complete, self-hosted Telegram Shop Bot for selling digital goods (subscription keys) with automated delivery, multi-currency support, and a modern React-based Mini App UI.

## Key Features

-   **Automated Sales**: Instant delivery of subscription keys (e.g., VPN keys, software licenses) via Remnawave API integration.
-   **Telegram Mini App**: A beautiful, responsive React frontend for browsing plans and managing subscriptions directly within Telegram.
-   **Multi-Payment Support**:
    -   **CryptoPay**: Accept cryptocurrency payments automatically.
    -   **Mobile Banking**: Semi-automated workflow for manual bank transfers (KPay, WavePay, etc.) with screenshot verification (AI-powered options available).
    -   **Wallet System**: Built-in user wallet for top-ups and quick purchases.
-   **Referral System**: Built-in referral tracking and rewards.
-   **Admin Tools**: Manage plans, users, and broadcasts directly from Telegram or the database.
-   **Dockerized**: Easy deployment with `docker-compose`.

## Tech Stack

-   **Backend**: Go (Golang) 1.21+
-   **Frontend**: React 18, TypeScript, Vite, TailwindCSS (Telegram Mini App)
-   **Database**: PostgreSQL 16
-   **Infrastructure**: Docker & Docker Compose
-   **Reverse Proxy**: Caddy (Automatic HTTPS)

## Prerequisites

-   **Docker** and **Docker Compose** installed on your server or local machine.
-   A **Telegram Bot Token** (from [@BotFather](https://t.me/BotFather)).
-   A **Remnawave Panel** URL and API Token (for key generation).
-   *(Optional)* **CryptoPay** API Token for crypto payments.
-   **Gemini API Key** for primary AI verification of payment screenshots.
-   *(Optional)* **OpenRouter API Key** as fallback if Gemini is unavailable.

## Getting Started

The project includes an interactive setup wizard to get you running in minutes.

### 1. Clone the Repository

```bash
git clone https://github.com/Ospeto/remnawave-telegram-shop-main.git
cd remnawave-telegram-shop-main
```

### 2. Run the Setup Wizard

This script will guide you through configuration, environment setup, and deployment.

```bash
chmod +x setup.sh
./setup.sh
```

Select **Option 1 (Fresh Install)** and answer the prompts.

### 3. Manual Configuration (Alternative)

If you prefer determining settings manually:

1.  Copy `.env.example` to `.env`.
2.  Edit `.env` with your credentials (`TELEGRAM_TOKEN`, `DATABASE_URL`, etc.).
3.  Start services:
    ```bash
    docker-compose up -d --build
    ```

### 4. Setup the Mini App

1.  Go to [@BotFather](https://t.me/BotFather) in Telegram.
2.  Select your bot.
3.  Go to **Bot Settings** -> **Menu Button** -> **Configure Menu Button**.
4.  Send the URL of your deployed Mini App (e.g., `https://your-domain.com`).
5.  Give the button a title (e.g., "Open Shop").

## Architecture Overview

### Directory Structure

```
├── cmd/                # Application entry points
│   └── app/            # Main bot executable
├── internal/           # Private application code
│   ├── config/         # Configuration loading
│   ├── database/       # Database access layer (Repositories)
│   ├── handler/        # Telegram command & callback handlers
│   ├── payment/        # Payment processing logic
│   ├── remnawave/      # Remnawave API client
│   └── service/        # Business logic services
├── web-app/            # Frontend (React + Vite)
│   ├── src/            # Source code
│   └── dist/           # Built assets (served by Go backend)
├── db/                 # Database migrations
├── setup.sh            # Interactive deployment script
├── docker-compose.yaml # Container orchestration
└── Dockerfile          # Multi-stage build definition
```

### Data Flow

```
User → Telegram Bot (Command/Mini App)
↓
Go Backend (Webhook/Polling)
↓
PostgreSQL (State/Transactions) ←→ Remnawave API (Key Management)
```

## Environment Variables

Key variables used in `.env`:

| Variable | Description |
| :--- | :--- |
| `TELEGRAM_TOKEN` | Bot API token from BotFather |
| `ADMIN_TELEGRAM_ID` | Your Telegram numeric ID for admin commands |
| `REMNAWAVE_URL` | URL of your Remnawave panel |
| `REMNAWAVE_TOKEN` | Admin API token for Remnawave |
| `DATABASE_URL` | Connection string `postgres://user:pass@host:5432/db` |
| `PLANS` | Config string for subscription plans |
| `DOMAIN_NAME` | Your domain for Caddy (SSL) |

## Backup & Restore

The `setup.sh` script includes built-in tools for migration:

-   **Backup**: `./setup.sh` -> Option 9. Creates a tarball of DB, Config, and Certs.
-   **Restore**: `./setup.sh` -> Option 10. Restores a system from a backup tarball.

## Troubleshooting

-   **Bot not responding?** Check logs: `docker-compose logs -f bot`
-   **Database crashed?** Use `fix_db_crash.sh` or reset via setup wizard.
-   **Mini App Blank Screen?** Ensure `MINI_APP_URL` is set correctly and HTTPS is working.

## License

MIT License.
