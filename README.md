![GitHub stars](https://img.shields.io/github/stars/croncommander/cc-agent?style=social)
![License](https://img.shields.io/github/license/croncommander/cc-agent)
[![cc-spec](https://img.shields.io/badge/spec-cc--spec-blue)](https://github.com/croncommander/cc-spec)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev)

# cc-agent

> Part of the **CronCommander** project — a control plane for cron jobs across your infrastructure.

CronCommander Agent is a lightweight Go binary that connects your servers to the CronCommander control plane. It gives you visibility and control over cron jobs without replacing cron itself.

## Features

- **Daemon Mode**: Long-lived agent using short HTTPS requests to `gateway.croncommander.com`
- **Cron Synchronization**: Receives job definitions from the server and writes them to `/etc/cron.d/croncommander`
- **Execution Wrapper**: Wraps each job to capture stdout/stderr, exit codes, and timing
- **Dual Modes**: User Mode (unprivileged, manages its own crontab) or System Mode (root, manages `/etc/cron.d`)
- **Security Hardened**: No-new-privileges, minimal environment, controlled working directory
- **Version Injection**: Version is embedded at compile time via `-ldflags`

## Installation

### Quick Install (Linux / FreeBSD / macOS)

```bash
curl -sSL https://croncommander.com/install.sh | bash
```

Set your API key and gateway URL via environment variables:

```bash
CC_API_KEY="your-api-key" \
  curl -sSL https://croncommander.com/install.sh | bash
```

The installer auto-detects your OS and architecture and downloads the correct binary.

### Manual Installation

1. **Download the binary** from [releases/](releases/) or build from source:
   ```bash
   make build
   ```

2. **Install the binary**:
   ```bash
   sudo cp bin/cc-agent-*-linux-amd64 /usr/local/bin/cc-agent
   sudo chmod 755 /usr/local/bin/cc-agent
   ```

3. **Create config**:
   ```bash
   sudo mkdir -p /etc/croncommander
   sudo tee /etc/croncommander/config.yaml > /dev/null <<EOF
   api_key: your-api-key
   server_url: https://gateway.croncommander.com
   state_file: /var/lib/croncommander/agent-state.json
   spool_dir: /var/lib/croncommander/spool
   EOF
   ```

4. **Run**:
   ```bash
   cc-agent daemon --config /etc/croncommander/config.yaml
   ```

## Usage

### Daemon Mode

The agent runs as a daemon, polling the CronCommander gateway with a randomized
30-90 second interval:

```bash
cc-agent daemon --config /etc/croncommander/config.yaml
```

### Exec Mode

The `exec` subcommand wraps job execution for reporting. It is called automatically by cron — not by users directly:

```bash
cc-agent exec --job-id abc123 -- /path/to/script.sh arg1 arg2
```

This captures:
- Start time and duration
- Exit code
- stdout/stderr output (capped at 256KB each)
- Executing user and UID

### Version

```bash
cc-agent --version
```

## Security

CronCommander Agent is designed with security as a first-class concern:

| Feature | Description |
|---------|-------------|
| **User Mode (default)** | Agent runs as `cc-agent-user`, manages its own crontab only |
| **System Mode** | Opt-in root mode for `/etc/cron.d` management |
| **No-new-privileges** | Uses `PR_SET_NO_NEW_PRIVS` to prevent setuid escalation (Linux 3.5+) |
| **Minimal environment** | Only PATH, HOME, LANG, and LC_ALL are set |
| **Controlled working directory** | Jobs execute in `/var/lib/croncommander` |
| **Systemd hardening** | ProtectSystem=strict, ProtectHome=yes, NoNewPrivileges=yes |

For more details, see [Security Documentation](https://croncommander.com/docs.html#security).

## Configuration

The agent is configured via YAML file:

```yaml
# Agent version
version: 1.1.0

# Workspace API key for authentication
api_key: your-workspace-api-key

# HTTPS gateway origin
server_url: https://gateway.croncommander.com

# Agent-scoped credential, manifest state, and durable report queue
state_file: /var/lib/croncommander/agent-state.json
spool_dir: /var/lib/croncommander/spool

# Local development only; leave false in production
allow_insecure_http: false

# Execution mode: "user" (default) or "system"
execution_mode: user
```

Default config location: `/etc/croncommander/config.yaml`

## Building

### Requirements

- Go 1.23+
- The root `VERSION` file (read automatically by the Makefile)

### Build

```bash
make build
```

Produces versioned binaries in `bin/`, e.g. `cc-agent-1-1-0-linux-amd64`.

### Publish Release

```bash
make publish
```

Copies binaries to `releases/` and `../build/` for local development.

### Testing

```bash
go test ./...
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Host/Container                                         │
│                                                         │
│  ┌──────────────┐    ┌─────────────────────────────┐   │
│  │  cc-agent    │◄───│  /etc/cron.d/croncommander  │   │
│  │  (daemon)    │    └─────────────────────────────┘   │
│  │              │                                       │
│  │ HTTPS poller │    ┌─────────────────────────────┐   │
│  │ and spool    │◄───│  cc-agent exec              │   │
│  │              │    │  (reports via Unix socket)  │   │
│  └──────┬───────┘    └─────────────────────────────┘   │
│         │                                               │
└─────────┼───────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────┐
│  CronCommander      │
│  Gateway             │
│  (gateway.           │
│   croncommander.com) │
└─────────────────────┘
```

## License

Apache-2.0 — see [LICENSE](LICENSE)

## Project

Part of the **CronCommander** project — a control plane for cron jobs across your infrastructure.

- Website: https://croncommander.com
- Documentation: https://croncommander.com/docs.html
- Pricing: https://croncommander.com/pricing.html
- GitHub: https://github.com/croncommander
