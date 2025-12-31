#!/bin/bash
set -e

# CronCommander Agent Installer
# Usage: curl -sSL https://croncommander.com/install.sh | bash [options]
# Options:
#   --configure-as-root    Install in System Mode (runs as root, controls /etc/cron.d)

VERSION="${CC_VERSION:-latest}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/croncommander"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
SERVICE_FILE="/etc/systemd/system/cc-agent.service"
DOWNLOAD_URL="${CC_DOWNLOAD_URL:-https://github.com/croncommander/cc-agent/releases/download/${VERSION}/cc-agent-linux-amd64}"
SERVER_URL="${CC_SERVER_URL:-ws://localhost:8081/agent}"

# Internal user for User Mode
AGENT_USER="cc-agent-user"

# Mode configuration
MODE="user" # Default to 'user'

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_banner() {
    echo -e "${BLUE}"
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║                    CronCommander Agent                        ║"
    echo "║                       Installer v1.1                          ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

parse_args() {
    for arg in "$@"; do
        case $arg in
            --configure-as-root)
                MODE="system"
                ;;
            *)
                # Ignore unknown args or handle as needed
                ;;
        esac
    done
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        if command -v sudo &> /dev/null; then
            SUDO="sudo"
            info "Running with sudo privileges"
        else
            error "This script requires root privileges. Please run as root or install sudo."
        fi
    else
        SUDO=""
        info "Running as root"
    fi
}

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
    elif command -v lsb_release &> /dev/null; then
        OS=$(lsb_release -si | tr '[:upper:]' '[:lower:]')
        OS_VERSION=$(lsb_release -sr)
    else
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        OS_VERSION=$(uname -r)
    fi
    
    info "Detected OS: $OS $OS_VERSION"
}

detect_init_system() {
    if command -v systemctl &> /dev/null && systemctl --version &> /dev/null 2>&1; then
        INIT_SYSTEM="systemd"
    elif [ -f /etc/init.d/cron ]; then
        INIT_SYSTEM="sysvinit"
    else
        INIT_SYSTEM="unknown"
    fi
    
    info "Init system: $INIT_SYSTEM"
}

prompt_api_key() {
    if [ -n "$CC_API_KEY" ]; then
        API_KEY="$CC_API_KEY"
        info "Using API key from environment variable"
    else
        echo ""
        echo -e "${YELLOW}Please enter your Workspace API Key:${NC}"
        echo "(You can find this in your CronCommander dashboard under Settings)"
        echo ""
        read -p "API Key: " API_KEY
        
        if [ -z "$API_KEY" ]; then
            error "API key is required"
        fi
    fi
}

create_user() {
    # We create cc-agent-user regardless of mode, as system mode might use it for unprivileged jobs.
    if ! id "$AGENT_USER" &>/dev/null; then
        info "Creating $AGENT_USER user and group..."
        $SUDO groupadd -r $AGENT_USER 2>/dev/null || true
        $SUDO useradd -r -g $AGENT_USER -s /usr/sbin/nologin -d /var/lib/croncommander -M $AGENT_USER
        success "Created $AGENT_USER user (no login shell, no password)"
    else
        info "$AGENT_USER user already exists"
    fi

    # Create working directory
    info "Creating working directory..."
    $SUDO mkdir -p /var/lib/croncommander
    $SUDO chown $AGENT_USER:$AGENT_USER /var/lib/croncommander
    
    # In System mode, root needs access too (default 755 is usually fine, but let's be strict but allow group)
    $SUDO chmod 770 /var/lib/croncommander
    # Add root to agent group if needed? root strictly has access anyway.
}

