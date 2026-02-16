# Codebase Structure

This document outlines the architectural organization of the Remnawave Telegram Shop.

## Directory Layout

### 1. Root Level
- **`setup.sh`**: The interactive installation and management wizard. This is the **primary entry point** for users to install, configure, update, and manage the bot.
- **`cmd/app/`**: Contains the `main.go` entry point for the Go backend.
- **`web-app/`**: The frontend source code (React/TypeScript/Vite) for the Telegram Mini App shop.
- **`db/migrations/`**: SQL migration files (`up` and `down`) managed by `golang-migrate`.
- **`translations/`**: Backend JSON translation files (`en`, `ru`, `my`) for Telegram bot messages.
- **`docs/`**: Documentation files.

### 2. Backend (`internal/`)
The Go backend logic is modularized within the `internal` directory:

| Package | Description |
| :--- | :--- |
| **`api`** | HTTP server handlers for the Mini App API (e.g., `/api/plans`, `/api/purchase`). |
| **`bot`** | Telegram Bot API integration (handlers, keyboards, callback processing). |
| **`config`** | Configuration loading from environment variables. |
| **`database`** | PostgreSQL repositories and query logic (using `pgx` and `squirrel`). |
| **`handler`** | Telegram bot command/callback logic (Buy, Profile, Support, etc.). |
| **`payment`** | Payment processing logic (CryptoPay, Mobile Banking, YooKassa). Includes verification flows. |
| **`remnawave`** | Client for the Remnawave Panel API (User/Key management). |
| **`gemini`** | Integration with Google Gemini for analyzing mobile banking screenshots. |
| **`notification`**| Expiry notification system. |
| **`translation`** | i18n manager for backend text. |

### 3. Frontend (`web-app/`)
A React single-page application built with Vite, designed to run inside Telegram Web Apps.

- **`src/main.tsx`**: Entry point.
- **`src/App.tsx`**: Main router and auth wrapper.
- **`src/pages/`**:
  - `Home.tsx`: Dashboard with plans and active keys.
  - `Checkout.tsx`: Payment flow (Method selection -> Payment -> Verification).
- **`src/lib/`**: Utilities, including `translations.ts` (Frontend-specific localization).

## Data Flow

1.  **User Interaction**: User opens Mini App in Telegram.
2.  **Frontend**: React app fetches plans from Backend (`/api/plans`).
3.  **Purchase**:
    -   User selects plan -> Checkout.
    -   Frontend sends request to Backend (`/api/create_purchase`).
    -   Backend creates "Pending" purchase in DB.
4.  **Verification**:
    -   **Crypto**: Handled via webhooks from CryptoPay.
    -   **Mobile Banking**: User uploads screenshot -> Backend sends to Gemini AI -> If valid, purchase marked "Completed".
5.  **Fulfillment**:
    -   On completion, Backend calls Remnawave API to create/extend user.
    -   Backend saves new Key info to local DB.
    -   Frontend polls status and displays "Success".

## Deployment Architecture

- **Docker Compose**: Orchestrates services.
    -   `bot`: The Go binary. Also serves the built frontend static files.
    -   `caddy`: Reverse proxy (HTTPS/SSL).
    -   `db`: PostgreSQL database.
-   **Build Process**:
    -   `Dockerfile` has a multi-stage build.
    -   Stage 1: Build Frontend (`npm run build`).
    -   Stage 2: Build Backend (`go build`).
    -   Stage 3: Copy artifacts to final scratch image.
    -   **CRITICAL**: Frontend is baked into the image. Updates require a full rebuild (`docker compose build --no-cache`).
