# go-data

A lightweight self-hosted monitoring system in Go (CPU, RAM, disk, network,
temperature, Docker, SSH attempts, incoming connections). One binary, one
web dashboard.

Docker image: `gonzxa/go-data`.

## Run it

```bash
docker compose up -d
```

Dashboard: `http://localhost:19245`

## Configuration (environment variables)

All settings can be set as environment variables in `docker-compose.yml`.
They override `config.json`, so you don't need to rebuild anything to
change behavior — just edit `environment:` and run `docker compose up -d`
again.

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
| `CONN_WATCH_AUTO` | Auto-detect which ports to watch (anything listening on a public address, e.g. a game server) instead of listing them by hand. | `true` |
| `CONN_WATCH_PORTS` | Extra ports to always watch, comma-separated (e.g. `25565,7777`). | none |
| `DISK_MOUNTS` | Which mount points to show in the Disk tab, comma-separated. Empty = auto-detect. | auto |
| `DATA_DIR` | Where local logs (reboot history, SSH/connection events) are stored. | `./data` |
| `HOST_PROC_PATH` | `/proc` path used for connection watching and the CPU top-processes list. Auto-detects `/hostproc` if mounted. | auto |
| `AUTH_ENABLED` | Require HTTP Basic Auth (like nginx's `auth_basic`) for the dashboard and API. Needs `AUTH_USER`/`AUTH_PASSWORD` set, or the app refuses to start. | `false` |
| `AUTH_USER` | Basic Auth username. | — |
| `AUTH_PASSWORD` | Basic Auth password. | — |

`SSH_WATCH_ENABLED` needs `/var/log` mounted read-only into the container.
`CONN_WATCH_ENABLED` needs the host's `/proc` mounted read-only (as
`/hostproc`) — both are already set up in `docker-compose.yml`.

## docker-compose.yml example

Using the published image (no need to clone the source or build anything):

```yaml
services:
  app:
    image: gonzxa/go-data:latest
    ports:
      - "19245:9000"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro   # Docker containers tab
      - /var/log:/var/log:ro # SSH_WATCH_ENABLED
      - /proc:/hostproc:ro # CONN_WATCH_ENABLED
    environment:
      - USE_INFLUX=true
      - INFLUX_URL=http://influx:8086
      - INFLUX_TOKEN=change-me
      - INFLUX_ORG=my-org
      - INFLUX_BUCKET=metrics
      - DOCKER_ENABLED=true
      - SSH_WATCH_ENABLED=true
      - CONN_WATCH_ENABLED=true
      - CONN_WATCH_AUTO=true
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

Any of the three `/var/run/docker.sock`, `/var/log`, `/proc` mounts can be
dropped if you don't need that tab — the app degrades gracefully (that
feature just won't show data) instead of crashing.

Don't use `admin123`-style values for real — replace `change-me` with your
own token/password before running this anywhere reachable from the internet.

## API

All endpoints are `GET`, unauthenticated, and return `application/json`. The
web dashboard is just a client of this same API — anything it shows, you can
query directly.

### `GET /api/stats`

The latest snapshot (same shape saved every second into the in-memory ring
buffer). `disks`/`disk_io`/`net`/`temp`/`processes` are arrays because a host
can have more than one of each.

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

### `GET /api/history?metric=<key>&points=60&of=<selector>`

Sparkline data, bucket-averaged over the ~5-minute in-memory window.

- `metric` — one of a fixed set of keys (see table below).
- `points` — how many buckets to return (default `60`).
- `of` — optional selector for per-mount/per-device/per-interface metrics
  (a mount path for `disk.*`, a device name for `disk_io.*`, an interface
  name for `net.*`, a thermal zone id for `temp.celsius`). Omit it and
  `disk.*` defaults to `/`, `disk_io.*`/`net.*` sum across all
  devices/interfaces (max for `disk_io.latency_ms`), and `temp.celsius`
  picks the primary CPU zone.

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

### `GET /api/host`

Static host info, read once at startup (hardware never changes at runtime).

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

### `GET /api/containers`

Docker containers with live resource usage. Empty array (not an error) if
`DOCKER_ENABLED` is off or the socket isn't reachable.

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

### `GET /api/alarms`

System-level alarms only (CPU, RAM, disk, iowait/steal, temperature) —
container state isn't included here, it's shown directly in the Docker tab.

```json
{ "alarms": [{ "id": "cpu.usage", "metric": "cpu.usage_pct", "severity": "warn", "message": "CPU alto", "value": 84.2, "since": "2026-07-31T21:00:00Z" }], "count": 1 }
```

### `GET /api/uptime/sessions`

Reboot history, most recent first, reconstructed from `/proc/uptime` boot
time changes (not from Netdata-style long-window chart reconstruction).

```json
{ "sessions": [{ "start": "2026-07-31T13:22:21Z", "end": null, "duration_seconds": 27384.5, "ongoing": true }] }
```

### `GET /api/security/ssh`

`enabled: false` (with an empty `events` array) if `SSH_WATCH_ENABLED` is off
or the auth log hasn't been found yet.

```json
{ "events": [{ "time": "2026-07-31T21:05:00Z", "ip": "45.83.66.10", "user": "admin", "action": "failed", "success": false, "detail": "..." }], "enabled": true }
```

`action` is one of `accepted`, `failed`, `invalid_user`, `closed`.

### `GET /api/security/connections`

`enabled: false` (with an empty `events` array) if `CONN_WATCH_ENABLED` is
off. One event per remote IP the first time it's seen connected to a
watched port (not repeated every poll while the connection stays open).

```json
{ "events": [{ "time": "2026-07-31T21:06:00Z", "ip": "190.2.3.4", "local_port": 25565 }], "enabled": true }
```
