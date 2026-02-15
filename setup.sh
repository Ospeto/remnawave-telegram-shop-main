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
    echo -e "    ${GREEN}3${NC})  💰  Edit Pricing"
    echo -e "    ${GREEN}4${NC})  ▶️   Start / Restart Services"
    echo -e "    ${GREEN}5${NC})  ⏹   Stop Services"
    echo -e "    ${GREEN}6${NC})  📋  View Logs"
    echo -e "    ${GREEN}7${NC})  🔄  Update ${DIM}(rebuild from source)${NC}"
    echo -e "    ${GREEN}8${NC})  🗑   Uninstall ${DIM}(remove containers + data)${NC}"
    echo -e "    ${GREEN}9${NC})  📱  Setup Mini App ${DIM}(build + configure)${NC}"
    echo ""
    echo -e "    ${RED}0${NC})  🚪  Exit"
    echo ""
    echo -ne "  ${ARROW}  Your choice: "
}

# ──── Fresh Install Wizard ──────────────────────────────────
wizard() {
    # If .env already exists, ask whether to skip
    if [[ -f "$ENV_FILE" ]]; then
        echo ""
        print_info "An existing .env file was found."
        echo ""
        echo -e "    ${GREEN}1${NC})  Keep current .env and start services"
        echo -e "    ${GREEN}2${NC})  Overwrite with fresh wizard"
        echo -e "    ${GREEN}3${NC})  Edit current .env in text editor"
        echo ""
        echo -ne "  ${ARROW}  Your choice [1]: "
        local env_choice
        read -r env_choice
        env_choice="${env_choice:-1}"
        case "$env_choice" in
            1)
                print_success "Keeping existing .env."
                do_start
                return
                ;;
            3)
                do_edit
                return
                ;;
            2)
                print_info "Starting fresh wizard..."
                ;;
            *)
                print_success "Keeping existing .env."
                do_start
                return
                ;;
        esac
    fi

    # Use bash associative array to collect all config values
    declare -A CFG

    # ── 1. Telegram ─────────────────────────────────────────
    print_section "1/10  Telegram Bot"
    ask_required "Bot Token" "" "TELEGRAM_TOKEN"
    ask_required "Admin Telegram ID" "" "ADMIN_TELEGRAM_ID"

    # ── 2. Remnawave ────────────────────────────────────────
    print_section "2/10  Remnawave Panel"
    ask_required "Panel URL" "https://example.com" "REMNAWAVE_URL"
    ask_required "API Token" "" "REMNAWAVE_TOKEN"
    ask "Mode (remote/local)" "remote" "REMNAWAVE_MODE"
    ask "Remnawave Tag" "TEST_PUPA" "REMNAWAVE_TAG"

    # ── 3. Pricing ──────────────────────────────────────────
    print_section "3/10  Subscription Plans"
    print_info "Add plans one by one. Format: Label, Days, Price, Traffic (GB, 0=unlimited)."
    ask "Currency code" "MMK" "CURRENCY"
    echo ""

    local PLANS_LIST=""
    local plan_num=1
    while true; do
        echo -e "  ${CYAN}── Plan ${plan_num} ──${NC}"
        local p_label p_days p_price p_traffic
        echo -ne "  ${ARROW} Label ${DIM}[Unlimited]${NC}: "
        read -r p_label
        p_label="${p_label:-Unlimited}"
        echo -ne "  ${ARROW} Duration in days ${DIM}[30]${NC}: "
        read -r p_days
        p_days="${p_days:-30}"
        echo -ne "  ${ARROW} Price ${DIM}[10000]${NC}: "
        read -r p_price
        p_price="${p_price:-10000}"
        echo -ne "  ${ARROW} Traffic GB (0=unlimited) ${DIM}[0]${NC}: "
        read -r p_traffic
        p_traffic="${p_traffic:-0}"

        if [[ -n "$PLANS_LIST" ]]; then
            PLANS_LIST="${PLANS_LIST},${p_label}|${p_days}|${p_price}|${p_traffic}"
        else
            PLANS_LIST="${p_label}|${p_days}|${p_price}|${p_traffic}"
        fi
        echo -e "  ${GREEN}✓${NC} Added: ${p_label} ${p_days}d ${p_price} ${CFG[CURRENCY]} ${p_traffic}GB"
        echo ""

        echo -ne "  ${ARROW}  Add another plan? ${DIM}(y/n)${NC} [n]: "
        local more
        read -r more
        if [[ "$more" != "y" && "$more" != "Y" ]]; then
            break
        fi
        ((plan_num++))
        echo ""
    done
    CFG[PLANS]="$PLANS_LIST"

    # ── 4. CryptoPay ───────────────────────────────────────
    print_section "4/11  Payment — CryptoPay"
    ask_bool "Enable CryptoPay?" "true" "CRYPTO_PAY_ENABLED"
    if [[ "${CFG[CRYPTO_PAY_ENABLED]}" == "true" ]]; then
        ask_required "CryptoPay Token" "" "CRYPTO_PAY_TOKEN"
        ask "CryptoPay API URL" "https://pay.crypt.bot" "CRYPTO_PAY_URL"
    else
        CFG[CRYPTO_PAY_TOKEN]="token"
        CFG[CRYPTO_PAY_URL]="https://pay.crypt.bot"
        print_info "CryptoPay disabled — skipping."
    fi

    # ── 5. Mobile Banking ──────────────────────────────────
    print_section "5/11  Payment — Mobile Banking (KPay/WavePay/AyaPay)"
    ask_bool "Enable Mobile Banking?" "false" "MOBILE_BANKING_ENABLED"
    if [[ "${CFG[MOBILE_BANKING_ENABLED]}" == "true" ]]; then
        ask_required "Receiving Phone Number" "" "MOBILE_BANKING_PHONE"
        ask_required "Gemini API Key" "" "GEMINI_API_KEY"
        ask "Gemini Model" "gemini-2.5-flash" "GEMINI_MODEL"
    else
        CFG[MOBILE_BANKING_PHONE]=""
        CFG[GEMINI_API_KEY]=""
        CFG[GEMINI_MODEL]="gemini-2.5-flash"
        print_info "Mobile Banking disabled — skipping."
    fi

    # ── 6. Trial ────────────────────────────────────────────
    print_section "6/11  Trial Subscriptions"
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

    # ── 7. Traffic & Referral ──────────────────────────────
    print_section "7/11  Traffic & Referral"
    ask_reset_strategy "Traffic reset strategy" "MONTH" "TRAFFIC_LIMIT_RESET_STRATEGY"
    ask_number "Referral bonus days (0 = disabled)" "7" "REFERRAL_DAYS"

    # ── 8. Squads ──────────────────────────────────────────
    print_section "8/11  Squad Assignment"
    print_info "Leave empty to assign all available squads."
    ask "Squad UUIDs (comma-separated)" "" "SQUAD_UUIDS"
    ask "External Squad UUID" "" "EXTERNAL_SQUAD_UUID"

    # ── 9. URLs ────────────────────────────────────────────
    print_section "9/11  Optional URLs"
    print_info "Leave empty to hide the corresponding button."
    ask "Server Status URL" "" "SERVER_STATUS_URL"
    ask "Support URL" "" "SUPPORT_URL"
    ask "Feedback URL" "" "FEEDBACK_URL"
    ask "Channel URL" "" "CHANNEL_URL"
    ask "Terms of Service URL" "" "TOS_URL"

    # ── 10. Blocked / Whitelisted IDs ───────────────────────
    print_section "10/11  Access Control"
    ask "Blocked Telegram IDs (comma-separated)" "" "BLOCKED_TELEGRAM_IDS"
    ask "Whitelisted Telegram IDs (comma-separated)" "" "WHITELISTED_TELEGRAM_IDS"

    # ── 11. Database & Advanced ─────────────────────────────
    print_section "11/11  Database & Advanced"
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
REMNAWAVE_TAG=$(echo "${CFG[REMNAWAVE_TAG]}" | tr '[:lower:]' '[:upper:]')

