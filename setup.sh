#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────
#  Remnawave Telegram Shop — Interactive Setup Wizard
#  Works on Linux & macOS  •  Requires Docker + Docker Compose
# ─────────────────────────────────────────────────────────────
set -euo pipefail

# ──── Colors & Symbols ──────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m' # No Color

CHECK="${GREEN}✔${NC}"
CROSS="${RED}✘${NC}"
ARROW="${CYAN}➜${NC}"
INFO="${YELLOW}ℹ${NC}"

# ──── Globals ───────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env"
COMPOSE_CMD=""

# ──── Helpers ───────────────────────────────────────────────
print_header() {
    echo ""
    echo -e "${CYAN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}${BOLD}  $1${NC}"
    echo -e "${CYAN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_section() {
    echo ""
    echo -e "  ${MAGENTA}${BOLD}── $1 ──${NC}"
    echo ""
}

print_success() {
    echo -e "  ${CHECK}  $1"
}

print_error() {
    echo -e "  ${CROSS}  ${RED}$1${NC}"
}

print_info() {
    echo -e "  ${INFO}  ${YELLOW}$1${NC}"
}

print_arrow() {
    echo -e "  ${ARROW}  $1"
}

# Ask for a value. Usage: ask "Prompt" "default" "variable_name"
# Stores result in the global associative array CFG.
ask() {
    local prompt="$1"
    local default="$2"
    local varname="$3"

    if [[ -n "$default" ]]; then
        echo -ne "     ${BOLD}${prompt}${NC} ${DIM}[${default}]${NC}: "
    else
        echo -ne "     ${BOLD}${prompt}${NC}: "
    fi

    local input
    read -r input
    input="${input:-$default}"
    CFG["$varname"]="$input"
}

# Ask for a required value (loops until non-empty).
ask_required() {
    local prompt="$1"
    local default="$2"
    local varname="$3"

    while true; do
        ask "$prompt" "$default" "$varname"
        if [[ -n "${CFG[$varname]}" ]]; then
            return
        fi
        print_error "This field is required. Please enter a value."
    done
}

# Ask for a numeric value.
ask_number() {
    local prompt="$1"
    local default="$2"
    local varname="$3"

    while true; do
        ask "$prompt" "$default" "$varname"
        if [[ "${CFG[$varname]}" =~ ^[0-9]+$ ]]; then
            return
        fi
        print_error "Please enter a valid number."
    done
}

# Ask for a true/false toggle.
ask_bool() {
    local prompt="$1"
    local default="$2"
    local varname="$3"

    while true; do
        ask "${prompt} (true/false)" "$default" "$varname"
        local val="${CFG[$varname]}"
        val="$(echo "$val" | tr '[:upper:]' '[:lower:]')"
        if [[ "$val" == "true" || "$val" == "false" ]]; then
            CFG["$varname"]="$val"
            return
        fi
        print_error "Please enter 'true' or 'false'."
    done
}

# Ask for a traffic reset strategy.
ask_reset_strategy() {
    local prompt="$1"
    local default="$2"
    local varname="$3"

    while true; do
        ask "${prompt} (DAY/WEEK/MONTH/NO_RESET)" "$default" "$varname"
        local val="${CFG[$varname]}"
        val="$(echo "$val" | tr '[:lower:]' '[:upper:]')"
        if [[ "$val" == "DAY" || "$val" == "WEEK" || "$val" == "MONTH" || "$val" == "NO_RESET" ]]; then
            CFG["$varname"]="$val"
            return
        fi
        print_error "Allowed values: DAY, WEEK, MONTH, NO_RESET"
    done
}

# ──── Ctrl-C trap ───────────────────────────────────────────
cleanup() {
    echo ""
    print_info "Interrupted — exiting gracefully."
    exit 0
}
trap cleanup INT

# ──── Docker detection ──────────────────────────────────────
detect_docker() {
    if ! command -v docker &>/dev/null; then
        print_error "Docker is not installed!"
        echo ""
        print_info "Install Docker: https://docs.docker.com/get-docker/"
        exit 1
    fi

    if docker compose version &>/dev/null; then
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose &>/dev/null; then
        COMPOSE_CMD="docker-compose"
    else
        print_error "Docker Compose is not installed!"
        echo ""
        print_info "Install Docker Compose: https://docs.docker.com/compose/install/"
        exit 1
    fi

    print_success "Docker detected"
    print_success "Docker Compose detected  ${DIM}(${COMPOSE_CMD})${NC}"
}

