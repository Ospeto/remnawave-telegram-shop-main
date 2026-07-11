# Telegram Mini App — Setup Guide

This guide covers the **Telegram Mini App** for the Remnawave Shop bot. The Mini App runs inside Telegram and is served by the Go backend from `web-app/dist`.

---

## What is the Mini App?

The Mini App is a React/Vite web app that opens from the bot menu button. Current surfaces include:

- **Home** — subscription status, key import, auto-renew toggles
- **Plans** — browse plans and apply promo codes before checkout
- **Checkout** — mobile banking payment instructions, screenshot upload, wallet pay
- **Wallet** — balance, history, top-up entry
- **Admin** (authorized operators) — plan and promo management where enabled

---

## Prerequisites

| Requirement | How to check | How to install |
|---|---|---|
| **Node.js 20** | `node -v` | [nodejs.org](https://nodejs.org) or `brew install node@20` (macOS) |
| **npm** | `npm -v` | Comes with Node.js |
| **Bot already set up** | `.env` file exists | Run `./setup.sh` → Fresh Install |
| **Public HTTPS URL** | Domain + TLS | Bundled Caddy `edge` profile, or your own reverse proxy |

Docker production builds use Node 20 in the image; local frontend builds should match.

---

## Quick Setup (Recommended)

Use the setup script from the repo root:

```bash
./setup.sh
```

Choose the **Mini App** setup option from the menu (label may vary by script version).

Typical outcomes:

1. Frontend is built (in Docker, or locally under `web-app/`)
2. You set a public HTTPS `MINI_APP_URL` in `.env`
3. You configure BotFather’s menu button to the same origin

---

## Manual Setup

### Step 1: Install dependencies and build

```bash
cd web-app
npm ci
npm run build
```

This creates `web-app/dist/`. The Go app serves that directory (relative to process CWD) as the Mini App SPA.

### Step 2: Configure `.env`

```env
MINI_APP_URL=https://your-domain.com
```

> **Must be public HTTPS.** Telegram requires a secure Mini App URL.

If you use the **bundled Caddy edge** profile, also set `DOMAIN_NAME` (and `ACME_EMAIL`) to match that host. If you terminate TLS elsewhere, only `MINI_APP_URL` needs to match your public origin.

#### Where do I get a public URL?

**Option A: Bundled Caddy (production-style)**

1. Set `DOMAIN_NAME` and `ACME_EMAIL` in `.env`.
2. Start app + edge:

```bash
docker compose up -d --build bot db
docker compose --profile edge up -d --build caddy
```

3. Use `https://$DOMAIN_NAME` as `MINI_APP_URL`.

**Option B: Your own reverse proxy**

Point TLS at the bot’s HTTP port (default host mapping `127.0.0.1:8080` → container `8080`). Keep `HEALTH_CHECK_PORT` aligned with compose/Caddy if you change it.

**Option C: Tunnel for testing** (Cloudflare Tunnel / ngrok)

Expose local `8080` over HTTPS and set `MINI_APP_URL` to the tunnel URL. URLs may change on restart.

### Step 3: Restart the bot

```bash
docker compose up -d --build bot db
```

Or use `./setup.sh` maintenance options.

### Step 4: Configure BotFather

1. Open [@BotFather](https://t.me/BotFather)
2. `/mybots` → your bot → **Bot Settings** → **Menu Button**
3. Set the URL to the same origin as `MINI_APP_URL` (e.g. `https://your-domain.com`)

---

## How It Works

```
Telegram client
  └── Mini App (React SPA from web-app/dist)
        │ HTTPS same origin
        ▼
Go backend
  ├── /api/*   JSON API (auth via Telegram initData)
  └── /        static SPA + fallback
```

High-level API surfaces used by the Mini App:

| Area | Examples |
|---|---|
| Session / home | `/api/me`, keys, auto-renew |
| Catalog / checkout | `/api/plans`, `/api/purchase`, `/api/upload_screenshot`, `/api/promo/validate` |
| Wallet | `/api/wallet`, `/api/wallet/history` |
| Admin (authorized) | plan/promo admin routes |

### Security

- Auth uses Telegram `initData` (HMAC validated with `TELEGRAM_TOKEN`).
- No separate password login.
- Purchase creation should send a unique `Idempotency-Key` header (the bundled Mini App does this).

---

## Troubleshooting

| Problem | Solution |
|---|---|
| "Node.js not found" / wrong version | Install **Node 20** |
| `npm ci` / build failed | Delete `web-app/node_modules` and retry; check Node version |
| Mini App blank | Public HTTPS, `MINI_APP_URL` match, BotFather URL; see `HOWTOUSE.md` |
| "Unauthorized" | `TELEGRAM_TOKEN` must match BotFather |
| Menu button missing | Configure BotFather menu button (Step 4) |
| Bundled Caddy not starting | Use `docker compose --profile edge up -d caddy` |

---

## Development (Optional)

```bash
cd web-app
npm ci
npm run dev
```

Dev server: `http://localhost:5173` (API proxied to the bot, typically `http://localhost:8080`).

> Full auth only works inside Telegram (`initData`). Local UI work is fine; production Mini App is the built `dist` served by Go.

---

## File Structure

```
web-app/
├── src/
│   ├── App.tsx
│   ├── main.tsx
│   ├── index.css
│   ├── pages/           # Home, Plans, Checkout, Wallet, admin pages, …
│   ├── components/
│   └── lib/             # auth, http, translations, Telegram helpers, …
├── package.json
├── vite.config.ts
└── dist/                # after `npm run build` (served by Go)
```

---

## Admin Finance

- Route: `/admin/finance` (admin session required).
- Data source: `GET /api/revenue` returns a structured `FinanceReport` (not raw purchase rows).
- CSV: `GET /api/revenue/export` with the same query params; totals match JSON.
- Timezone: all period boundaries are `Asia/Yangon`.
- Metrics:
  - Gross service revenue: paid plan purchases (includes wallet spend; excludes wallet top-ups)
  - Refunds: `financial_adjustment` rows with `adjustment_type=refund` on effective date
  - Net Income: gross − refunds
  - Cash collected: external money including wallet top-ups
- The browser never aggregates money; it only renders server values.
- Trend chart is pure SVG (no chart library).
