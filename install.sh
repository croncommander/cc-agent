#!/bin/bash
set -e

# CronCommander Agent Installer
# Usage: curl -sSL https://croncommander.com/install.sh | bash

VERSION="${CC_VERSION:-latest}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/croncommander"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
SERVICE_FILE="/etc/systemd/system/cc-agent.service"
DOWNLOAD_URL="${CC_DOWNLOAD_URL:-https://github.com/croncommander/cc-agent/releases/download/${VERSION}/cc-agent-linux-amd64}"
SERVER_URL="${CC_SERVER_URL:-ws://localhost:8081/agent}"

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
    echo "║                       Installer v1.0                          ║"
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
    # SECURITY: ccrunner is a dedicated unprivileged user for job execution.
    # Security properties:
    #   - System user (-r): No expiry, UID in system range
    #   - No login shell (/usr/sbin/nologin): Prevents interactive login attacks
    #   - No password: Account is locked, cannot authenticate
    #   - No sudo access: Never added to sudoers
    #   - Dedicated home dir: For working directory isolation
    if ! id "ccrunner" &>/dev/null; then
        info "Creating ccrunner user and group..."
        $SUDO groupadd -r ccrunner 2>/dev/null || true
        $SUDO useradd -r -g ccrunner -s /usr/sbin/nologin -d /var/lib/croncommander -M ccrunner
        success "Created ccrunner user (no login shell, no password)"
    else
        info "ccrunner user already exists"
    fi

    # Create working directory with restrictive permissions
    info "Creating ccrunner working directory..."
    $SUDO mkdir -p /var/lib/croncommander
    $SUDO chown ccrunner:ccrunner /var/lib/croncommander
    $SUDO chmod 750 /var/lib/croncommander

    # Pre-create cron file owned by ccrunner so daemon can run unprivileged
    info "Creating cron file owned by ccrunner..."
    $SUDO touch /etc/cron.d/croncommander
    $SUDO chown ccrunner:ccrunner /etc/cron.d/croncommander
    $SUDO chmod 644 /etc/cron.d/croncommander
    success "ccrunner security setup complete"
}


create_directories() {
    info "Creating directories..."
    $SUDO mkdir -p "$CONFIG_DIR"
    $SUDO mkdir -p "$INSTALL_DIR"
    $SUDO chown root:ccrunner "$CONFIG_DIR"
    $SUDO chmod 750 "$CONFIG_DIR"
    success "Directories created"
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
                warn "Run: cd cc-agent && go build -o /tmp/cc-agent ."
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
EOF
    
    $SUDO chown root:ccrunner "$CONFIG_FILE"
    $SUDO chmod 640 "$CONFIG_FILE"
    success "Configuration saved to $CONFIG_FILE"
}

install_systemd_service() {
    info "Installing systemd service..."
    
    $SUDO tee "$SERVICE_FILE" > /dev/null << 'EOF'
[Unit]
Description=CronCommander Agent
Documentation=https://croncommander.com/docs
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
# SECURITY: Daemon runs as unprivileged ccrunner user.
# The cron file /etc/cron.d/croncommander is pre-created and owned by ccrunner.
User=ccrunner
Group=ccrunner
ExecStart=/usr/local/bin/cc-agent daemon --config /etc/croncommander/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/tmp /etc/cron.d/croncommander /var/lib/croncommander

[Install]
WantedBy=multi-user.target
EOF
    
    $SUDO systemctl daemon-reload
    $SUDO systemctl enable cc-agent
    $SUDO systemctl start cc-agent
    
    success "Systemd service installed and started"
}


install_sysvinit_service() {
    info "Installing SysVinit service..."
    
    $SUDO tee /etc/init.d/cc-agent > /dev/null << 'EOF'
#!/bin/bash
### BEGIN INIT INFO
# Provides:          cc-agent
# Required-Start:    $network $remote_fs $syslog
# Required-Stop:     $network $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: CronCommander Agent
# Description:       CronCommander Agent daemon
### END INIT INFO

DAEMON=/usr/local/bin/cc-agent
DAEMON_ARGS="daemon --config /etc/croncommander/config.yaml"
PIDFILE=/var/run/cc-agent.pid
LOGFILE=/var/log/cc-agent.log

case "$1" in
    start)
        echo "Starting cc-agent..."
        nohup $DAEMON $DAEMON_ARGS >> $LOGFILE 2>&1 &
        echo $! > $PIDFILE
        ;;
    stop)
        echo "Stopping cc-agent..."
        if [ -f $PIDFILE ]; then
            kill $(cat $PIDFILE) 2>/dev/null
            rm -f $PIDFILE
        fi
        ;;
    restart)
        $0 stop
        sleep 2
        $0 start
        ;;
    status)
        if [ -f $PIDFILE ] && kill -0 $(cat $PIDFILE) 2>/dev/null; then
            echo "cc-agent is running"
        else
            echo "cc-agent is not running"
            exit 1
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
EOF
    
    $SUDO chmod 755 /etc/init.d/cc-agent
    $SUDO update-rc.d cc-agent defaults 2>/dev/null || true
    $SUDO /etc/init.d/cc-agent start
    
    success "SysVinit service installed and started"
}

verify_installation() {
    echo ""
    info "Verifying installation..."
    
    if [ "$INIT_SYSTEM" = "systemd" ]; then
        sleep 2
        if systemctl is-active --quiet cc-agent; then
            success "cc-agent is running!"
            echo ""
            echo -e "${GREEN}Installation complete!${NC}"
            echo ""
            echo "Useful commands:"
            echo "  Check status:    systemctl status cc-agent"
            echo "  View logs:       journalctl -u cc-agent -f"
            echo "  Restart:         systemctl restart cc-agent"
            echo ""
        else
            warn "cc-agent service may not be running correctly"
            echo "Check logs: journalctl -u cc-agent -e"
        fi
    else
        sleep 2
        if [ -f /var/run/cc-agent.pid ]; then
            success "cc-agent appears to be running"
            echo ""
            echo "Useful commands:"
            echo "  Check status:    /etc/init.d/cc-agent status"
            echo "  View logs:       tail -f /var/log/cc-agent.log"
            echo "  Restart:         /etc/init.d/cc-agent restart"
            echo ""
        else
            warn "cc-agent may not be running correctly"
            echo "Check logs: cat /var/log/cc-agent.log"
        fi
    fi
    
    echo "Your agent should appear in the CronCommander dashboard shortly."
    echo ""
    echo -e "${YELLOW}Security Notes:${NC}"
    echo "  • Jobs execute as unprivileged 'ccrunner' user"
    echo "  • ccrunner has no login shell and no password"
    echo "  • Agent cannot run jobs as root (blocked by design)"
    echo "  • Do NOT add ccrunner to sudoers or privileged groups"
    echo ""
}


main() {
    print_banner
    check_root
    detect_os
    detect_init_system
    prompt_api_key
    create_user
    create_directories
    download_agent
    create_config
    
    if [ "$INIT_SYSTEM" = "systemd" ]; then
        install_systemd_service
    else
        install_sysvinit_service
    fi
    
    verify_installation
}

main "$@"
