#!/usr/bin/env bash
# Non-destructive contract tests for setup.sh backup artifact classification.
# Does not run restore, docker, or DB writes.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SETUP="${ROOT}/setup.sh"

if [[ ! -f "$SETUP" ]]; then
  echo "FAIL: setup.sh not found at $SETUP" >&2
  exit 1
fi

# Source only the pure helpers by extracting/evaluating them in a subshell-safe way:
# define the same function bodies expected by setup.sh (must stay in sync with setup.sh).
# Prefer sourcing via bash -c that defines SCRIPT_DIR and loads helpers by parsing.

# shellcheck disable=SC1090
# Extract helper functions from setup.sh into a temp snippet and source them.
HELPERS="$(mktemp)"
trap 'rm -f "$HELPERS"' EXIT

awk '
  /^is_bot_db_backup_file\(\)/ { capture=1 }
  /^is_full_backup_bundle_file\(\)/ { capture=1 }
  /^collect_restorable_backups\(\)/ { capture=1 }
  /^backup_artifact_label\(\)/ { capture=1 }
  capture { print }
  /^}$/ && capture {
    # end of a top-level function body
    # count braces roughly: when we print a lone }, stop if next line is blank or comment/function
    # simpler: stop after first closing brace at column 0
    if ($0 == "}") { capture=0; print "" }
  }
' "$SETUP" > "$HELPERS"

# Fallback if awk extraction is empty
if [[ ! -s "$HELPERS" ]]; then
  echo "FAIL: could not extract helpers from setup.sh" >&2
  exit 1
fi

# shellcheck disable=SC1090
source "$HELPERS"

fail=0
assert_eq() {
  local got="$1" want="$2" msg="$3"
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: $msg (got='$got' want='$want')"
    fail=1
  else
    echo "OK: $msg"
  fi
}

assert_true() {
  local msg="$1"
  shift
  if "$@"; then
    echo "OK: $msg"
  else
    echo "FAIL: $msg"
    fail=1
  fi
}

assert_false() {
  local msg="$1"
  shift
  if "$@"; then
    echo "FAIL: $msg (expected false)"
    fail=1
  else
    echo "OK: $msg"
  fi
}

# --- Classification contract (D4) ---
assert_true "bot dump recognized" is_bot_db_backup_file "/backups/db_20260710_120000.sql.gz"
assert_true "bot dump basename only" is_bot_db_backup_file "db_20260101_000000.sql.gz"
assert_false "full bundle is not bot dump" is_bot_db_backup_file "/backups/backup_20260710_120000.tar.gz"
assert_false "plain sql not bot dump" is_bot_db_backup_file "db_dump.sql"
assert_false "wrong prefix not bot dump" is_bot_db_backup_file "backup_db_1.sql.gz"
assert_false "ungzipped not bot dump" is_bot_db_backup_file "db_20260710.sql"

assert_true "setup full bundle recognized" is_full_backup_bundle_file "/x/backup_20260710_120000.tar.gz"
assert_true "generic tar.gz recognized" is_full_backup_bundle_file "migrate_old.tar.gz"
assert_false "caddy nested archive alone not full bundle" is_full_backup_bundle_file "caddy_data.tar.gz"
assert_false "bot dump is not full bundle" is_full_backup_bundle_file "db_20260710_120000.sql.gz"

assert_eq "$(backup_artifact_label "db_1.sql.gz")" "bot DB-only" "label bot"
assert_eq "$(backup_artifact_label "backup_1.tar.gz")" "full bundle" "label full"

# --- collect_restorable_backups ordering ---
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"; rm -f "$HELPERS"' EXIT
touch "$tmpdir/backup_20260101_000000.tar.gz"
touch "$tmpdir/db_20260102_000000.sql.gz"
touch "$tmpdir/db_20260103_000000.sql.gz"
touch "$tmpdir/caddy_data.tar.gz"   # must be ignored
touch "$tmpdir/notes.txt"           # must be ignored
touch "$tmpdir/extra_migrate.tar.gz"

mapfile -t collected < <(collect_restorable_backups "$tmpdir")
# Expected order: backup_*.tar.gz, then other *.tar.gz (not caddy), then db_*.sql.gz
got_names=()
for p in "${collected[@]}"; do
  got_names+=("$(basename "$p")")
done

assert_eq "${#got_names[@]}" "4" "collect count (2 tar + 2 bot; caddy/notes excluded)"

# first should be setup full bundle
assert_eq "${got_names[0]}" "backup_20260101_000000.tar.gz" "full bundle first"

# bot dumps present
joined="$(IFS=,; echo "${got_names[*]}")"
if [[ "$joined" == *db_20260102_000000.sql.gz* && "$joined" == *db_20260103_000000.sql.gz* ]]; then
  echo "OK: bot dumps included in collect"
