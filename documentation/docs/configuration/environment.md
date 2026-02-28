---
sidebar_position: 2
title: Environment Variables
---

# Environment Variables

Configuration is stored in `config.toml` (created automatically on first run, or manually with `rui generate-config`). You can also use environment variables:

For the complete list (including `config.toml` keys, defaults, and notes), see [Configuration Reference](./reference).

## Server

```bash
RUI__HOST=0.0.0.0        # Listen address
RUI__PORT=7476           # Port number
RUI__BASE_URL=/rui/      # Optional: serve from subdirectory
```

## Security

```bash
RUI__SESSION_SECRET_FILE=...  # Path to file containing secret. Takes precedence over RUI__SESSION_SECRET
RUI__SESSION_SECRET=...       # Auto-generated if not set
```

## Logging

```bash
RUI__LOG_LEVEL=INFO      # Options: ERROR, DEBUG, INFO, WARN, TRACE
RUI__LOG_PATH=...        # Optional: log file path
RUI__LOG_MAX_SIZE=50     # Optional: rotate when log file exceeds N megabytes (default: 50)
RUI__LOG_MAX_BACKUPS=3   # Optional: retain N rotated files (default: 3, 0 keeps all)
```

When `logPath` is set the server writes to disk using size-based rotation. Adjust `logMaxSize` and `logMaxBackups` in `config.toml` or the corresponding environment variables to control the rotation thresholds and retention.

## Storage

```bash
RUI__DATA_DIR=...        # Optional: custom data directory (default: next to config)
```

## Cross-Seed

```bash
RUI__CROSS_SEED_RECOVER_ERRORED_TORRENTS=false  # Optional: recover errored/missingFiles torrents; can add ~25+ minutes per torrent (default: false)
```

## Tracker Icons

```bash
RUI__TRACKER_ICONS_FETCH_ENABLED=false  # Optional: set to false to disable remote tracker icon fetching (default: true)
```

## Updates

```bash
RUI__CHECK_FOR_UPDATES=false  # Optional: disable update checks and UI indicators (default: true)
```

## Profiling (pprof)

```bash
RUI__PPROF_ENABLED=true  # Optional: enable pprof server on :6060 (default: false)
```

## Metrics

```bash
RUI__METRICS_ENABLED=true      # Optional: enable Prometheus metrics (default: false)
RUI__METRICS_HOST=127.0.0.1    # Optional: metrics server bind address (default: 127.0.0.1)
RUI__METRICS_PORT=9074         # Optional: metrics server port (default: 9074)
RUI__METRICS_BASIC_AUTH_USERS=user:hash  # Optional: basic auth for metrics (bcrypt hashed)
```

## Authentication

```bash
RUI__AUTH_DISABLED=true                 # Optional: disable built-in auth (default: false)
RUI__I_ACKNOWLEDGE_THIS_IS_A_BAD_IDEA=true  # Required confirmation to actually disable auth
RUI__AUTH_DISABLED_ALLOWED_CIDRS=127.0.0.1/32,192.168.1.0/24  # Required when auth is disabled (IPs or CIDRs)
```

Built-in authentication is disabled only when:

- `RUI__AUTH_DISABLED=true`
- `RUI__I_ACKNOWLEDGE_THIS_IS_A_BAD_IDEA=true`
- `RUI__AUTH_DISABLED_ALLOWED_CIDRS` is set to one or more allowed IPs/CIDR ranges

If auth is disabled and `RUI__AUTH_DISABLED_ALLOWED_CIDRS` is missing or invalid, rui refuses to start and rejects invalid live reloads.

`RUI__AUTH_DISABLED_ALLOWED_CIDRS` accepts comma-separated entries. Each entry may be a canonical CIDR (`192.168.1.0/24`) or a single IP (`10.0.0.5`, treated as `/32` or `/128`).

Non-canonical CIDRs with host bits set (for example `10.0.0.5/8`) are rejected.

`RUI__OIDC_ENABLED=true` cannot be combined with auth-disabled mode.

Only use this when rui runs behind a reverse proxy that already handles authentication (e.g., Authelia, Authentik, Caddy with forward_auth). See the [Configuration Reference](./reference#authentication) for a full explanation of the risks.

## External Programs

Configure the allow list from `config.toml`; there is no environment override to keep it read-only from the UI.

## Default Locations

- **Linux/macOS**: `~/.config/rui/config.toml`
- **Windows**: `%APPDATA%\rui\config.toml`
