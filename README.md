<div align="center">

# go-data

**A lightweight, self-hosted server monitoring dashboard — one Go binary, zero bloat.**

CPU · RAM · disk · network · temperature · Docker · SSH attempts · incoming connections

[![Go Report Card](https://goreportcard.com/badge/github.com/gonzxa/go-data)](https://goreportcard.com/report/github.com/gonzxa/go-data)
[![Docker Image](https://img.shields.io/docker/v/gonzxa/go-data?label=docker&sort=semver)](https://hub.docker.com/r/gonzxa/go-data)
[![Go Version](https://img.shields.io/github/go-mod/go-version/gonzxa/go-data)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

---

## Why go-data

Netdata-style visibility without the footprint. `go-data` ships as a single
statically-linked binary plus a static web dashboard — no agent sprawl, no
external database required to get started, no JavaScript build step.

- **One binary, one dashboard.** Point a browser at it and you're done.
- **Zero mandatory dependencies.** Metrics work out of the box, in memory.
  Add InfluxDB only if you want history beyond ~5 minutes.
- **Opt-in everything.** Docker stats, SSH watch, connection watch, and auth
  are all off by default and enabled with a single environment variable.
- **Degrades gracefully.** Missing a mount (Docker socket, `/var/log`,
  `/proc`)? The relevant tab just shows no data instead of crashing.
- **A real JSON API.** The dashboard is just a client of the same endpoints
  you can query yourself.

## Features

| | |
|---|---|
| 🖥️ **CPU** | Usage, per-core breakdown, user/system/iowait/steal, top processes |
| 🧠 **RAM & swap** | Used/available/free/cached, swap usage |
| 📈 **Load average** | 1/5/15 min, normalized against core count |
| 💾 **Disk** | Space per mount, I/O throughput, IOPS, latency |
| 🌐 **Network** | Per-interface throughput, packets, errors, drops |
| 🌡️ **Temperature** | Per thermal zone, primary CPU zone detection |
| 🐳 **Docker** | Live container list with CPU/mem/net/block I/O and port mappings |
| 🔐 **SSH watch** | Login attempts (IP, user, success/fail) parsed from the auth log |
| 🔌 **Connection watch** | New inbound connections per port — handy for game servers |
| 🔁 **Uptime history** | Reboot sessions reconstructed from `/proc/uptime` |
| 🚨 **Alarms** | Threshold-based warnings for CPU, RAM, disk, iowait/steal, temperature |
| 🔒 **Basic Auth** | Optional, protects both the dashboard and the API |

## Quick start

```bash
docker compose up -d
```

Dashboard: **http://localhost:19245**

Using the published image directly, no clone required:

```yaml
services:
  app:
    image: gonzxa/go-data:latest
    ports:
      - "19245:9000"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro   # Docker tab
      - /var/log:/var/log:ro                            # SSH watch
      - /proc:/hostproc:ro                               # connection watch
    environment:
      - USE_INFLUX=true
      - INFLUX_URL=http://influx:8086
      - INFLUX_TOKEN=change-me
      - INFLUX_ORG=my-org
      - INFLUX_BUCKET=metrics
      - DOCKER_ENABLED=true
      - SSH_WATCH_ENABLED=true
      - CONN_WATCH_ENABLED=true
      - AUTH_ENABLED=true
      - AUTH_USER=admin
      - AUTH_PASSWORD=change-me
    depends_on:
      - influx

  influx:
    image: influxdb:2.7
    ports:
      - "19246:8086"
    environment:
      - DOCKER_INFLUXDB_INIT_MODE=setup
      - DOCKER_INFLUXDB_INIT_USERNAME=admin
      - DOCKER_INFLUXDB_INIT_PASSWORD=change-me
      - DOCKER_INFLUXDB_INIT_ORG=my-org
      - DOCKER_INFLUXDB_INIT_BUCKET=metrics
      - DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=change-me
```

Any of the three volume mounts can be dropped if you don't need that tab —
the app degrades gracefully instead of crashing. Replace every `change-me`
before exposing this to the internet.

### Building from source

```bash
git clone https://github.com/gonzxa/go-data.git
cd go-data
go build -o go-data ./cmd/server
./go-data
```

## Configuration

All settings can be set as environment variables (they override
`config.json`, so no rebuild is needed to change behavior).

| Variable | What it does | Default |
|---|---|---|
| `USE_INFLUX` | Also persist metrics to InfluxDB (long-term history). Without it, metrics still work but only keep ~5 minutes in memory. | `false` |
| `INFLUX_URL` | InfluxDB URL. | — |
| `INFLUX_TOKEN` | InfluxDB auth token. | — |
| `INFLUX_ORG` | InfluxDB org. | — |
| `INFLUX_BUCKET` | InfluxDB bucket. | — |
| `DOCKER_ENABLED` | Show the Docker containers tab. | `true` |
| `SSH_WATCH_ENABLED` | Log SSH login attempts (IP, user, success/fail) from the auth log. | `false` |
| `SSH_LOG_PATH` | Custom auth log path. Leave empty to auto-detect. | auto |
| `CONN_WATCH_ENABLED` | Log incoming connections (IP + port) — useful for game servers. | `false` |
| `CONN_WATCH_AUTO` | Auto-detect which ports to watch (anything listening on a public address) instead of listing them by hand. | `true` |
| `CONN_WATCH_PORTS` | Extra ports to always watch, comma-separated (e.g. `25565,7777`). | none |
| `DISK_MOUNTS` | Which mount points to show in the Disk tab, comma-separated. Empty = auto-detect. | auto |
| `DATA_DIR` | Where local logs (reboot history, SSH/connection events) are stored. | `./data` |
| `HOST_PROC_PATH` | `/proc` path used for connection watching and the CPU top-processes list. Auto-detects `/hostproc` if mounted. | auto |
| `AUTH_ENABLED` | Require HTTP Basic Auth for the dashboard and API. Needs `AUTH_USER`/`AUTH_PASSWORD` set, or the app refuses to start. | `false` |
| `AUTH_USER` | Basic Auth username. | — |
| `AUTH_PASSWORD` | Basic Auth password. | — |

`SSH_WATCH_ENABLED` needs `/var/log` mounted read-only into the container.
`CONN_WATCH_ENABLED` needs the host's `/proc` mounted read-only (as
`/hostproc`).

## API

All endpoints are `GET`, unauthenticated by default (unless `AUTH_ENABLED`
is set), and return `application/json`. The dashboard is just a client of
this same API.

| Endpoint | Returns |
|---|---|
| `GET /api/stats` | Latest snapshot: CPU, RAM, swap, load, disks, disk I/O, network, temperature, sockets, top processes |
| `GET /api/history?metric=<key>&points=60&of=<selector>` | Bucket-averaged sparkline data over the in-memory window |
| `GET /api/host` | Static host info (hostname, OS, kernel, CPU model, RAM, disks, USB devices) — read once at startup |
| `GET /api/containers` | Docker containers with live CPU/mem/net/block I/O and port mappings |
| `GET /api/alarms` | Active system-level alarms (CPU, RAM, disk, iowait/steal, temperature) |
| `GET /api/uptime/sessions` | Reboot history, most recent first |
| `GET /api/security/ssh` | SSH login attempts (needs `SSH_WATCH_ENABLED`) |
| `GET /api/security/connections` | New inbound connections per watched port (needs `CONN_WATCH_ENABLED`) |

<details>
<summary><b>Example: <code>GET /api/stats</code></b></summary>

```json
{
  "time": "2026-07-31T21:10:45Z",
  "cpu": { "usage_pct": 7.4, "user_pct": 4.8, "system_pct": 2.6, "iowait_pct": 0, "steal_pct": 0, "idle_pct": 92.6, "per_core": [7.2, 5.1] },
  "ram": { "total_kb": 32180436, "used_kb": 12707460, "available_kb": 20581740, "free_kb": 4077648, "cached_kb": 14396852, "buffers_kb": 998476, "used_pct": 39.5, "available_pct": 64.0, "free_pct": 12.7, "cache_pct": 44.7 },
  "swap": { "total_kb": 8388604, "used_kb": 0, "used_pct": 0, "present": true },
  "load": { "load1": 2.32, "load5": 2.25, "load15": 2.15, "normalized_pct": 19.3 },
  "disks": [{ "mount": "/", "device": "/dev/nvme0n1p2", "fs_type": "ext4", "total_bytes": 501809635328, "used_bytes": 234733932544, "avail_bytes": 241509900288, "used_pct": 46.8 }],
  "disk_io": [{ "device": "nvme0n1", "read_bps": 0, "write_bps": 12300, "read_iops": 0, "write_iops": 3.1, "latency_ms": 0.4 }],
  "net": [{ "name": "wlo1", "rx_bps": 104999.9, "tx_bps": 45984.7, "packets_ps": 183.5, "errors_ps": 0, "drops_ps": 0 }],
  "temp": [{ "zone": "thermal_zone0", "label": "x86_pkg_temp", "celsius": 54.0, "is_primary_cpu": true }],
  "sockets": { "tcp_in_use": 32, "tcp_orphan": 0, "tcp_time_wait": 60, "udp_in_use": 17, "total_used": 1811 },
  "processes": [{ "pid": 1234, "name": "chrome", "cpu_pct": 12.3 }]
}
```
</details>

<details>
<summary><b>Example: <code>GET /api/history</code></b></summary>

```json
{ "metric": "cpu.usage_pct", "points": [{ "time": 1753991400, "value": 7.4 }, { "time": 1753991405, "value": 8.1 }] }
```

Valid `metric` keys: `cpu.usage_pct`, `cpu.user_pct`, `cpu.system_pct`,
`cpu.iowait_pct`, `cpu.steal_pct`, `cpu.idle_pct`, `load.load1`,
`load.load5`, `load.load15`, `load.normalized_pct`, `ram.used_pct`,
`ram.available_pct`, `ram.free_pct`, `ram.cache_pct`, `swap.used_pct`,
`disk.used_pct`, `disk.avail_bytes`, `disk.total_bytes`,
`disk_io.read_bps`, `disk_io.write_bps`, `disk_io.read_iops`,
`disk_io.write_iops`, `disk_io.latency_ms`, `net.rx_bps`, `net.tx_bps`,
`net.packets_ps`, `net.errors_ps`, `net.drops_ps`, `temp.celsius`.

`of` is an optional selector for per-mount/per-device/per-interface metrics
(a mount path for `disk.*`, a device name for `disk_io.*`, an interface
name for `net.*`, a thermal zone id for `temp.celsius`). Omitted, `disk.*`
defaults to `/`, `disk_io.*`/`net.*` sum across all devices/interfaces
(max for `disk_io.latency_ms`), and `temp.celsius` picks the primary CPU
zone.
</details>

<details>
<summary><b>Example: <code>GET /api/host</code></b></summary>

```json
{
  "hostname": "myserver", "os_name": "Debian GNU/Linux", "os_version": "12", "kernel_version": "6.1.0-28-amd64",
  "architecture": "x86_64", "virtualization": "none", "cores_total": 12, "boot_time": "2026-07-31T13:22:21Z",
  "cpu_vendor": "AuthenticAMD", "cpu_model": "AMD Ryzen 5 7530U with Radeon Graphics", "cpu_mhz": 4547.9, "cpu_mhz_is_max": true,
  "ram_total_kb": 32180436, "ram_frequency_mhz": 0,
  "disks": [{ "device": "nvme0n1", "model": "MTFDKBA512QFM-1BD1AABHA", "size_bytes": 512110190592 }],
  "usb_devices": [{ "vendor_id": "0bda", "product_id": "b85c", "manufacturer": "Realtek", "product": "Bluetooth Radio" }]
}
```
</details>

<details>
<summary><b>Example: <code>GET /api/containers</code></b></summary>

```json
{ "containers": [{
  "id": "5b8a...", "name": "go-data-app-1", "image": "gonzxa/go-data:latest", "state": "running", "status": "Up 43 seconds",
  "ports": [{ "host_ip": "0.0.0.0", "host_port": 19245, "container_port": 9000, "proto": "tcp", "published": true }],
  "port_chips": [{ "label": "19245→9000/tcp", "published": true }],
  "size_rw_bytes": 0, "volume_usage_bytes": 284, "restart_count": 0, "health": "",
  "cpu_pct": 1.19, "mem_used_bytes": 11534336, "mem_limit_bytes": 16511410176, "mem_pct": 0.07,
  "block_read_bps": 0, "block_write_bps": 0, "net_rx_bps": 19548.7, "net_tx_bps": 33717.7
}] }
```

Empty array (not an error) if `DOCKER_ENABLED` is off or the socket isn't
reachable.
</details>

<details>
<summary><b>Example: <code>GET /api/alarms</code>, <code>/api/uptime/sessions</code>, <code>/api/security/*</code></b></summary>

```json
{ "alarms": [{ "id": "cpu.usage", "metric": "cpu.usage_pct", "severity": "warn", "message": "CPU alto", "value": 84.2, "since": "2026-07-31T21:00:00Z" }], "count": 1 }
```

```json
{ "sessions": [{ "start": "2026-07-31T13:22:21Z", "end": null, "duration_seconds": 27384.5, "ongoing": true }] }
```

```json
{ "events": [{ "time": "2026-07-31T21:05:00Z", "ip": "45.83.66.10", "user": "admin", "action": "failed", "success": false, "detail": "..." }], "enabled": true }
```

`action` is one of `accepted`, `failed`, `invalid_user`, `closed`.

```json
{ "events": [{ "time": "2026-07-31T21:06:00Z", "ip": "190.2.3.4", "local_port": 25565 }], "enabled": true }
```

`/api/security/*` endpoints return `enabled: false` with an empty `events`
array when their feature flag is off (or, for SSH, the auth log hasn't
been found yet). Connection events fire once per remote IP the first time
it's seen on a watched port, not repeated every poll while the connection
stays open.
</details>

## Project structure

```
cmd/server/       entrypoint
internal/
  ├── api/        HTTP handlers
  ├── app/        wiring / dependency injection
  ├── alarm/      threshold-based alarm engine
  ├── auth/       HTTP Basic Auth middleware
  ├── collector/  metric collection (CPU, RAM, disk, net, temp, Docker...)
  ├── config/     config.json + env var loading
  ├── domain/     shared types
  ├── security/   SSH & connection watchers
  ├── storage/    memory + InfluxDB backends
  └── uptime/     reboot session tracking
web/static/       dashboard (HTML + JS, no build step)
config.json       default configuration
docker-compose.yml
```

Each backend implements the same `storage.Storage` interface, so metrics can
fan out to memory and InfluxDB simultaneously without either one knowing
about the other.

## License

MIT
