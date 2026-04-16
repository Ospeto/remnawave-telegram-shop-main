# Production Migration Runbook (VPS to VPS)

This runbook is for moving the bot + mini app to a new VPS with minimal user impact.

## Key Reality

- Database backup restore alone is not enough.
- You must migrate:
  - database state
  - `.env`
  - Caddy TLS data (or re-issue certificates)
  - Telegram WebApp URL/menu button
- True zero-downtime is not guaranteed because the bot uses one Telegram token and long polling.

## Pre-Cutover Checklist (T-24h to T-30m)

1. Confirm current production health.
   - `docker compose ps`
   - `curl -fsS https://<mini-app-domain>/`
   - `curl -i https://<mini-app-domain>/api/me` (expect 401 without auth header)
2. Confirm backup scheduler and last successful backup.
   - Telegram admin: `/backup status`, `/backup list`
   - DB check:
     - `select key, value from app_config where key like 'backup_%' order by key;`
3. Create fresh manual backup artifact.
   - Telegram admin: `/backup now`
4. Validate backup integrity on server.
   - `docker run --rm -v <bot_backups_volume>:/backups alpine:3.22 sh -lc "apk add --no-cache gzip >/dev/null; for f in /backups/db_*.sql.gz; do gzip -t \"$f\" || exit 1; done; echo OK"`
5. Create full-system backup bundle.
   - `./setup.sh` -> `Backup`
6. Freeze operational changes during cutover window.
   - avoid price/provider changes, avoid manual DB edits, avoid deploys

## Cutover Steps (Low Impact Path)

1. Prepare new VPS.
   - clone repo
   - copy backup bundle from old VPS
   - restore via `./setup.sh` -> `Restore`
2. Start new stack and validate before traffic switch.
   - `docker compose up -d --build`
   - `docker compose ps`
   - verify mini app URL and API reachability
3. Switch external routing.
   - DNS / reverse proxy / SNI routing to new VPS
4. Update Telegram menu button to new URL.
   - ensure both default and admin-chat override point to new URL
5. Stop old bot polling once new bot is confirmed healthy.
   - prevents token contention and duplicate consumers
6. Post-cutover smoke test.
   - open mini app
   - create a test purchase flow (non-destructive path)
   - run `/admin`, `/backup status`, `/backup list`

## Rollback Plan (If Cutover Fails)

Rollback trigger examples:

- mini app timeout or 5xx persists > 3 minutes
- admin commands fail on new VPS
- purchase or verification flow fails

Rollback steps:

1. Re-point DNS/proxy back to old VPS.
2. Re-set Telegram menu button URL back to old stable URL.
3. Stop bot on new VPS.
4. Confirm old VPS health:
   - `docker compose ps`
   - mini app opens
   - `/backup status` works
5. Announce rollback complete and start incident review.

## Success Criteria

- New VPS serves mini app over HTTPS with valid cert.
- `/api/me` responds as expected (401 without auth).
- Admin commands work (`/admin`, `/backup status`, `/backup list`).
- Latest backup is being generated on new VPS.
- No timeout reports from users in first 15 minutes post-cutover.
