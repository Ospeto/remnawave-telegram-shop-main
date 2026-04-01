# Bot Backup Runbook

This deployment supports bot-managed PostgreSQL backup operations. Live restore execution through the running bot is disabled for safety.

## Runtime Requirements

- The bot image includes PostgreSQL client tools (`pg_dump`, `psql`) and `gzip`.
- Docker Compose mounts a persistent backup volume at `/backups` inside the bot container.
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

## Operational Notes

- Backup artifacts are DB-only and stored in `/backups`.
- Telegram delivery is convenience distribution, not the only retention layer.
- Restore should be treated as destructive even with pre-restore safety backup and must be performed offline/manual while the app is stopped.
- If Telegram upload fails, verify whether the backup file still exists locally and inspect bot logs.

## Quick Checks

Inspect bot logs:

```bash
docker compose logs -f bot
```

Inspect backup volume from a temporary container:

```bash
docker run --rm -v bot_backups:/backups alpine:3.22 ls -lah /backups
```
