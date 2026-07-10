# Bot Backup Runbook

This deployment supports bot-managed PostgreSQL backup operations. Live restore execution through the running bot is disabled for safety.

There are **two** offline-restorable artifact types. Do not mix them up:

| Artifact | Produced by | Location | Restored via `setup.sh` option 11 |
|----------|-------------|----------|-----------------------------------|
| **Full bundle** `backup_*.tar.gz` | `setup.sh` option 10 | Host `./backups/` | Yes — DB + optional `.env`, translations, Caddy certs |
| **Bot DB-only** `db_*.sql.gz` | Bot `/backup now` or schedule | Container `/backups` on volume `bot_backups` | Yes — **database only** (after copy to host `./backups/`) |

Live runtime restore through the bot remains disabled. Use `/restore list` only to identify a filename, then restore offline with `setup.sh`.

## Runtime Requirements

- The bot image includes PostgreSQL client tools (`pg_dump`, `psql`) and `gzip`.
- Docker Compose mounts a persistent backup volume at `/backups` inside the bot container (`bot_backups` volume).
- Rebuild the bot image after pulling runtime changes:

```bash
docker compose up -d --build bot
```

## Recommended Environment Variables

Add these values to `.env` as needed:

```dotenv
BACKUP_ENABLED=true
BACKUP_SCHEDULE_CRON=10 0 * * *
BACKUP_TIMEZONE=Asia/Rangoon
BACKUP_DIR=/backups
BACKUP_RETENTION_DAYS=7
BACKUP_MAX_LOCAL_FILES=7
BACKUP_SEND_TO_TELEGRAM=true
BACKUP_RESTORE_ENABLED=false
BACKUP_CONFIRM_TTL_MINUTES=10
BACKUP_JOB_TIMEOUT_SECONDS=900
BACKUP_RESTORE_TIMEOUT_SECONDS=1800
```

`BACKUP_RESTORE_ENABLED` may enable restore-related config, but **live restore in the running bot stays disabled**. Offline restore is always via `setup.sh`.

## Admin Commands

- `/backup now`
- `/backup status`
- `/backup list`
- `/backup enable`
- `/backup disable`
- `/backup schedule`
- `/backup schedule HH:MM`
- `/restore list`
- `/restore latest`, `/restore file <name>`, `/restore confirm <token>`, `/restore cancel` return guidance to use an offline/manual restore workflow while the app is stopped.

## Offline restore (recommended)

### A) Full system bundle (`backup_*.tar.gz`)

1. Create with `./setup.sh` → option **10 (Backup)**.
2. File lands in host `./backups/backup_YYYYMMDD_HHMMSS.tar.gz`.
3. On the target host, place the file under `./backups/`.
4. Run `./setup.sh` → option **11 (Restore)** and select the full bundle.
5. Script stops services, restores DB (and `.env`/translations/certs when present), restarts bot+db.

### B) Bot DB-only dump (`db_*.sql.gz`)

1. Create with `/admin` → Backups → Run Backup Now, or `/backup now`.
2. Artifact is `db_YYYYMMDD_HHMMSS.sql.gz` on the `bot_backups` volume (path `/backups` in the bot container). It is **not** a `setup.sh` tar bundle and does **not** include `.env` or certs.
3. Copy the dump onto the host `./backups/` directory (examples):

```bash
# List volume contents
docker run --rm -v bot_backups:/backups alpine:3.22 ls -lah /backups

# Copy all bot dumps into host ./backups (from repo root)
mkdir -p backups
docker run --rm \
  -v bot_backups:/from \
  -v "$(pwd)/backups:/to" \
  alpine:3.22 sh -c 'cp -v /from/db_*.sql.gz /to/ 2>/dev/null || echo "no db_*.sql.gz found"'
```

4. Ensure a valid `.env` already exists on the target host (bot dumps do not restore config).
5. Run `./setup.sh` → option **11 (Restore)** and select the `db_*.sql.gz` entry (labeled **bot DB-only**).
6. Script stops services, gunzips SQL into Postgres via the offline compose flow, restarts bot+db.
7. `.env`, translations, and Caddy data are **not** modified by this path.

### Integrity check (bot dumps)

```bash
docker run --rm -v bot_backups:/backups alpine:3.22 sh -lc \
  'apk add --no-cache gzip >/dev/null; for f in /backups/db_*.sql.gz; do gzip -t "$f" || exit 1; done; echo OK'
```

## Operational Notes

- Bot backup artifacts are **DB-only** (`db_*.sql.gz`) and stored under `/backups` on `bot_backups`.
- Full disaster recovery / VPS migration should prefer a **full** `setup.sh` bundle (or bot dump **plus** separate `.env`/cert handling). See `docs/PRODUCTION_MIGRATION_RUNBOOK.md`.
- Telegram delivery is convenience distribution, not the only retention layer.
- Restore is destructive; always perform it offline/manual while the app is stopped via `setup.sh`.
- If Telegram upload fails, verify whether the backup file still exists on the volume and inspect bot logs.

## Quick Checks

Inspect bot logs:

```bash
docker compose logs -f bot
```

Inspect backup volume from a temporary container:

```bash
docker run --rm -v bot_backups:/backups alpine:3.22 ls -lah /backups
```

Non-destructive script contract (classification only; no restore):

```bash
bash scripts/test-backup-restore-contract.sh
bash -n setup.sh
```