# ──── Banner ────────────────────────────────────────────────
show_banner() {
    clear
    echo ""
    echo -e "${CYAN}${BOLD}"
    echo "  ╔══════════════════════════════════════════════════════╗"
    echo "  ║                                                      ║"
    echo "  ║     🛒  Remnawave Telegram Shop  —  Setup Wizard     ║"
    echo "  ║                                                      ║"
    echo "  ╚══════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

# ──── Main Menu ─────────────────────────────────────────────
show_menu() {
    local env_status
    if [[ -f "$ENV_FILE" ]]; then
        env_status="${GREEN}● .env found${NC}"
    else
        env_status="${YELLOW}○ .env not found${NC}"
    fi

    echo -e "  ${DIM}Status: ${env_status}${NC}"
    echo ""
    echo -e "  ${BOLD}Choose an action:${NC}"
    echo ""
    echo -e "    ${GREEN}1${NC})  🚀  Fresh Install ${DIM}(guided wizard)${NC}"
    echo -e "    ${GREEN}2${NC})  ✏️   Edit Configuration"
    echo -e "    ${GREEN}3${NC})  ▶️   Start / Restart Services"
    echo -e "    ${GREEN}4${NC})  ⏹   Stop Services"
    echo -e "    ${GREEN}5${NC})  📋  View Logs"
    echo -e "    ${GREEN}6${NC})  🔄  Update ${DIM}(pull latest image)${NC}"
    echo -e "    ${GREEN}7${NC})  🗑   Uninstall ${DIM}(remove containers + data)${NC}"
    echo ""
    echo -e "    ${RED}0${NC})  🚪  Exit"
    echo ""
    echo -ne "  ${ARROW}  Your choice: "
}

# ──── Fresh Install Wizard ──────────────────────────────────
wizard() {
    # Use bash associative array to collect all config values
    declare -A CFG

    # ── 1. Telegram ─────────────────────────────────────────
    print_section "1/14  Telegram Bot"
    ask_required "Bot Token" "" "TELEGRAM_TOKEN"
    ask_required "Admin Telegram ID" "" "ADMIN_TELEGRAM_ID"

    # ── 2. Remnawave ────────────────────────────────────────
    print_section "2/14  Remnawave Panel"
    ask_required "Panel URL" "https://example.com" "REMNAWAVE_URL"
    ask_required "API Token" "" "REMNAWAVE_TOKEN"
    ask "Mode (remote/local)" "remote" "REMNAWAVE_MODE"
    ask "Remnawave Tag" "TEST_PUPA" "REMNAWAVE_TAG"

    # ── 3. Pricing ──────────────────────────────────────────
    print_section "3/14  Subscription Pricing"
    print_info "Set prices in your local currency units."
    ask_number "Price for 1 month" "99" "PRICE_1"
    ask_number "Price for 3 months" "321" "PRICE_3"
    ask_number "Price for 6 months" "674" "PRICE_6"
    ask_number "Price for 12 months" "1200" "PRICE_12"
    echo ""
    print_info "Set prices in Telegram Stars."
    ask_number "Stars price for 1 month" "99" "STARS_PRICE_1"
    ask_number "Stars price for 3 months" "321" "STARS_PRICE_3"
    ask_number "Stars price for 6 months" "674" "STARS_PRICE_6"
    ask_number "Stars price for 12 months" "1200" "STARS_PRICE_12"
    ask_number "Days in month" "30" "DAYS_IN_MONTH"

    # ── 4. CryptoPay ───────────────────────────────────────
    print_section "4/14  Payment — CryptoPay"
    ask_bool "Enable CryptoPay?" "true" "CRYPTO_PAY_ENABLED"
    if [[ "${CFG[CRYPTO_PAY_ENABLED]}" == "true" ]]; then
        ask_required "CryptoPay Token" "" "CRYPTO_PAY_TOKEN"
        ask "CryptoPay API URL" "https://pay.crypt.bot" "CRYPTO_PAY_URL"
    else
        CFG[CRYPTO_PAY_TOKEN]="token"
        CFG[CRYPTO_PAY_URL]="https://pay.crypt.bot"
        print_info "CryptoPay disabled — skipping."
    fi

    # ── 5. YooKassa ─────────────────────────────────────────
    print_section "5/14  Payment — YooKassa"
    ask_bool "Enable YooKassa?" "false" "YOOKASA_ENABLED"
    if [[ "${CFG[YOOKASA_ENABLED]}" == "true" ]]; then
        ask_required "YooKassa Secret Key" "" "YOOKASA_SECRET_KEY"
        ask_required "YooKassa Shop ID" "" "YOOKASA_SHOP_ID"
        ask "YooKassa API URL" "https://api.yookassa.ru/v3" "YOOKASA_URL"
        ask "YooKassa Email" "" "YOOKASA_EMAIL"
    else
        CFG[YOOKASA_SECRET_KEY]="key"
        CFG[YOOKASA_SHOP_ID]="id"
        CFG[YOOKASA_URL]="https://api.yookassa.ru/v3"
        CFG[YOOKASA_EMAIL]=""
        print_info "YooKassa disabled — skipping."
    fi

    # ── 6. Telegram Stars ───────────────────────────────────
    print_section "6/14  Payment — Telegram Stars"
    ask_bool "Enable Telegram Stars?" "true" "TELEGRAM_STARS_ENABLED"
    ask_bool "Require paid purchase before Stars?" "false" "REQUIRE_PAID_PURCHASE_FOR_STARS"

    # ── 7. Tribute ──────────────────────────────────────────
    print_section "7/14  Payment — Tribute"
    echo -ne "     ${BOLD}Enable Tribute? (true/false)${NC} ${DIM}[false]${NC}: "
    local tribute_input
    read -r tribute_input
    tribute_input="${tribute_input:-false}"
    tribute_input="$(echo "$tribute_input" | tr '[:upper:]' '[:lower:]')"
    if [[ "$tribute_input" == "true" ]]; then
        ask_required "Tribute Webhook URL path (e.g. /tribute/webhook)" "" "TRIBUTE_WEBHOOK_URL"
        ask_required "Tribute API Key" "" "TRIBUTE_API_KEY"
        ask_required "Tribute Payment URL" "" "TRIBUTE_PAYMENT_URL"
        ask_number "Health Check Port" "82251" "HEALTH_CHECK_PORT"
    else
        CFG[TRIBUTE_WEBHOOK_URL]=""
        CFG[TRIBUTE_API_KEY]=""
        CFG[TRIBUTE_PAYMENT_URL]=""
        CFG[HEALTH_CHECK_PORT]=""
        print_info "Tribute disabled — skipping."
    fi

    # ── 8. Tax / Moynalog ───────────────────────────────────
    print_section "8/14  Tax — Moynalog"
    ask_bool "Enable Moynalog?" "false" "MOYNALOG_ENABLED"
    if [[ "${CFG[MOYNALOG_ENABLED]}" == "true" ]]; then
        ask_required "Moynalog Username" "" "MOYNALOG_USERNAME"
        ask_required "Moynalog Password" "" "MOYNALOG_PASSWORD"
        ask "Moynalog API URL" "https://lknpd.nalog.ru/api/v1" "MOYNALOG_URL"
    else
        CFG[MOYNALOG_USERNAME]=""
        CFG[MOYNALOG_PASSWORD]=""
        CFG[MOYNALOG_URL]="https://lknpd.nalog.ru/api/v1"
        print_info "Moynalog disabled — skipping."
    fi

    # ── 9. Trial ────────────────────────────────────────────
    print_section "9/14  Trial Subscriptions"
    ask_number "Trial days (0 = disabled)" "0" "TRIAL_DAYS"
    if [[ "${CFG[TRIAL_DAYS]}" -gt 0 ]]; then
        ask_number "Trial traffic limit (GB)" "20" "TRIAL_TRAFFIC_LIMIT"
        ask_reset_strategy "Trial traffic reset strategy" "MONTH" "TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY"
        ask "Trial Squad UUIDs (comma-separated)" "" "TRIAL_INTERNAL_SQUADS"
        ask "Trial External Squad UUID" "" "TRIAL_EXTERNAL_SQUAD_UUID"
        ask "Trial Remnawave Tag" "" "TRIAL_REMNAWAVE_TAG"
    else
        CFG[TRIAL_TRAFFIC_LIMIT]="20"
        CFG[TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY]="MONTH"
        CFG[TRIAL_INTERNAL_SQUADS]=""
        CFG[TRIAL_EXTERNAL_SQUAD_UUID]=""
        CFG[TRIAL_REMNAWAVE_TAG]=""
        print_info "Trials disabled — skipping."
    fi

    # ── 10. Traffic & Referral ──────────────────────────────
    print_section "10/14  Traffic & Referral"
    ask_number "Traffic limit (GB, 0 = unlimited)" "100" "TRAFFIC_LIMIT"
    ask_reset_strategy "Traffic reset strategy" "MONTH" "TRAFFIC_LIMIT_RESET_STRATEGY"
    ask_number "Referral bonus days (0 = disabled)" "7" "REFERRAL_DAYS"

    # ── 11. Squads ──────────────────────────────────────────
    print_section "11/14  Squad Assignment"
    print_info "Leave empty to assign all available squads."
    ask "Squad UUIDs (comma-separated)" "" "SQUAD_UUIDS"
    ask "External Squad UUID" "" "EXTERNAL_SQUAD_UUID"

    # ── 12. URLs ────────────────────────────────────────────
    print_section "12/14  Optional URLs"
    print_info "Leave empty to hide the corresponding button."
    ask "Server Status URL" "" "SERVER_STATUS_URL"
    ask "Support URL" "" "SUPPORT_URL"
    ask "Feedback URL" "" "FEEDBACK_URL"
    ask "Channel URL" "" "CHANNEL_URL"
    ask "Terms of Service URL" "" "TOS_URL"

    # ── 13. Blocked / Whitelisted IDs ───────────────────────
    print_section "13/14  Access Control"
    ask "Blocked Telegram IDs (comma-separated)" "" "BLOCKED_TELEGRAM_IDS"
    ask "Whitelisted Telegram IDs (comma-separated)" "" "WHITELISTED_TELEGRAM_IDS"

    # ── 14. Database & Advanced ─────────────────────────────
    print_section "14/14  Database & Advanced"
    ask "PostgreSQL User" "postgres" "POSTGRES_USER"
    ask "PostgreSQL Password" "postgres" "POSTGRES_PASSWORD"
    ask "PostgreSQL Database" "postgres" "POSTGRES_DB"

    # Build DATABASE_URL from parts
    CFG[DATABASE_URL]="postgres://${CFG[POSTGRES_USER]}:${CFG[POSTGRES_PASSWORD]}@db:5432/${CFG[POSTGRES_DB]}?sslmode=disable"

    echo ""
    ask "Default Language (en/ru)" "en" "DEFAULT_LANGUAGE"
    ask "Mini App URL (leave empty to skip)" "" "MINI_APP_URL"
    ask "Additional Remnawave Headers (key1:val1;key2:val2)" "" "REMNAWAVE_HEADERS"

    # ── Write .env ──────────────────────────────────────────
    echo ""
    print_header "Review & Save"
    echo ""

    if [[ -f "$ENV_FILE" ]]; then
        local backup="${ENV_FILE}.backup.$(date +%Y%m%d_%H%M%S)"
        cp "$ENV_FILE" "$backup"
        print_info "Existing .env backed up to ${backup##*/}"
    fi

    # Write the env file
    cat > "$ENV_FILE" <<ENVEOF
# ────────────────────────────────────────────────────────────
#  Remnawave Telegram Shop  —  Generated by setup.sh
#  $(date '+%Y-%m-%d %H:%M:%S')
# ────────────────────────────────────────────────────────────

# ── Telegram ────────────────────────────────────────────────
TELEGRAM_TOKEN=${CFG[TELEGRAM_TOKEN]}
ADMIN_TELEGRAM_ID=${CFG[ADMIN_TELEGRAM_ID]}

# ── Remnawave Panel ─────────────────────────────────────────
REMNAWAVE_URL=${CFG[REMNAWAVE_URL]}
REMNAWAVE_TOKEN=${CFG[REMNAWAVE_TOKEN]}
REMNAWAVE_MODE=${CFG[REMNAWAVE_MODE]}
REMNAWAVE_TAG=${CFG[REMNAWAVE_TAG]}

# ── Subscription Pricing ────────────────────────────────────
PRICE_1=${CFG[PRICE_1]}
PRICE_3=${CFG[PRICE_3]}
PRICE_6=${CFG[PRICE_6]}
PRICE_12=${CFG[PRICE_12]}
STARS_PRICE_1=${CFG[STARS_PRICE_1]}
STARS_PRICE_3=${CFG[STARS_PRICE_3]}
STARS_PRICE_6=${CFG[STARS_PRICE_6]}
STARS_PRICE_12=${CFG[STARS_PRICE_12]}
DAYS_IN_MONTH=${CFG[DAYS_IN_MONTH]}

# ── Payment — CryptoPay ─────────────────────────────────────
CRYPTO_PAY_ENABLED=${CFG[CRYPTO_PAY_ENABLED]}
CRYPTO_PAY_TOKEN=${CFG[CRYPTO_PAY_TOKEN]}
CRYPTO_PAY_URL=${CFG[CRYPTO_PAY_URL]}

# ── Payment — YooKassa ───────────────────────────────────────
YOOKASA_ENABLED=${CFG[YOOKASA_ENABLED]}
YOOKASA_SECRET_KEY=${CFG[YOOKASA_SECRET_KEY]}
YOOKASA_SHOP_ID=${CFG[YOOKASA_SHOP_ID]}
YOOKASA_URL=${CFG[YOOKASA_URL]}
YOOKASA_EMAIL=${CFG[YOOKASA_EMAIL]}

# ── Payment — Telegram Stars ────────────────────────────────
TELEGRAM_STARS_ENABLED=${CFG[TELEGRAM_STARS_ENABLED]}
REQUIRE_PAID_PURCHASE_FOR_STARS=${CFG[REQUIRE_PAID_PURCHASE_FOR_STARS]}

# ── Payment — Tribute ───────────────────────────────────────
TRIBUTE_WEBHOOK_URL=${CFG[TRIBUTE_WEBHOOK_URL]}
TRIBUTE_API_KEY=${CFG[TRIBUTE_API_KEY]}
TRIBUTE_PAYMENT_URL=${CFG[TRIBUTE_PAYMENT_URL]}
HEALTH_CHECK_PORT=${CFG[HEALTH_CHECK_PORT]}

# ── Tax — Moynalog ──────────────────────────────────────────
MOYNALOG_ENABLED=${CFG[MOYNALOG_ENABLED]}
MOYNALOG_USERNAME=${CFG[MOYNALOG_USERNAME]}
MOYNALOG_PASSWORD=${CFG[MOYNALOG_PASSWORD]}
MOYNALOG_URL=${CFG[MOYNALOG_URL]}

# ── Traffic & Referral ──────────────────────────────────────
TRAFFIC_LIMIT=${CFG[TRAFFIC_LIMIT]}
TRAFFIC_LIMIT_RESET_STRATEGY=${CFG[TRAFFIC_LIMIT_RESET_STRATEGY]}
REFERRAL_DAYS=${CFG[REFERRAL_DAYS]}

# ── Trial ────────────────────────────────────────────────────
TRIAL_DAYS=${CFG[TRIAL_DAYS]}
TRIAL_TRAFFIC_LIMIT=${CFG[TRIAL_TRAFFIC_LIMIT]}
TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY=${CFG[TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY]}
TRIAL_INTERNAL_SQUADS=${CFG[TRIAL_INTERNAL_SQUADS]}
TRIAL_EXTERNAL_SQUAD_UUID=${CFG[TRIAL_EXTERNAL_SQUAD_UUID]}
TRIAL_REMNAWAVE_TAG=${CFG[TRIAL_REMNAWAVE_TAG]}

# ── Squads ───────────────────────────────────────────────────
SQUAD_UUIDS=${CFG[SQUAD_UUIDS]}
EXTERNAL_SQUAD_UUID=${CFG[EXTERNAL_SQUAD_UUID]}

# ── Optional URLs ────────────────────────────────────────────
SERVER_STATUS_URL=${CFG[SERVER_STATUS_URL]}
SUPPORT_URL=${CFG[SUPPORT_URL]}
FEEDBACK_URL=${CFG[FEEDBACK_URL]}
CHANNEL_URL=${CFG[CHANNEL_URL]}
TOS_URL=${CFG[TOS_URL]}

# ── Access Control ───────────────────────────────────────────
BLOCKED_TELEGRAM_IDS=${CFG[BLOCKED_TELEGRAM_IDS]}
WHITELISTED_TELEGRAM_IDS=${CFG[WHITELISTED_TELEGRAM_IDS]}

# ── Database ─────────────────────────────────────────────────
DATABASE_URL=${CFG[DATABASE_URL]}
POSTGRES_USER=${CFG[POSTGRES_USER]}
POSTGRES_PASSWORD=${CFG[POSTGRES_PASSWORD]}
POSTGRES_DB=${CFG[POSTGRES_DB]}

# ── Advanced ─────────────────────────────────────────────────
DEFAULT_LANGUAGE=${CFG[DEFAULT_LANGUAGE]}
MINI_APP_URL=${CFG[MINI_APP_URL]}
REMNAWAVE_HEADERS=${CFG[REMNAWAVE_HEADERS]}
ENVEOF

    print_success ".env file created successfully!"
    echo ""

    # Offer to start services right away
    echo -ne "  ${ARROW}  Start services now? ${DIM}(y/n)${NC} [y]: "
    local start_now
    read -r start_now
    start_now="${start_now:-y}"

    if [[ "$start_now" == "y" || "$start_now" == "Y" ]]; then
        do_start
    else
        print_info "You can start services later from the main menu (option 3)."
    fi
}

# ──── Edit Config ───────────────────────────────────────────
do_edit() {
    if [[ ! -f "$ENV_FILE" ]]; then
        print_error "No .env file found. Run 'Fresh Install' first (option 1)."
        return
    fi

    local editor="${EDITOR:-}"
    if [[ -z "$editor" ]]; then
        if command -v nano &>/dev/null; then
            editor="nano"
        elif command -v vi &>/dev/null; then
            editor="vi"
        else
            print_error "No text editor found. Set the EDITOR environment variable."
            return
        fi
    fi

    print_info "Opening .env in ${editor}..."
    "$editor" "$ENV_FILE"
    print_success "Configuration saved."
}

# ──── Start / Restart ───────────────────────────────────────
do_start() {
    if [[ ! -f "$ENV_FILE" ]]; then
        print_error "No .env file found. Run 'Fresh Install' first (option 1)."
        return
    fi

    print_arrow "Starting services..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD up -d)
    echo ""
    print_success "Services are running!"
    print_info "Use option 5 to view logs."
}

# ──── Stop ──────────────────────────────────────────────────
do_stop() {
    print_arrow "Stopping services..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD down)
    echo ""
    print_success "Services stopped."
}

# ──── View Logs ─────────────────────────────────────────────
do_logs() {
    print_info "Showing last 100 lines. Press Ctrl-C to exit."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD logs -f --tail 100) || true
}

