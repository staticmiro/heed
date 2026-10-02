System monitoring daemon written in Go.

It operates as a single binary with a single configuration file (`config.toml`), storing events in a local JSONL file (`events.jsonl`). It does not require external databases, application servers, or web dashboards.

**Footprint:**
- **Binary Size:** ~10 MB
- **Memory Usage:** ~10-15 MB RAM
- **CPU Usage:** < 0.1%

## Features

**System Metrics**
- CPU, Memory, Swap, and Disk Space usage.
- CPU Temperature and Load Average.
- System Uptime.

**Network Monitoring**
- **HTTP/HTTPS**: Status codes and response times.
- **TCP**: Port connectivity and latency.
- **DNS**: Host resolution checks.
- **Ping**: ICMP echo requests (auto-detects OS capabilities).
- **SSL/TLS**: Certificate expiration warnings (days left).

**System & Infrastructure Checks**
- **Processes**: Ensure minimum number of processes are running (matched by name or cmdline).
- **Systemd**: Verify services are active.
- **Files**: Check file existence, size, and modification age (useful for backup monitoring).
- **Commands**: Run custom shell commands and alert on non-zero exit codes or timeouts.
- **Docker**: Direct UNIX socket integration to monitor container state, health, and restart loops.

**Intelligent Alerting**
- **Trend Detection**: Observes numeric metrics (like CPU/RAM) and calculates ETA to critical thresholds, alerting you *before* an outage happens.
- **Cooldowns**: Prevents alert spam by enforcing cooldown periods.
- **Recovery Notifications**: Alerts you when a failing service recovers.
- **Event Logging**: All state changes are written to a rotated, append-only JSONL log (`events.jsonl`).

**Notification Channels**
- **Stdout**: CLI output.
- **Telegram**: Direct bot messages.
- **Discord**: Webhook integration.
- **Slack**: Webhook integration.
- **SMTP**: Email notifications.
- **Generic Webhook**: HTTP POST with optional HMAC-SHA256 signature for custom integrations.

## Stack

- [Golang](https://go.dev)
- [Docker](https://www.docker.com) - optional containerization

## Installation & Running

Choose your preferred deployment method. `heed` runs identically in both scenarios, but native installation is recommended for accessing host-level metrics like `systemd` and `fail2ban`.

### Option A: Native Systemd (Recommended)

Download and install the pre-compiled binary directly to your system. This script automatically configures a systemd service.

```bash
# 1. Install heed (downloads binary to /usr/local/bin and sets up heed.service)
curl -sSL https://raw.githubusercontent.com/staticmiro/heed/main/install.sh | bash

# 2. Setup your configuration
sudo mkdir -p /etc/heed
sudo curl -sSL -o /etc/heed/config.toml https://raw.githubusercontent.com/staticmiro/heed/main/config.example.toml
# Edit /etc/heed/config.toml with your tokens and chat IDs

# 3. Enable and start the daemon
sudo systemctl enable --now heed
```

### Option B: Docker

If you prefer containerization:

```bash
git clone https://github.com/staticmiro/heed.git
cd heed
cp config.example.toml config.toml
# Edit config.toml
docker compose up -d
```

### Option C: Build from Source

```bash
git clone https://github.com/staticmiro/heed.git
cd heed
go build -o heed ./cmd/heed
./heed -config config.example.toml
```

## CLI Commands

`heed` provides the following commands for managing the daemon and checking its status:

- `heed -config config.toml` — Run the daemon.
- `heed status -config config.toml` — One-shot status check (prints all current metric states).
- `heed history -n 20 -config config.toml` — View the last 20 state-change events.
- `heed validate -config config.toml` — Verify your configuration syntax.
- `heed test -config config.toml` — Send a test notification to all configured channels.
- `heed reload` — Reload daemon configuration dynamically via SIGHUP (without downtime).
- `heed silence -config config.toml [check_name] 30m` — Silence alerts (globally or for a specific check) for a given duration.

## Available Checks

`heed` comes with a battery of zero-dependency checks. No extra plugins to download:

- **Network**: HTTP(S) (advanced POST/JSON matching, latency thresholds), TCP, DNS, Ping, SSL expiry.
- **System**: CPU, Memory, Swap, Disk, Temperature, Load, Uptime, Public IP change.
- **Services**: Process limits (min count, max CPU, max Memory), Systemd state, Docker containers.
- **Filesystem**: File age/size, Backup freshness (newest file age), Log file monitoring (regex match with offset tracking).
- **Security & Hardware**: OS Updates (apt), Fail2Ban (banned IPs), SMART disk health.
- **Custom**: Arbitrary command execution (`type = "command"`).

## Config

Configuration is done entirely via a single TOML file. See [config.example.toml](config.example.toml) for all options and examples of how to set up checks and notifiers.
