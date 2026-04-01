## 1. Runtime And Config Foundations

- [x] 1.1 Add backup-related config keys and defaults in `internal/config/config.go` (enable flag, timezone, schedule, backup directory, retention, restore enable, confirm TTL, job timeouts).
- [x] 1.2 Update `Dockerfile` runtime stage from `scratch` to minimal image including `pg_dump`, `psql` (or `pg_restore`), and `gzip`.
- [x] 1.3 Update compose configuration to mount a persistent backup directory volume to bot service.

## 2. Backup Service Core

- [x] 2.1 Create backup service package under `internal/service/` for dump, compression, metadata formatting, and retention pruning.
- [x] 2.2 Implement single-flight operation lock shared by backup and restore paths.
- [x] 2.3 Implement local backup listing and status inspection helpers for admin commands.

## 3. Admin Command Surface

- [x] 3.1 Add `/backup` command handlers for `now`, `status`, `list`, `enable`, `disable`, and schedule display/update.
- [x] 3.2 Add `/restore` command handlers for `list`, `latest`/`file`, `confirm`, and `cancel`.
- [x] 3.3 Register new handlers in `cmd/app/main.go` and update `/help` command text in `internal/handler/admin.go`.

## 4. Scheduled Daily Backup

- [x] 4.1 Wire daily backup cron job in `cmd/app/main.go` using configured timezone and schedule.
- [x] 4.2 Ensure scheduled job sends successful backup document to `ADMIN_TELEGRAM_ID`.
- [x] 4.3 Ensure scheduled job sends explicit failure alert when backup generation or delivery fails.

## 5. Restore Safety Workflow

- [x] 5.1 Implement short-lived restore confirmation token flow for destructive restore operations.
- [x] 5.2 Implement pre-restore safety backup creation before restore execution.
- [x] 5.3 Implement restore execution from local backup artifacts only, with clear success/failure reporting.

## 6. Validation And Operational Readiness

- [x] 6.1 Add unit tests for command authorization, token validation, retention pruning, and concurrency guard behavior.
- [x] 6.2 Add integration-style tests for backup success path and failure notifications.
- [x] 6.3 Update documentation for backup/restore commands, configuration keys, and operator runbook guidance.