setup_cron_access() {
    if [ "$MODE" = "system" ]; then
        info "Configuring System Mode (Root)..."
        # Create cron file owned by root
        $SUDO touch /etc/cron.d/croncommander
        $SUDO chown root:root /etc/cron.d/croncommander
        $SUDO chmod 644 /etc/cron.d/croncommander
        success "System cron file created at /etc/cron.d/croncommander"
    else
        info "Configuring User Mode ($AGENT_USER)..."
        # Ensure agent user can use crontab
        # Remove /etc/cron.d/croncommander if it exists to avoid conflicts/confusion
        if [ -f /etc/cron.d/croncommander ]; then
            warn "Removing existing system cron file /etc/cron.d/croncommander (switching to User Mode)"
            $SUDO rm -f /etc/cron.d/croncommander
        fi
        
        # Verify crontab access
        if [ -f /etc/cron.allow ]; then
             if ! grep -q "$AGENT_USER" /etc/cron.allow; then
                 warn "User $AGENT_USER might need to be added to /etc/cron.allow"
             fi
        fi
    fi
}


create_directories() {
    info "Creating config directories..."
    $SUDO mkdir -p "$CONFIG_DIR"
    $SUDO mkdir -p "$INSTALL_DIR"
    
    # Config dir ownership
    # User mode: owned by agent user
    # System mode: owned by root
    if [ "$MODE" = "system" ]; then
        $SUDO chown root:root "$CONFIG_DIR"
    else
        $SUDO chown $AGENT_USER:$AGENT_USER "$CONFIG_DIR"
    fi
    $SUDO chmod 750 "$CONFIG_DIR"
}

download_agent() {
    info "Downloading cc-agent..."
    
    # For local development, check if binary exists locally
    if [ -f "./cc-agent" ]; then
        info "Using local cc-agent binary"
        $SUDO cp ./cc-agent "$INSTALL_DIR/cc-agent"
    elif [ -f "/tmp/cc-agent" ]; then
        info "Using /tmp/cc-agent binary"
        $SUDO cp /tmp/cc-agent "$INSTALL_DIR/cc-agent"
    else
        # Download from URL
        if command -v curl &> /dev/null; then
            $SUDO curl -sSL -o "$INSTALL_DIR/cc-agent" "$DOWNLOAD_URL" || {
                warn "Download failed. For local development, build cc-agent manually."
                error "Failed to download cc-agent"
            }
        elif command -v wget &> /dev/null; then
            $SUDO wget -qO "$INSTALL_DIR/cc-agent" "$DOWNLOAD_URL" || {
                warn "Download failed. For local development, build cc-agent manually."
                error "Failed to download cc-agent"
            }
        else
            error "Neither curl nor wget found. Please install one and try again."
        fi
    fi
    
    $SUDO chmod 755 "$INSTALL_DIR/cc-agent"
    $SUDO chown root:root "$INSTALL_DIR/cc-agent"
    success "cc-agent installed to $INSTALL_DIR/cc-agent"
}

create_config() {
    info "Creating configuration file..."
    
    $SUDO tee "$CONFIG_FILE" > /dev/null << EOF
# CronCommander Agent Configuration
# Generated by install.sh on $(date)

# Workspace API key for authentication
api_key: ${API_KEY}

# WebSocket server URL
server_url: ${SERVER_URL}

# Execution Mode
execution_mode: ${MODE}
EOF
    
    if [ "$MODE" = "system" ]; then
        $SUDO chown root:root "$CONFIG_FILE"
    else
        $SUDO chown $AGENT_USER:$AGENT_USER "$CONFIG_FILE"
    fi
    $SUDO chmod 640 "$CONFIG_FILE"
    success "Configuration saved to $CONFIG_FILE"
}

