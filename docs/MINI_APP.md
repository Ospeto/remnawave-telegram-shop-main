# 📱 Telegram Mini App — Setup Guide

This guide walks you through setting up the **Telegram Mini App** for your Remnawave Shop bot. The Mini App lets users manage their subscription directly inside Telegram.

---

## What is the Mini App?

The Mini App is a small web application that runs **inside Telegram** when users tap the Menu Button or the "Connect" button. It shows:

- ✅ **Subscription status** (active/expired, expiry date)
- 🚀 **One-click "Import to Happ Proxy"** button
- 📊 User info (username, avatar)

---

## Prerequisites

| Requirement | How to check | How to install |
|---|---|---|
| **Node.js 18+** | `node -v` | [nodejs.org](https://nodejs.org) or `brew install node` (macOS) |
| **npm** | `npm -v` | Comes with Node.js |
| **Bot already set up** | `.env` file exists | Run `./setup.sh` → Option 1 (Fresh Install) |
| **Public HTTPS URL** | Your server has a domain | Use a reverse proxy (nginx/Caddy) with SSL |

---

## Quick Setup (Recommended)

The easiest way is to use the setup script:

```bash
./setup.sh
```

Choose **Option 9 → 📱 Setup Mini App**

The script will:
1. ✅ Check Node.js is installed
2. 📦 Install frontend dependencies (`npm install`)
3. 🔨 Build the Mini App (`npm run build`)
4. ⚙️ Ask for your public HTTPS URL and save it to `.env`
5. 📋 Show next steps

---

## Manual Setup

If you prefer to set things up manually:

### Step 1: Install Dependencies

```bash
cd web-app
npm install
```

### Step 2: Build

```bash
npm run build
```

This creates `web-app/dist/` with the compiled frontend.

### Step 3: Configure `.env`

Add or update this line in your `.env` file:

```env
MINI_APP_URL=https://your-domain.com
```

> ⚠️ **Must be HTTPS**. Telegram requires a secure URL for Mini Apps.

#### ❓ Where do I get a Public URL?

**Option A: Cloudflare Tunnel (Recommended & Free)**
Easiest way to expose your local bot securely.
1. Install `cloudflared`.
2. Run: `cloudflared tunnel --url http://localhost:8080`
3. Copy the `https://....trycloudflare.com` URL.

**Option B: Ngrok (Testing)**
Good for quick testing, but URLs change on restart.
1. Install `ngrok`.
2. Run: `ngrok http 8080`
3. Copy the `https://....ngrok-free.app` URL.

**Option C: VPS + Domain (Production)**
If you have a VPS (DigitalOcean, Hetzner) and a domain:
1. Point your domain A record to your VPS IP.
2. Use a reverse proxy (Caddy/Nginx) to handle SSL.
3. Your URL is `https://your-domain.com`.

### Step 4: Restart the Bot

```bash
docker-compose up -d --build
```

Or use `./setup.sh` → Option 4.

### Step 5: Configure BotFather

1. Open [@BotFather](https://t.me/BotFather) in Telegram
2. Send `/mybots`
3. Select your bot
4. Go to **Bot Settings** → **Menu Button**
5. Choose **Configure menu button**
6. Send your URL (e.g., `https://your-domain.com`)

---

## How It Works

```
┌─────────────────────────────┐
│     Telegram (Mobile)       │
│  ┌───────────────────────┐  │
│  │   Mini App (React)    │  │
│  │                       │  │
│  │  📊 Subscription      │  │
│  │  ● Active             │  │
│  │  Expires: 2026-03-15  │  │
│  │                       │  │
│  │  [🚀 Import to        │  │
│  │     Happ Proxy]       │  │
│  │                       │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
         │ API calls (HTTPS)
         ▼
┌─────────────────────────────┐
│  Go Backend (your server)   │
│  ├── /api/me    (auth)      │
│  ├── /api/plans (public)    │
│  └── /         (static)     │
└─────────────────────────────┘
```

### Security

- The Mini App authenticates using Telegram's `initData` — a signed payload that proves the user is who they say they are.
- The backend validates the HMAC-SHA256 hash using your `TELEGRAM_TOKEN`.
- No passwords or separate login needed!

---

## Troubleshooting

| Problem | Solution |
|---|---|
| "Node.js not found" | Install Node.js 18+ from [nodejs.org](https://nodejs.org) |
| "npm install failed" | Delete `web-app/node_modules` and retry |
| "Build failed" | Check the error message; usually a missing dependency |
| Mini App shows blank | Check browser console (F12). Likely API URL mismatch |
| "Unauthorized" in Mini App | Make sure `TELEGRAM_TOKEN` in `.env` matches BotFather |
| Menu Button not showing | Configure it in BotFather (Step 5 above) |

---

## Development (Optional)

For local development:

```bash
cd web-app
npm run dev
```

This starts a dev server on `http://localhost:5173` with hot reload. API calls are proxied to `http://localhost:8080` (your bot's health check port).

> **Note**: The Mini App won't fully work locally because Telegram's `initData` is only available when opened inside Telegram. You can test the UI, but authentication won't work.

---

## File Structure

```
web-app/
├── src/
│   ├── App.tsx          # Main component (dashboard + import button)
│   ├── main.tsx         # Entry point
│   ├── index.css        # Styles (Tailwind CSS)
│   ├── types.d.ts       # Telegram WebApp type definitions
│   └── lib/
│       └── twa.ts       # Telegram WebApp hook (useTelegram)
├── package.json         # Dependencies
├── vite.config.ts       # Build configuration
├── tailwind.config.js   # Tailwind CSS configuration
└── dist/                # Built files (after `npm run build`)
```
