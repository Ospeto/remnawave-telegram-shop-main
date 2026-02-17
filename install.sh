#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────
#  Remnawave Telegram Shop — One-Line Installer
#
#  Usage:
#    bash <(curl -fsSL https://raw.githubusercontent.com/Ospeto/remnawave-telegram-shop-main/main/install.sh)
#
#  This script will:
#    1. Install Docker, Docker Compose, git, curl (if missing)
#    2. Clone the project to /opt/remnawave-shop
#    3. Launch the interactive setup wizard
# ─────────────────────────────────────────────────────────────
set -euo pipefail

# ──── Colors ────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

CHECK="${GREEN}✔${NC}"
CROSS="${RED}✘${NC}"
ARROW="${CYAN}➜${NC}"
INFO="${YELLOW}ℹ${NC}"

REPO_URL="https://github.com/Ospeto/remnawave-telegram-shop-main.git"
INSTALL_DIR="/opt/remnawave-shop"

# ──── Helpers ───────────────────────────────────────────────
print_success() { echo -e "  ${CHECK}  $1"; }
print_error()   { echo -e "  ${CROSS}  ${RED}$1${NC}"; }
print_info()    { echo -e "  ${INFO}  ${YELLOW}$1${NC}"; }
print_arrow()   { echo -e "  ${ARROW}  $1"; }

# ──── Banner ────────────────────────────────────────────────
echo ""
echo -e "${CYAN}${BOLD}"
echo "  ╔══════════════════════════════════════════════════════╗"
echo "  ║                                                      ║"
echo "  ║   🛒  Remnawave Telegram Shop  —  Quick Installer   ║"
echo "  ║                                                      ║"
echo "  ╚══════════════════════════════════════════════════════╝"
echo -e "${NC}"

# ──── Must run as root on Linux ─────────────────────────────
if [[ "$(uname)" != "Darwin" && "$(id -u)" -ne 0 ]]; then
    print_error "Please run as root:"
    echo -e "    ${CYAN}sudo bash <(curl -fsSL <URL>)${NC}"
    exit 1
fi

# ──── Install package helper ────────────────────────────────
install_pkg() {
    local pkg="$1"
    if command -v apt-get &>/dev/null; then
        apt-get update -qq && apt-get install -y -qq "$pkg"
    elif command -v yum &>/dev/null; then
        yum install -y "$pkg"
    elif command -v dnf &>/dev/null; then
        dnf install -y "$pkg"
    elif command -v pacman &>/dev/null; then
        pacman -Sy --noconfirm "$pkg"
    elif command -v brew &>/dev/null; then
        brew install "$pkg"
    else
        print_error "Cannot install '$pkg'. No supported package manager found."
        print_info  "Please install '$pkg' manually and re-run."
        exit 1
    fi
}

# ──── Step 1: Prerequisites ─────────────────────────────────
echo -e "  ${CYAN}${BOLD}── Step 1/4 — Prerequisites ──${NC}"
echo ""

for dep in curl git; do
    if ! command -v "$dep" &>/dev/null; then
        print_info "Installing ${dep}..."
        install_pkg "$dep"
    fi
    print_success "$dep"
done

# ──── Step 2: Docker ────────────────────────────────────────
echo ""
echo -e "  ${CYAN}${BOLD}── Step 2/4 — Docker ──${NC}"
echo ""

if ! command -v docker &>/dev/null; then
    if [[ "$(uname)" == "Darwin" ]]; then
        print_error "Docker Desktop is required on macOS."
        print_info  "Download: https://www.docker.com/products/docker-desktop/"
        print_info  "Install it, launch it, then re-run this script."
        exit 1
    fi

    print_info "Installing Docker..."
    curl -fsSL https://get.docker.com | sh || {
        print_error "Docker installation failed."
        print_info  "Try: https://docs.docker.com/engine/install/"
        exit 1
    }

    systemctl start docker  2>/dev/null || true
    systemctl enable docker 2>/dev/null || true

    if [[ -n "${SUDO_USER:-}" ]]; then
        usermod -aG docker "$SUDO_USER" 2>/dev/null || true
    fi

    print_success "Docker installed"
else
    print_success "Docker already installed"
fi

# Docker Compose
COMPOSE_CMD=""
if docker compose version &>/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
elif command -v docker-compose &>/dev/null; then
    COMPOSE_CMD="docker-compose"
else
    print_info "Installing Docker Compose plugin..."
    mkdir -p /usr/local/lib/docker/cli-plugins
    curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" \
        -o /usr/local/lib/docker/cli-plugins/docker-compose
    chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

    if docker compose version &>/dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
        print_success "Docker Compose installed"
    else
        print_error "Docker Compose installation failed."
        print_info  "https://docs.docker.com/compose/install/"
        exit 1
    fi
fi
print_success "Docker Compose ready ${DIM}(${COMPOSE_CMD})${NC}"

# ──── Step 3: Clone project ────────────────────────────────
echo ""
echo -e "  ${CYAN}${BOLD}── Step 3/4 — Download Project ──${NC}"
echo ""

if [[ -d "$INSTALL_DIR" ]]; then
    print_info "Project directory already exists at ${INSTALL_DIR}"

    # Backup .env if exists
    if [[ -f "$INSTALL_DIR/.env" ]]; then
         print_info "Found existing .env configuration."
         cp "$INSTALL_DIR/.env" "$INSTALL_DIR/.env.pre-update"
         print_success "Backed up .env to .env.pre-update"
    fi

    echo -ne "  ${ARROW}  Update to latest version? ${DIM}(y/n)${NC} [y]: "
    read -r update_choice
    update_choice="${update_choice:-y}"
    if [[ "$update_choice" == "y" || "$update_choice" == "Y" ]]; then
        (cd "$INSTALL_DIR" && git fetch && git reset --hard origin/main) || {
            print_error "git update failed."
        }
        # Restore .env
        if [[ -f "$INSTALL_DIR/.env.pre-update" ]]; then
            cp "$INSTALL_DIR/.env.pre-update" "$INSTALL_DIR/.env"
            print_success "Restored .env from backup"
        fi
        print_success "Updated to latest version"
    fi
else
    print_arrow "Cloning to ${INSTALL_DIR}..."
    git clone "$REPO_URL" "$INSTALL_DIR" || {
        print_error "Failed to clone repository."
        print_info  "Check your internet connection and try again."
        exit 1
    }
    print_success "Project downloaded"
fi

# ──── Step 4: Launch Setup Wizard ───────────────────────────
echo ""
echo -e "  ${CYAN}${BOLD}── Step 4/4 — Launch Setup Wizard ──${NC}"
echo ""
print_success "All dependencies installed!"
print_arrow "Launching setup wizard..."
echo ""
sleep 1

cd "$INSTALL_DIR"
chmod +x setup.sh
exec bash setup.sh
