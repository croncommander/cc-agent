![GitHub stars](https://img.shields.io/github/stars/croncommander/cc-agent?style=social)
![License](https://img.shields.io/github/license/croncommander/cc-agent)
[![cc-spec](https://img.shields.io/badge/spec-cc--spec-blue)](https://github.com/croncommander/cc-spec)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)

# cc-agent

> Part of the **CronCommander** project — a control plane for cron jobs across your infrastructure.

CronCommander Agent is a lightweight Go binary that connects your servers to the CronCommander control plane. It gives you visibility and control over cron jobs without replacing cron itself.

## Features

- **Daemon Mode**: Long-lived agent using short HTTPS requests to `gateway.croncommander.com`
- **Cron Synchronization**: Receives the deterministic, plan-eligible job manifest from the server and writes it to `/etc/cron.d/croncommander`. Plan enforcement stays in the control plane, so jobs suppressed after a downgrade are not deleted by the agent.
- **Cron Discovery**: Scans existing user and system cron sources for review and import
- **Execution Wrapper**: Wraps each job to capture stdout/stderr, exit codes, and timing
- **Bounded Durable Spool**: Retains reports across outages with configurable age, record, and byte limits
- **Dual Modes**: User Mode (unprivileged, manages its own crontab) or System Mode (root, manages `/etc/cron.d`)
- **Security Hardened**: No-new-privileges, minimal environment, controlled working directory
- **Version Injection**: Version is embedded at compile time via `-ldflags`

## Installation

> **Current release:** v2.3.0. Published binaries and adjacent SHA-256
> checksums are available from the GitHub Releases page.

### Quick Install (Linux)

```bash
curl -sSL https://croncommander.com/install.sh | bash
```

Set your API key and gateway URL via environment variables:

```bash
CC_API_KEY="your-api-key" \
  curl -sSL https://croncommander.com/install.sh | bash
```

The automatic service installer supports Linux and detects systemd, OpenRC, or
SysV init. FreeBSD and macOS release binaries require manual service
configuration.
Remote installs also download the adjacent `.sha256` asset and refuse to
install a binary that does not match it.

### Manual Installation

1. **Download the binary** from the published GitHub Release or build from source:
   ```bash
   make build
   ```

2. **Verify the binary**:
   ```bash
   sha256sum -c cc-agent-2-3-0-linux-amd64.sha256
   ```

3. **Install the binary**:
   ```bash
   sudo cp cc-agent-2-3-0-linux-amd64 /usr/local/bin/cc-agent
   sudo chmod 755 /usr/local/bin/cc-agent
   ```

4. **Create config**:
   ```bash
   sudo mkdir -p /etc/croncommander
   sudo tee /etc/croncommander/config.yaml > /dev/null <<EOF
   api_key: your-api-key
   server_url: https://gateway.croncommander.com
   state_file: /var/lib/croncommander/agent-state.json
   spool_dir: /var/lib/croncommander/spool
   EOF
   ```

5. **Run**:
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

### Discovery

The daemon scans after registration and then on the configured discovery
interval (24 hours by default). Run an on-demand scan with:

```bash
cc-agent discover --config /etc/croncommander/config.yaml
```

User mode scans the agent user's crontab. System mode also scans
`/etc/crontab` and `/etc/cron.d`, excluding CronCommander-managed entries.

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
# Workspace API key for authentication
api_key: your-workspace-api-key

# HTTPS gateway origin
server_url: https://gateway.croncommander.com

# Agent-scoped credential, manifest state, and durable report queue
state_file: /var/lib/croncommander/agent-state.json
spool_dir: /var/lib/croncommander/spool

# Durable report queue bounds (oldest reports are evicted first)
spool_max_bytes: 268435456
spool_max_records: 10000
spool_retention: 720h

# Local development only; leave false in production
allow_insecure_http: false

# Execution mode: "user" (default) or "system"
execution_mode: user

# Full host cron scan interval (minimum 5m)
discovery_interval: 24h
```

Default config location: `/etc/croncommander/config.yaml`

## Building

### Requirements

- Go 1.25+
- The repository `VERSION` file (read automatically by the Makefile)

### Version Review

Before every agent build, review the changes since the previous build and
increase the repository `VERSION` value using semantic versioning:

- Patch for backward-compatible fixes
- Minor for backward-compatible features
- Major for breaking changes

Do not distribute or deploy a changed agent binary under a previously used
version. The build embeds this value in the binary and reports it to
CronCommander during registration and polling.

### Build

```bash
make build
```

Produces versioned binaries and adjacent checksum files in `bin/`, e.g.
`cc-agent-2-3-0-linux-amd64` and
`cc-agent-2-3-0-linux-amd64.sha256`.

For the local Docker Compose agents, build and sync the Linux amd64 artifact:

```bash
make local-linux-amd64
```

### Publish Release

```bash
make publish
```

Copies binaries and checksums to `releases/` and `../build/` for local
development.

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
