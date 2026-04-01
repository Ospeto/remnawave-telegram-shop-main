## Context

The repository already contains host-driven backup/restore logic in `setup.sh`, plus bot admin command handling and cron scheduling in Go. The current final bot image is `scratch`, which cannot execute `pg_dump` or `psql`, so bot-driven DB backup/restore is not possible without runtime changes.

This design introduces DB-only backup/restore through the Telegram admin bot, with daily automated delivery and operator safety controls.

## Goals / Non-Goals

**Goals:**
- Provide admin-only Telegram commands to run DB backup and restore operations.
- Run daily automated PostgreSQL backup in Myanmar time and deliver it to admin chat.
- Persist backups locally with retention pruning.
- Require two-step confirmation for destructive restore actions.
- Create a safety backup immediately before restore.
- Keep failure reporting explicit to admin and logs.

**Non-Goals:**
- Full-system backup parity with `setup.sh` (Caddy certs, `.env`, translations).
- Point-in-time recovery (WAL archiving).
- Non-admin restore flows.
- Multi-party approval workflows.
- Cloud/offsite backup as part of first release.

## Decisions

### 1) In-process backup control plane inside the bot

The bot already owns admin command authorization and operator messaging, so backup/restore orchestration stays in-process rather than adding a separate scheduler service.

Alternative considered:
- Sidecar/worker container for backup execution.
  - Pros: stronger separation.
  - Cons: more moving parts, split command/operation ownership.

### 2) Runtime packaging must support postgres client binaries

Backup and restore operations require `pg_dump`, `psql` (or `pg_restore`), and compression tools. The final image will move from `scratch` to a minimal image that includes these tools.

Alternative considered:
- Keep `scratch` and execute backup through host Docker CLI.
  - Rejected because it reintroduces host-coupled operations and violates bot-driven requirement.

### 3) DB-only backup artifact format

The backup artifact is `db_YYYYMMDD_HHMMSS.sql.gz` stored in a dedicated backup directory (for example `/backups`). This keeps v1 scope tightly focused and reduces accidental secret sprawl.

### 4) Schedule and runtime settings

Schedule defaults are env-driven, with optional runtime override support in `app_config` for admin UX. Default schedule target: `00:10` in `Asia/Rangoon`, avoiding collision with existing midnight reporting jobs.

### 5) Restore safety protocol

Restore is a two-step flow:
1. `/restore latest` or `/restore file <name>` generates a short-lived confirmation token.
2. `/restore confirm <token>` executes restore.

Before restore starts, the system creates a fresh pre-restore safety backup.

### 6) Single-flight operation lock

Backup and restore operations share a mutual exclusion guard so concurrent backup/restore execution is blocked.

### 7) Failure and delivery behavior

Every run reports explicit success/failure to admin. If Telegram document delivery fails (for example size limits), the local backup remains preserved and admin receives failure context.

## Risks / Trade-offs

- [Telegram file-size limits] -> Keep local backups as source of truth; report upload failures clearly.
- [Runtime image growth from `scratch` to minimal Linux image] -> Use smallest feasible base and keep package set minimal.
- [Restore is destructive] -> Enforce two-step confirmation token and pre-restore safety backup.
- [Schedule drift/timezone confusion] -> Use explicit `Asia/Rangoon` timezone config, not host-local defaults.
- [Disk growth from backup artifacts] -> Enforce retention by age and count.

## Migration Plan

1. Introduce config keys and defaults for backup enablement, schedule, timezone, storage, and retention.
2. Add runtime image/deployment updates so backup tools are available at runtime and backup path is mounted.
3. Add backup service and admin command handlers for manual backup, status/list, and restore confirmation flow.
4. Add scheduled daily backup job wiring in bot startup using existing cron patterns.
5. Roll out with backups enabled and restore command gated behind explicit enable flag.
6. Validate restore flow in staging with synthetic data before production use.

Rollback:
- Disable backup scheduler and restore command via config flags.
- Revert image/runtime change and command wiring if operational regressions occur.

## Open Questions

- Should backup schedule edits from bot commands be persisted in `app_config`, env-only, or both?
- Do we require backup encryption at rest/transit in v1 or defer to next phase?
- What maximum backup size threshold should trigger alternate delivery behavior?
