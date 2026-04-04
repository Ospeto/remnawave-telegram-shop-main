#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat <<'EOF'
Usage:
  scripts/check_shared_443_shop.sh [--repo-only]
  scripts/check_shared_443_shop.sh --remote-only --ssh-host <host> [--domain <domain>] [--app-dir <dir>]

Checks the shared-443 VPS deployment invariant used by mini.clonia.xyz:
host Caddy proxies to 127.0.0.1:8080, so the bot container must publish
127.0.0.1:8080:8080 on the host.
EOF
}

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

pass() {
    echo "OK: $*"
}

extract_service_block() {
    local service="$1"
    local file="$2"

    awk -v service="$service" '
        $0 == "  " service ":" { in_service = 1; next }
        in_service && $0 ~ /^  [^[:space:]][^:]*:$/ { exit }
        in_service { print }
    ' "$file"
}

check_repo() {
    local repo_root="$1"
    local compose_file="$repo_root/docker-compose.yaml"
    local bot_block

    [[ -f "$compose_file" ]] || fail "missing $compose_file"

    bot_block="$(extract_service_block bot "$compose_file")"
    [[ -n "$bot_block" ]] || fail "could not find bot service in docker-compose.yaml"

    printf '%s\n' "$bot_block" | grep -Fq "ports:" \
        || fail "bot service does not declare ports"
    printf '%s\n' "$bot_block" | grep -Fq "127.0.0.1:8080:8080" \
        || fail "bot service must publish 127.0.0.1:8080:8080 for host Caddy"
    printf '%s\n' "$bot_block" | grep -Fq '127.0.0.1:$${HEALTH_CHECK_PORT:-8080}/livez' \
        || fail "bot healthcheck must still probe the in-container /livez endpoint"

    pass "repo compose preserves the loopback bot port required by host Caddy"
}

check_remote() {
    local ssh_host="$1"
    local domain="$2"
    local app_dir="$3"

    [[ -n "$ssh_host" ]] || fail "--ssh-host is required for --remote-only"

    ssh "$ssh_host" "grep -Fq 'reverse_proxy 127.0.0.1:8080' /etc/caddy/Caddyfile" \
        || fail "remote Caddyfile is not proxying to 127.0.0.1:8080"
    ssh "$ssh_host" "grep -Fq '127.0.0.1:8080:8080' '$app_dir/docker-compose.yaml'" \
        || fail "remote docker-compose.yaml is missing the loopback 8080 publish"
    ssh "$ssh_host" "ss -tln | grep -Fq '127.0.0.1:8080'" \
        || fail "remote host is not listening on 127.0.0.1:8080"
    ssh "$ssh_host" "curl -fsS http://127.0.0.1:8080/healthcheck >/dev/null" \
        || fail "remote host loopback healthcheck failed"
    curl -fsS "https://$domain/healthcheck" >/dev/null \
        || fail "public healthcheck failed for https://$domain/healthcheck"

    pass "remote shared-443 deployment is reachable on loopback and public HTTPS"
}

mode="repo"
ssh_host=""
domain="mini.clonia.xyz"
app_dir="/opt/remnawave-shop"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo-only)
            mode="repo"
            ;;
        --remote-only)
            mode="remote"
            ;;
        --ssh-host)
            [[ $# -ge 2 ]] || fail "--ssh-host requires a value"
            ssh_host="$2"
            shift
            ;;
        --domain)
            [[ $# -ge 2 ]] || fail "--domain requires a value"
            domain="$2"
            shift
            ;;
        --app-dir)
            [[ $# -ge 2 ]] || fail "--app-dir requires a value"
            app_dir="$2"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            fail "unknown argument: $1"
            ;;
    esac
    shift
done

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

case "$mode" in
    repo)
        check_repo "$repo_root"
        ;;
    remote)
        check_remote "$ssh_host" "$domain" "$app_dir"
        ;;
    *)
        fail "unsupported mode: $mode"
        ;;
esac
