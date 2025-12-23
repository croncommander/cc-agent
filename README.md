![GitHub stars](https://img.shields.io/github/stars/croncommander/cc-agent?style=social)
![License](https://img.shields.io/github/license/croncommander/cc-agent)
[![cc-spec](https://img.shields.io/badge/spec-cc--spec-blue)](https://github.com/croncommander/cc-spec)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev)

# cc-agent

> Part of the **CronCommander** project — a centralized control plane for cron jobs.

CronCommander Agent is a lightweight Go daemon that connects cron-based systems
to the CronCommander control plane.

It provides the foundation for centralized visibility and management of cron jobs
across servers, containers, and environments.

## Features

- **Daemon Mode**: Runs as a long-lived agent connecting via WebSocket to the CronCommander server
- **Cron Synchronization**: Receives job definitions from the server and writes them to `/etc/cron.d/croncommander`
- **Execution Wrapper**: Wraps job execution to capture stdout/stderr, exit codes, and timing
- **Privileged Separation**: Jobs run as an unprivileged `ccrunner` user, never as root
- **Security Hardened**: Includes no-new-privileges, minimal environment, and controlled working directory

## Installation

### Quick Install (Linux with systemd)

```bash
curl -sSL https://croncommander.com/install.sh | bash
```

Or set your API key and server URL via environment variables:

```bash
CC_API_KEY="your-api-key" CC_SERVER_URL="ws://your-server:8081/agent" \
  curl -sSL https://croncommander.com/install.sh | bash
```

### Manual Installation

1. **Build from source**:
   ```bash
   cd cc-agent
   go build -o cc-agent .
   ```

2. **Install the binary**:
   ```bash
   sudo cp cc-agent /usr/local/bin/
   sudo chmod 755 /usr/local/bin/cc-agent
   ```

3. **Create config**:
   ```bash
   sudo mkdir -p /etc/croncommander
   sudo tee /etc/croncommander/config.yaml > /dev/null <<EOF
   api_key: your-api-key
   server_url: ws://your-server:8081/agent
   EOF
   ```

4. **Run**:
   ```bash
   cc-agent daemon --config /etc/croncommander/config.yaml
   ```

## Usage

### Daemon Mode

The agent runs as a daemon, maintaining a WebSocket connection to the CronCommander server:

```bash
cc-agent daemon --config /etc/croncommander/config.yaml
```

### Exec Mode

The `exec` subcommand wraps job execution for reporting. It is called automatically by cron—not by users directly:

```bash
cc-agent exec --job-id abc123 -- /path/to/script.sh arg1 arg2
```

This captures:
- Start time and duration
- Exit code
- stdout/stderr output (capped at 256KB each)
- Executing user and UID

## Security

CronCommander Agent is designed with security in mind:

| Feature | Description |
|---------|-------------|
| **Unprivileged execution** | Jobs run as `ccrunner`, a dedicated system user with no login shell |
| **Root rejection** | `cc-agent exec` refuses to run if UID is 0 |
| **No-new-privileges** | Uses `PR_SET_NO_NEW_PRIVS` to prevent setuid escalation (Linux 3.5+) |
| **Minimal environment** | Only PATH, HOME, LANG, and LC_ALL are set |
| **Controlled working directory** | Jobs execute in `/var/lib/croncommander` |
| **Systemd hardening** | ProtectSystem=strict, ProtectHome=yes, NoNewPrivileges=yes |

For more details, see [Security Documentation](https://croncommander.com/docs/security).

## Configuration

The agent is configured via YAML file:

```yaml
# Workspace API key for authentication
api_key: your-workspace-api-key

# WebSocket server URL
server_url: ws://localhost:8081/agent
```

Default config location: `/etc/croncommander/config.yaml`

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Host/Container                                         │
│                                                         │
│  ┌──────────────┐    ┌─────────────────────────────┐   │
│  │  cc-agent    │◄───│  /etc/cron.d/croncommander  │   │
│  │  (daemon)    │    └─────────────────────────────┘   │
│  │              │                                       │
│  │  WebSocket   │    ┌─────────────────────────────┐   │
│  │  connection  │◄───│  cc-agent exec              │   │
│  │              │    │  (reports via Unix socket)  │   │
│  └──────┬───────┘    └─────────────────────────────┘   │
│         │                                               │
└─────────┼───────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────┐
│  CronCommander      │
│  Server             │
└─────────────────────┘
```

## Development

### Requirements

- Go 1.23+
- Linux (for full security features) or macOS/Windows (limited features)

### Building

```bash
go build -o cc-agent .
```

### Testing

```bash
go test ./...
```

## License

Apache-2.0 — see [LICENSE](LICENSE)

## Project

Part of the **CronCommander** project — a centralized control plane for cron jobs.

- Website: https://croncommander.com
- Documentation: https://croncommander.com/docs
- Security: https://croncommander.com/docs/security