else
  echo "FAIL: bot dumps missing from collect: $joined"
  fail=1
fi
if [[ "$joined" == *caddy_data.tar.gz* ]]; then
  echo "FAIL: caddy_data.tar.gz should not be listed"
  fail=1
else
  echo "OK: caddy_data.tar.gz excluded"
fi
if [[ "$joined" == *extra_migrate.tar.gz* ]]; then
  echo "OK: generic tar.gz included"
else
  echo "FAIL: extra_migrate.tar.gz missing"
  fail=1
fi

# --- setup.sh must wire bot path (static contract) ---
if grep -q 'is_bot_db_backup_file' "$SETUP" \
  && grep -q 'restore_bot_db_backup' "$SETUP" \
  && grep -q 'collect_restorable_backups' "$SETUP" \
  && grep -q 'gzip -dc' "$SETUP" \
  && grep -q 'restore_temp_bot_db.sql' "$SETUP"; then
  echo "OK: setup.sh wires bot db_*.sql.gz restore path"
else
  echo "FAIL: setup.sh missing bot restore wiring"
  fail=1
fi

if grep -q 'db_\*\.sql\.gz' "$SETUP" || grep -q 'db_*.sql.gz' "$SETUP"; then
  echo "OK: setup.sh references db_*.sql.gz pattern"
else
  echo "FAIL: setup.sh does not reference db_*.sql.gz"
  fail=1
fi

# --- restore_sql_into_db fail-closed import contract ---
# Offline restore must stop on SQL errors (ON_ERROR_STOP) and not report success
# after a partial import. Both full-bundle and bot-db paths share this importer.
RESTORE_SQL_FN="$(mktemp)"
trap 'rm -rf "$tmpdir"; rm -f "$HELPERS" "$RESTORE_SQL_FN"' EXIT

awk '
  /^restore_sql_into_db\(\)/ { capture=1 }
  capture { print }
  /^}$/ && capture {
    if ($0 == "}") { capture=0 }
  }
' "$SETUP" > "$RESTORE_SQL_FN"

if [[ ! -s "$RESTORE_SQL_FN" ]]; then
  echo "FAIL: could not extract restore_sql_into_db() from setup.sh"
  fail=1
else
  if grep -q 'ON_ERROR_STOP=1' "$RESTORE_SQL_FN"; then
    echo "OK: restore_sql_into_db uses ON_ERROR_STOP=1"
  else
    echo "FAIL: restore_sql_into_db missing ON_ERROR_STOP=1"
    fail=1
  fi

  # Import may be split across lines (docker exec ... \n psql ...); flatten for match.
  restore_flat="$(tr '\n' ' ' < "$RESTORE_SQL_FN")"
  if [[ "$restore_flat" == *'if !'*psql* ]] \
    && grep -q 'return 1' "$RESTORE_SQL_FN" \
    && grep -q 'print_error' "$RESTORE_SQL_FN"; then
    echo "OK: restore_sql_into_db guards import failure with return 1"
  else
    echo "FAIL: restore_sql_into_db missing if ! ...; then ... return 1; fi import guard"
    fail=1
  fi

  # Success message must appear after the guarded import block.
  if awk '
    /if !/ { guarded=1 }
    guarded && /return 1/ { saw_return=1 }
    saw_return && /print_success "Database restored"/ { found=1 }
    END { exit found ? 0 : 1 }
  ' "$RESTORE_SQL_FN"; then
    echo "OK: print_success Database restored appears after guarded import"
  else
    echo "FAIL: print_success Database restored not after guarded import failure path"
    fail=1
  fi

  if grep -qE 'cat "\$sql_file" \| docker exec' "$RESTORE_SQL_FN"; then
    echo "FAIL: restore_sql_into_db still uses cat|docker exec psql pipeline"
    fail=1
  else
    echo "OK: old cat|docker exec psql pipeline absent from restore_sql_into_db"
  fi
fi

if grep -q 'restore_sql_into_db' "$SETUP" \
  && awk '/^restore_full_backup_bundle\(\)/,/^}$/' "$SETUP" | grep -q 'restore_sql_into_db' \
  && awk '/^restore_bot_db_backup\(\)/,/^}$/' "$SETUP" | grep -q 'restore_sql_into_db'; then
  echo "OK: full bundle and bot db restore paths call restore_sql_into_db"
else
  echo "FAIL: restore_full_backup_bundle / restore_bot_db_backup must call restore_sql_into_db"
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  echo ""
  echo "backup-restore-contract: FAILED"
  exit 1
fi

echo ""
echo "backup-restore-contract: PASSED"
exit 0
