package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// UseInflux persists metrics to InfluxDB in addition to the in-memory
	// ring buffer. The memory buffer itself is not optional: /api/stats and
	// /api/history always read from it, since it's what makes the live
	// dashboard fast — there's no "memory off" mode.
	UseInflux bool   `json:"use_influx"`
	InfluxURL string `json:"influx_url"`
	Token     string `json:"token"`
	Bucket    string `json:"bucket"`
	Org       string `json:"org"`

	DiskMounts    []string `json:"disk_mounts,omitempty"` // empty = auto-discover
	DataDir       string   `json:"data_dir,omitempty"`
	DockerEnabled bool     `json:"docker_enabled"`

	SSHWatchEnabled bool   `json:"ssh_watch_enabled"`
	SSHLogPath      string `json:"ssh_log_path,omitempty"` // empty = auto-detect (/var/log/auth.log, /var/log/secure)

	ConnWatchEnabled bool   `json:"conn_watch_enabled"`
	ConnWatchAuto    bool   `json:"conn_watch_auto"`            // auto-discover watched ports from whatever is LISTENing on a non-loopback address; default true
	ConnWatchPorts   []int  `json:"conn_watch_ports,omitempty"` // extra ports to always watch, on top of auto-discovery (or instead of it, if conn_watch_auto is false)
	HostProcPath     string `json:"host_proc_path,omitempty"`   // empty = auto-detect ("/hostproc" if mounted, else "/proc")

	AuthEnabled  bool   `json:"auth_enabled"`
	AuthUser     string `json:"auth_user,omitempty"`
	AuthPassword string `json:"auth_password,omitempty"`
}

// Load reads config.json for defaults, then lets environment variables
// (e.g. set in docker-compose.yml) override any of them — env vars win,
// since they're the per-deployment knob, while config.json stays as the
// baseline shipped with the image.
func Load() Config {
	file, _ := os.ReadFile("config.json")
	cfg := Config{DockerEnabled: true, DataDir: "./data", ConnWatchAuto: true}
	json.Unmarshal(file, &cfg)

	cfg.UseInflux = envBool("USE_INFLUX", cfg.UseInflux)
	cfg.InfluxURL = envString("INFLUX_URL", cfg.InfluxURL)
	cfg.Token = envString("INFLUX_TOKEN", cfg.Token)
	cfg.Bucket = envString("INFLUX_BUCKET", cfg.Bucket)
	cfg.Org = envString("INFLUX_ORG", cfg.Org)

	cfg.DiskMounts = envStringList("DISK_MOUNTS", cfg.DiskMounts)
	cfg.DataDir = envString("DATA_DIR", cfg.DataDir)
	cfg.DockerEnabled = envBool("DOCKER_ENABLED", cfg.DockerEnabled)

	cfg.SSHWatchEnabled = envBool("SSH_WATCH_ENABLED", cfg.SSHWatchEnabled)
	cfg.SSHLogPath = envString("SSH_LOG_PATH", cfg.SSHLogPath)

	cfg.ConnWatchEnabled = envBool("CONN_WATCH_ENABLED", cfg.ConnWatchEnabled)
	cfg.ConnWatchAuto = envBool("CONN_WATCH_AUTO", cfg.ConnWatchAuto)
	cfg.ConnWatchPorts = envIntList("CONN_WATCH_PORTS", cfg.ConnWatchPorts)
	cfg.HostProcPath = envString("HOST_PROC_PATH", cfg.HostProcPath)

	cfg.AuthEnabled = envBool("AUTH_ENABLED", cfg.AuthEnabled)
	cfg.AuthUser = envString("AUTH_USER", cfg.AuthUser)
	cfg.AuthPassword = envString("AUTH_PASSWORD", cfg.AuthPassword)

	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.HostProcPath == "" {
		cfg.HostProcPath = "/proc"
		if _, err := os.Stat("/hostproc"); err == nil {
			cfg.HostProcPath = "/hostproc"
		}
	}
	return cfg
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// envStringList parses a comma-separated env var, e.g. "/,/data".
func envStringList(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// envIntList parses a comma-separated env var of ports, e.g. "25565,7777".
func envIntList(key string, def []int) []int {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	var out []int
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}