# ── Subscription Plans ──────────────────────────────────────
CURRENCY=${CFG[CURRENCY]}
PLANS=${CFG[PLANS]}

# ── Payment — CryptoPay ─────────────────────────────────────
CRYPTO_PAY_ENABLED=${CFG[CRYPTO_PAY_ENABLED]}
CRYPTO_PAY_TOKEN=${CFG[CRYPTO_PAY_TOKEN]}
CRYPTO_PAY_URL=${CFG[CRYPTO_PAY_URL]}

# ── Payment — Mobile Banking ────────────────────────────────
MOBILE_BANKING_ENABLED=${CFG[MOBILE_BANKING_ENABLED]}
MOBILE_BANKING_PHONE=${CFG[MOBILE_BANKING_PHONE]}
GEMINI_API_KEY=${CFG[GEMINI_API_KEY]}
GEMINI_MODEL=${CFG[GEMINI_MODEL]}

# ── Traffic & Referral ──────────────────────────────────────
TRAFFIC_LIMIT_RESET_STRATEGY=${CFG[TRAFFIC_LIMIT_RESET_STRATEGY]}
REFERRAL_DAYS=${CFG[REFERRAL_DAYS]}

# ── Trial ────────────────────────────────────────────────────
TRIAL_DAYS=${CFG[TRIAL_DAYS]}
TRIAL_TRAFFIC_LIMIT=${CFG[TRIAL_TRAFFIC_LIMIT]}
TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY=${CFG[TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY]}
TRIAL_INTERNAL_SQUADS=${CFG[TRIAL_INTERNAL_SQUADS]}
TRIAL_EXTERNAL_SQUAD_UUID=${CFG[TRIAL_EXTERNAL_SQUAD_UUID]}
TRIAL_REMNAWAVE_TAG=$(echo "${CFG[TRIAL_REMNAWAVE_TAG]}" | tr '[:lower:]' '[:upper:]')

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

    print_arrow "Building and starting services..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD up -d --build)
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
    print_arrow "Rebuilding from source..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD build --no-cache)
    echo ""
    print_arrow "Restarting services..."
    echo ""
    (cd "$SCRIPT_DIR" && $COMPOSE_CMD down && $COMPOSE_CMD up -d)
    echo ""
    print_success "Update complete! Services are running with the latest build."
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