# ──── Update ────────────────────────────────────────────────
do_update() {
    print_arrow "Pulling latest images..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD pull)
    echo ""
    print_arrow "Restarting services..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD down && $COMPOSE_CMD up -d)
    echo ""
    print_success "Update complete! Services are running with the latest image."
}

# ──── Uninstall ─────────────────────────────────────────────
do_uninstall() {
    echo ""
    print_error "⚠  WARNING: This will remove ALL containers AND data volumes."
    echo -ne "  ${ARROW}  Type ${RED}${BOLD}YES${NC} to confirm: "
    local confirm
    read -r confirm
    if [[ "$confirm" != "YES" ]]; then
        print_info "Uninstall cancelled."
        return
    fi

    echo -ne "  ${ARROW}  Are you absolutely sure? ${DIM}(y/n)${NC}: "
    local confirm2
    read -r confirm2
    if [[ "$confirm2" != "y" && "$confirm2" != "Y" ]]; then
        print_info "Uninstall cancelled."
        return
    fi

    print_arrow "Removing containers and volumes..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD down -v)
    echo ""
    print_success "Containers and data volumes removed."

    echo -ne "  ${ARROW}  Also delete .env file? ${DIM}(y/n)${NC} [n]: "
    local del_env
    read -r del_env
    del_env="${del_env:-n}"
    if [[ "$del_env" == "y" || "$del_env" == "Y" ]]; then
        rm -f "$ENV_FILE"
        print_success ".env file deleted."
    fi
}

# ──── Main Loop ─────────────────────────────────────────────
main() {
    show_banner
    detect_docker
    echo ""

    while true; do
        echo ""
        echo -e "${CYAN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "${CYAN}${BOLD}  Main Menu${NC}"
        echo -e "${CYAN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        show_menu

        local choice
        read -r choice

        case "$choice" in
            1) wizard ;;
            2) do_edit ;;
            3) do_start ;;
            4) do_stop ;;
            5) do_logs ;;
            6) do_update ;;
            7) do_uninstall ;;
            0)
                echo ""
                print_success "Goodbye! 👋"
                echo ""
                exit 0
                ;;
            *)
                print_error "Invalid choice. Please try again."
                ;;
        esac
    done
}

main
