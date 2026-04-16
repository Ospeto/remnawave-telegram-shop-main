# Project Structure

This document provides a detailed overview of the codebase organization for the Remnawave Telegram Shop Bot.

## Overview

The project follows a standard Go project layout combined with a modern frontend structure.

### Root Directory

| File/Dir | Description |
| :--- | :--- |
| `cmd/` | Main application entry points. |
| `internal/` | Private application code (library code not importable by other projects). |
| `web-app/` | The React frontend for the Telegram Mini App. |
| `db/` | Database migration files (`.sql`). |
| `docker-compose.yaml` | Defines services (bot, database, proxy) for local and prod deployment. |
| `Dockerfile` | Multi-stage build instructions for compiling Go and React. |
| `setup.sh` | Interactive CLI tool for installation, management, backup, and restore. |
| `go.mod` / `go.sum` | Go dependency definitions. |

---

## Backend (`cmd/`, `internal/`)

The backend is built with Go and follows a clean service-repository pattern.

### `cmd/app/`
-   **`main.go`**: The application bootstrapper. It initializes the database connection, runs migrations, sets up the Telegram bot webhook/polling, and starts the HTTP server.

### `internal/`

#### `config/`
-   Loads environment variables from `.env`.
-   Parses configuration strings (e.g., plan definitions).
-   Provides global access to config values safely.

#### `database/` (Repositories)
Handles all PostgreSQL interactions. Uses `pgx` driver and `squirrel` for query building.
-   **`persistance.go`**: Connection pool and migration runner.
-   **`purchase.go`**: `PurchaseRepository` for managing orders.
-   **`customer.go`**: `CustomerRepository` for managing user profiles and balances.
-   **`key.go`**: `SubscriptionKeyRepository` for keeping track of issued keys.
-   **`mobile_payment.go`**: Handles manual verification records for bank transfers.

#### `handler/` (Controllers)
Handles Telegram updates (messages, callbacks) and HTTP requests.
-   **`handler.go`**: Main `Handler` struct and routing logic.
-   **`start.go`**: `/start` command capabilities.
-   **`payment_handlers.go`**: Logic for buying plans, callbacks from CryptoPay, and mobile payment screenshots.
-   **`admin.go`**: Admin help plus the legacy slash-command handlers used behind the admin dashboard.
-   **`admin_dashboard.go`**: `/admin` dashboard, callback routing, confirmations, and guided admin flows.
-   **`admin_registry.go`**: Source-of-truth command metadata for visible bot commands and hidden admin fallbacks.

#### `service/` (Business Logic)
Contains high-level business rules, decoupled from the transport layer (Telegram/HTTP).
-   **`autorenew/`**: Background service for verifying keys and managing expirations.
-   **`invoicechecker/`**: Polling service for CryptoPay status updates.

#### `payment/`
-   **`payment.go`**: `PaymentService`. Orchestrates the complex flow of creating a purchase -> verification -> issuing key -> notifying user. Handles the "Test Mode" and duplicate check logic.

#### `remnawave/`
-   **`client.go`**: API Client for the Remnawave Panel. Handles key generation, user creation, and traffic limit updates.

#### `gemini/`
-   **`client.go`**: Client for Google Gemini API. utilized for analyzing screenshots in mobile payment verification.

---

## Frontend (`web-app/`)

The Mini App is a Single Page Application (SPA) built with React.

### Key Technologies
-   **Vite**: Build tool and dev server.
-   **React**: UI library.
-   **TypeScript**: Type safety.
-   **TailwindCSS**: Utility-first styling.
-   **shadcn/ui**: Reusable UI components.

### Structure
-   **`src/main.tsx`**: Entry point.
-   **`src/App.tsx`**: Main router and layout.
-   **`src/pages/`**: Individual screens (Home, Wallet, Profile).
-   **`src/components/`**: Shared UI elements (Buttons, Cards, Inputs).
-   **`src/lib/`**: Utilities (Telegram WebApp SDK wrapper, API clients, translations).

---

## Database (`db/`)

Managed by `golang-migrate`.
-   **`migrations/`**: Ordered SQL files (`000001_init.up.sql`, etc.) defining the schema evolution.
-   **Key Constraint Fixes**: Migrations 15, 16, 17 address specific crash loop issues encountered during development.

---

## Infrastructure

-   **`fix_db_crash.sh`**: Emergency script to reset dirty migration states.
-   **`Caddyfile`** (Generated/Used): Configures the Caddy reverse proxy for automatic HTTPS.