# ──── Edit Pricing Only ─────────────────────────────────────
do_edit_pricing() {
    if [[ ! -f "$ENV_FILE" ]]; then
        print_error "No .env file found. Run 'Fresh Install' first (option 1)."
        return
    fi

    # Read current values from .env
    local cur_currency cur_plans
    cur_currency=$({ grep -E '^CURRENCY=' "$ENV_FILE" || true; } | cut -d= -f2-)
    cur_currency="${cur_currency:-MMK}"
    cur_plans=$({ grep -E '^PLANS=' "$ENV_FILE" || true; } | cut -d= -f2-)

    print_section "Edit Plans"

    # Show existing plans
    if [[ -n "$cur_plans" ]]; then
        echo -e "  ${CYAN}Current plans:${NC}"
        local idx=1
        IFS=',' read -ra plan_arr <<< "$cur_plans"
        for entry in "${plan_arr[@]}"; do
            IFS='|' read -r label days price traffic <<< "$entry"
            local traffic_str="unlimited"
            [[ "$traffic" != "0" ]] && traffic_str="${traffic}GB/mo"
            echo -e "    ${idx}. ${label} — ${days} days — ${price} ${cur_currency} — ${traffic_str}"
            ((idx++))
        done
        echo ""
    fi

    # Ask for currency
    echo -ne "  ${ARROW} Currency ${DIM}[${cur_currency}]${NC}: "
    local new_currency
    read -r new_currency
    new_currency="${new_currency:-$cur_currency}"

    echo ""
    echo -e "  ${CYAN}Options:${NC}"
    echo -e "    ${GREEN}1${NC}) Keep existing plans"
    echo -e "    ${GREEN}2${NC}) Add a new plan"
    echo -e "    ${GREEN}3${NC}) Replace all plans"
    echo ""
    echo -ne "  ${ARROW} Choice [1]: "
    local plan_choice
    read -r plan_choice
    plan_choice="${plan_choice:-1}"

    local new_plans="$cur_plans"

    case "$plan_choice" in
        2)
            # Add plans to existing
            while true; do
                echo ""
                echo -e "  ${CYAN}── Add Plan ──${NC}"
                local p_label p_days p_price p_traffic
                echo -ne "  ${ARROW} Label ${DIM}[Unlimited]${NC}: "
                read -r p_label
                p_label="${p_label:-Unlimited}"
                echo -ne "  ${ARROW} Duration in days ${DIM}[30]${NC}: "
                read -r p_days
                p_days="${p_days:-30}"
                echo -ne "  ${ARROW} Price ${DIM}[10000]${NC}: "
                read -r p_price
                p_price="${p_price:-10000}"
                echo -ne "  ${ARROW} Traffic GB (0=unlimited) ${DIM}[0]${NC}: "
                read -r p_traffic
                p_traffic="${p_traffic:-0}"

                if [[ -n "$new_plans" ]]; then
                    new_plans="${new_plans},${p_label}|${p_days}|${p_price}|${p_traffic}"
                else
                    new_plans="${p_label}|${p_days}|${p_price}|${p_traffic}"
                fi
                echo -e "  ${GREEN}✓${NC} Added: ${p_label} ${p_days}d ${p_price} ${new_currency} ${p_traffic}GB"

                echo -ne "  ${ARROW}  Add another? ${DIM}(y/n)${NC} [n]: "
                local more
                read -r more
                [[ "$more" != "y" && "$more" != "Y" ]] && break
            done
            ;;
        3)
            # Replace all
            new_plans=""
            local plan_num=1
            while true; do
                echo ""
                echo -e "  ${CYAN}── Plan ${plan_num} ──${NC}"
                local p_label p_days p_price p_traffic
                echo -ne "  ${ARROW} Label ${DIM}[Unlimited]${NC}: "
                read -r p_label
                p_label="${p_label:-Unlimited}"
                echo -ne "  ${ARROW} Duration in days ${DIM}[30]${NC}: "
                read -r p_days
                p_days="${p_days:-30}"
                echo -ne "  ${ARROW} Price ${DIM}[10000]${NC}: "
                read -r p_price
                p_price="${p_price:-10000}"
                echo -ne "  ${ARROW} Traffic GB (0=unlimited) ${DIM}[0]${NC}: "
                read -r p_traffic
                p_traffic="${p_traffic:-0}"

                if [[ -n "$new_plans" ]]; then
                    new_plans="${new_plans},${p_label}|${p_days}|${p_price}|${p_traffic}"
                else
                    new_plans="${p_label}|${p_days}|${p_price}|${p_traffic}"
                fi
                echo -e "  ${GREEN}✓${NC} Added: ${p_label} ${p_days}d ${p_price} ${new_currency} ${p_traffic}GB"

                echo -ne "  ${ARROW}  Add another? ${DIM}(y/n)${NC} [n]: "
                local more
                read -r more
                [[ "$more" != "y" && "$more" != "Y" ]] && break
                ((plan_num++))
            done
            ;;
        *)
            print_info "Keeping existing plans."
            ;;
    esac

    # Update .env in-place
    local update_var
    update_var() {
        local key="$1" val="$2"
        if grep -qE "^${key}=" "$ENV_FILE"; then
            awk -v k="$key" -v v="$val" 'BEGIN{FS=OFS="="} $1==k{$0=k"="v}1' "$ENV_FILE" > "${ENV_FILE}.tmp" && mv "${ENV_FILE}.tmp" "$ENV_FILE"
        else
            echo "${key}=${val}" >> "$ENV_FILE"
        fi
    }

    update_var "CURRENCY" "$new_currency"
    update_var "PLANS"    "$new_plans"
    rm -f "${ENV_FILE}.bak"

    echo ""
    print_success "Pricing updated!"

    echo -ne "  ${ARROW}  Restart services now? ${DIM}(y/n)${NC} [y]: "
    local restart
    read -r restart
    restart="${restart:-y}"
    if [[ "$restart" == "y" || "$restart" == "Y" ]]; then
        do_start
    fi
}