install_systemd_service() {
    info "Installing systemd service..."
    
    SERVICE_USER="$AGENT_USER"
    SERVICE_GROUP="$AGENT_USER"
    
    if [ "$MODE" = "system" ]; then
        SERVICE_USER="root"
        SERVICE_GROUP="root"
    fi
    
    $SUDO tee "$SERVICE_FILE" > /dev/null << EOF
[Unit]
Description=CronCommander Agent
Documentation=https://croncommander.com/docs
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
ExecStart=/usr/local/bin/cc-agent daemon --config /etc/croncommander/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/tmp /var/lib/croncommander /etc/croncommander ${CONFIG_DIR}
EOF

    # Need write access to /etc/cron.d/croncommander ONLY in system mode
    if [ "$MODE" = "system" ]; then
        # Append ReadWritePaths for cron.d
        # We need to sed/append because heredoc interpolation with complex logic is messy or we can just append
        # Easier: Modify the file in place
        $SUDO sed -i 's|ReadWritePaths=.*|& /etc/cron.d/croncommander|' "$SERVICE_FILE"
    fi

    echo "
[Install]
WantedBy=multi-user.target
" | $SUDO tee -a "$SERVICE_FILE" > /dev/null
    
    # Reload daemon if systemd is running
    if systemctl list-units >/dev/null 2>&1; then
        $SUDO systemctl daemon-reload
        $SUDO systemctl enable cc-agent
        $SUDO systemctl restart cc-agent
        success "Systemd service installed and started (User: $SERVICE_USER)"
    else
        # Systemd installed but not running (e.g. docker build)
        # Just enable it manually via symlink if we can or try systemctl enable allowed
        # 'systemctl enable' might work even if not booted?
        $SUDO systemctl enable cc-agent || warn "Could not enable systemd service (systemd not running?)"
        success "Systemd service installed (will start on boot)"
    fi
}


verify_installation() {
    echo ""
    info "Verifying installation..."
    
    if [ "$INIT_SYSTEM" = "systemd" ]; then
        sleep 2
        if systemctl is-active --quiet cc-agent; then
            success "cc-agent is running!"
        else
            warn "cc-agent service may not be running correctly"
            echo "Check logs: journalctl -u cc-agent -e"
        fi
    fi
    
    echo ""
    if [ "$MODE" = "system" ]; then
        echo -e "${RED}INSTALLED IN SYSTEM MODE (ROOT)${NC}"
        echo "The agent has root privileges and manages /etc/cron.d/croncommander."
    else
        echo -e "${GREEN}INSTALLED IN USER MODE ($AGENT_USER)${NC}"
        echo "The agent runs as an unprivileged user and manages its own crontab."
    fi
    echo ""
}


main() {
    parse_args "$@"
    print_banner
    check_root
    detect_os
    detect_init_system
    prompt_api_key
    
    info "Installation Mode: ${MODE^^}"
    if [ "$MODE" = "system" ]; then
        warn "You have selected SYSTEM MODE. The agent will run as ROOT."
    fi
    
    create_user
    create_directories
    download_agent
    setup_cron_access
    create_config
    
    if [ "$INIT_SYSTEM" = "systemd" ]; then
        install_systemd_service
    elif [ "$INIT_SYSTEM" = "sysvinit" ]; then
        install_sysvinit_service
    else
        warn "Init system not detected or unsupported."
        info "Attempting to start agent directly (background)..."
        
        LOG_FILE="/var/log/cc-agent.log"
        CMD="/usr/local/bin/cc-agent daemon --config /etc/croncommander/config.yaml"
        
        # Touch log file
        $SUDO touch "$LOG_FILE"
        $SUDO chmod 666 "$LOG_FILE" 

        if [ "$MODE" = "system" ]; then
             $SUDO nohup $CMD > "$LOG_FILE" 2>&1 &
        else
            # Attempt to run as agent user
            if command -v runuser &> /dev/null; then
                $SUDO nohup runuser -u $AGENT_USER -- $CMD > "$LOG_FILE" 2>&1 &
            elif command -v su &> /dev/null; then
                $SUDO nohup su -s /bin/bash $AGENT_USER -c "$CMD" > "$LOG_FILE" 2>&1 &
            else
                # STRICT SECURITY: Do not fallback to root in User Mode.
                # If we can't drop privileges, we must fail.
                error "Could not switch to user '$AGENT_USER'. Missing 'runuser' or 'su'. Aborting to prevent root execution."
                exit 1
            fi
        fi
        success "Agent started in background"
    fi
    
    verify_installation
}

main "$@"
