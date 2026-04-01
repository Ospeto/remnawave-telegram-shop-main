## Why

Backups currently depend on host-side `setup.sh` usage, so operators must SSH into the server and run manual commands. The bot already owns admin controls and scheduled jobs, so database backup and restore should be operable from Telegram with daily automated delivery.

## What Changes

- Add admin-only bot commands for backup and restore operations (`/backup` and `/restore` command groups).
- Add daily automatic PostgreSQL backup creation and Telegram delivery to `ADMIN_TELEGRAM_ID`.
- Add local backup retention management (prune by age/count).
- Add guarded restore flow with explicit confirmation token and pre-restore safety backup.
- Add runtime/config support for backup scheduling and storage.
- **BREAKING**: change bot runtime packaging away from `scratch` so backup/restore binaries (`pg_dump`, `psql`, `gzip`) are available.

## Capabilities

### New Capabilities

- `bot-db-backup-restore`: Admin-controlled DB backup/restore via Telegram, including scheduled backups, local retention, and restore safety controls.

### Modified Capabilities

- None.

## Impact

- Bot command routing and admin UX in `cmd/app/main.go` and `internal/handler/admin.go`.
- New backup domain/service components under `internal/service/` and related wiring.
- Configuration parsing in `internal/config/config.go` and optional dynamic settings in `app_config`.
- Container runtime and deployment in `Dockerfile` and compose configuration.
- Operational behavior for backups, restore safety, and alerting.