# ──── Setup Mini App ────────────────────────────────────────
do_setup_miniapp() {
    print_header "📱 Mini App Setup"

    # 1. Check for Node.js
    if ! command -v node &>/dev/null; then
        print_error "Node.js is not installed."
        print_info  "Install Node.js 18+ from https://nodejs.org"
        print_info  "On Ubuntu/Debian:  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash - && sudo apt-get install -y nodejs"
        print_info  "On macOS:          brew install node"
        return 1
    fi
    local node_ver
    node_ver=$(node -v)
    print_success "Node.js found: $node_ver"

    if ! command -v npm &>/dev/null; then
        print_error "npm is not installed (should come with Node.js)."
        return 1
    fi
    print_success "npm found: $(npm -v)"

    # 2. Install dependencies
    print_section "Installing Dependencies"
    if [[ ! -d "${SCRIPT_DIR}/web-app" ]]; then
        print_error "web-app/ directory not found. Make sure you have the latest code."
        return 1
    fi

    (cd "${SCRIPT_DIR}/web-app" && npm install) || {
        print_error "npm install failed."
        return 1
    }
    print_success "Dependencies installed."

    # 3. Build
    print_section "Building Mini App"
    (cd "${SCRIPT_DIR}/web-app" && npm run build) || {
        print_error "Build failed. Check errors above."
        return 1
    }
    print_success "Mini App built → web-app/dist/"

    # 4. Configure MINI_APP_URL
    print_section "Configuration"
    print_info  "The Mini App needs a public HTTPS URL to work inside Telegram."
    print_info  "If you don't have a domain yet, you can use:"
    print_info  "  • Cloudflare Tunnel (Free, recommended)"
    print_info  "  • Ngrok (Temporary testing)"
    print_info  "  • Your VPS IP (requires valid SSL, hard to get)"
    print_info  "Example: https://shop.example.com"
    echo ""

    local current_url=""
    if [[ -f "$ENV_FILE" ]]; then
        current_url=$(grep -E '^MINI_APP_URL=' "$ENV_FILE" 2>/dev/null | cut -d'=' -f2- || true)
    fi

    declare -A CFG
    ask "Mini App URL (public HTTPS)" "${current_url}" "miniapp_url"

    if [[ -n "${CFG[miniapp_url]}" ]]; then
        if [[ -f "$ENV_FILE" ]]; then
            if grep -q '^MINI_APP_URL=' "$ENV_FILE" 2>/dev/null; then
                awk -v key="MINI_APP_URL" -v val="${CFG[miniapp_url]}" \
                    'BEGIN{FS=OFS="="} $1==key{$2=val}{print}' "$ENV_FILE" > "${ENV_FILE}.tmp" && mv "${ENV_FILE}.tmp" "$ENV_FILE"
            else
                echo "MINI_APP_URL=${CFG[miniapp_url]}" >> "$ENV_FILE"
            fi
            print_success "MINI_APP_URL set to: ${CFG[miniapp_url]}"
        else
            print_error ".env file not found. Please run Fresh Install first."
            return 1
        fi
    else
        print_info "Skipped — you can set MINI_APP_URL later in .env"
    fi

    # 5. Reminder
    echo ""
    print_success "Mini App setup complete!"
    echo ""
    print_info  "Next steps:"
    print_arrow "1.  Restart bot:  Choose option 4 (Start / Restart Services)"
    print_arrow "2.  BotFather:    /mybots → Bot Settings → Menu Button → your URL"
    print_arrow "3.  Test:         Open the Menu Button in your bot on mobile"
    echo ""
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
            3) do_edit_pricing ;;
            4) do_start ;;
            5) do_stop ;;
            6) do_logs ;;
            7) do_update ;;
            8) do_uninstall ;;
            9) do_setup_miniapp ;;
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
