# Remnawave Telegram Shop

[![Stars](https://img.shields.io/github/stars/Jolymmiels/remnawave-telegram-shop.svg?style=social)](https://github.com/Jolymmiels/remnawave-telegram-shop/stargazers)
[![Forks](https://img.shields.io/github/forks/Jolymmiels/remnawave-telegram-shop.svg?style=social)](https://github.com/Jolymmiels/remnawave-telegram-shop/network/members)
[![Issues](https://img.shields.io/github/issues/Jolymmiels/remnawave-telegram-shop.svg)](https://github.com/Jolymmiels/remnawave-telegram-shop/issues)

A powerful, production-ready Telegram bot for selling VPN subscriptions with automated fulfillment via [Remnawave](https://remna.st/). Features a Modern Mini App shop (Wavy VPN), multi-language support (English/Russian/Burmese), and AI-powered mobile payment verification.

## 🚀 Key Features

-   **Full Purchase Flow**: Users buy keys directly in Telegram via a Mini App.
-   **Automated Fulfillment**: Instantly creates/extends users in your Remnawave panel.
-   **Payment Flexibility**:
    -   **CryptoPay**: Automatic crypto payments.
    -   **Mobile Banking (KPay/Wave)**: AI-verified screenshot uploads (powered by Gemini).
    -   **Telegram Stars**: Native Telegram currency support.
    -   **YooKassa**: Ruble payments.
-   **Smart Notifications**: Expiry alerts sent 3 days before termination.
-   **Localization**: Complete support for English 🇺🇸, Russian 🇷🇺, and Burmese 🇲🇲.
-   **Analytics**: Tracks revenue, daily sales, and user growth.

---

## 🛠 Usage & Setup

The easiest way to install, configure, and manage the bot is via the interactive **Setup Wizard**.

### 1. Quick Start (Recommended)

Run the wizard to handle everything from Docker installation to SSL certificates.

```bash
chmod +x setup.sh
./setup.sh
```

**What the wizard does:**
1.  Checks/Installs Docker & Docker Compose.
2.  Helps you configure `.env` interactively.
3.  Sets up SSL (Caddy) automatically.
4.  Builds and starts the services.

### 2. Manual Commands

If you prefer managing Docker manually:

**Start/Restart:**
```bash
docker compose up -d
```

**View Logs:**
```bash
docker compose logs -f --tail 100
```

**🛑 Update (CRITICAL)**
Because the frontend (Mini App) is compiled *inside* the Docker image, you **MUST** rebuild when updating:

```bash
git pull
docker compose build --no-cache
docker compose up -d
```
> **Note**: If you use `./setup.sh`, simply choose **Option 7: Update**.

---

## 🏗 Architecture

The project consists of a Go backend and a React/Vite frontend.

-   **Backend**: Go (Golang) service that handles Telegram updates, payment logic, and database operations.
-   **Frontend**: React Mini App (`web-app/`) that runs inside Telegram for a smooth shopping experience.
-   **Database**: PostgreSQL for storing purchases, customers, and verification logs.

👉 **[Read full Architecture Documentation](docs/STRUCTURE.md)** for directory layout and data flow.

---

## ⚙️ Environment Variables

The `./setup.sh` wizard will generate this for you, but here is the reference.

### Required
| Variable | Description |
| :--- | :--- |
| `TELEGRAM_TOKEN` | Your Bot Token from @BotFather. |
| `ADMIN_TELEGRAM_ID` | Your Telegram User ID (for admin commands). |
| `REMNAWAVE_URL` | URL of your Remnawave Panel. |
| `REMNAWAVE_TOKEN` | API Token from Remnawave. |
| `DOMAIN_NAME` | Domain for the bot (e.g., `shop.example.com`). Required for SSL and Mini App. |

### Payments
| Variable | Description |
| :--- | :--- |
| `MOBILE_BANKING_ENABLED` | `true`. Enables AI screenshot verification. |
| `MOBILE_BANKING_PHONE` | The phone number users should send money to. |
| `GEMINI_API_KEY` | Google Gemini API key for analyzing screenshots. |
| `CRYPTO_PAY_TOKEN` | Token for @CryptoBot (if enabled). |

### Optional
| Variable | Description |
| :--- | :--- |
| `DEFAULT_LANGUAGE` | `en`, `ru`, or `my` (Burmese). |
| `SQUAD_UUIDS` | Specific Remnawave Squad UUIDs to assign users to. |
| `MINI_APP_URL` | Direct link to the Mini App (optional, for menu button). |

*See `.env.sample` for all available options.*

---

## 📱 Mini App (Wavy VPN)

The frontend is a React app located in `web-app/`.
It handles:
-   **Plan Selection**: Displays available plans.
-   **Checkout**: Copy-pasteable amounts and phone numbers.
-   **Verification**: Users upload screenshots directly in the UI.
-   **Key Management**: Users can see their active keys (`WV-123-1`) and "Copy Link".

**Development**:
To work on the frontend locally:
```bash
cd web-app
npm install
npm run dev
```

---

## 🤝 Version Support

| Remnawave Version | Bot Version |
| :--- | :--- |
| 1.6 | 2.3.6 |
| 2.0.0 - 2.1.9 | 3.2.4 |
| 2.2.* | 3.2.5 |
| 2.3.* | 3.5.* (Current) |

---

## 💸 Donations

If this project helps your business, consider supporting development!

-   **Bep20 USDT**: `0x4D1ee2445fdC88fA49B9d02FB8ee3633f45Bef48`
-   **SOL (Solana)**: `HNQhe6SCoU5UDZicFKMbYjQNv9Muh39WaEWbZayQ9Nn8`
-   **TRC20 USDT**: `TBJrguLia8tvydsQ2CotUDTYtCiLDA4nPW`
-   **TON**: `UQAdAhVxOr9LS07DDQh0vNzX2575Eu0eOByjImY1yheatXgr`
