package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"go-data/internal/domain"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const dockerSocket = "/var/run/docker.sock"

// DockerCollector talks to the Docker Engine API over its unix socket,
// polling on its own cadence (independent from the 1s system-metrics tick).
type DockerCollector struct {
	httpc *http.Client

	mu         sync.RWMutex
	containers []domain.Container

	prevStats    map[string]dockerPrevSample
	lastInspect  map[string]time.Time
	inspectCache map[string]inspectMeta
	volSizes     map[string]int64
	lastVolFetch time.Time
}

type dockerPrevSample struct {
	cpuTotal, systemTotal uint64
	blkRead, blkWrite     uint64
	netRx, netTx          uint64
	at                    time.Time
}

type inspectMeta struct {
	restartCount int
	health       string
}

func newDockerHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", dockerSocket)
			},
		},
		Timeout: 5 * time.Second,
	}
}

// NewDockerCollector fails fast if the Docker socket isn't reachable, so
// app.Build can degrade to "no containers" instead of crashing.
func NewDockerCollector() (*DockerCollector, error) {
	c := &DockerCollector{
		httpc:        newDockerHTTPClient(),
		prevStats:    map[string]dockerPrevSample{},
		lastInspect:  map[string]time.Time{},
		inspectCache: map[string]inspectMeta{},
		volSizes:     map[string]int64{},
	}
	if _, err := c.get("/_ping"); err != nil {
		return nil, fmt.Errorf("docker socket unreachable: %w", err)
	}
	return c, nil
}

func (d *DockerCollector) get(path string) ([]byte, error) {
	resp, err := d.httpc.Get("http://unix" + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("docker api %s: status %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Run polls the Docker API on the given cadence until ctx is cancelled.
func (d *DockerCollector) Run(ctx context.Context, tick time.Duration) {
	d.refresh()
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.refresh()
		}
	}
}

// Snapshot returns the latest containers list, safe for concurrent reads.
func (d *DockerCollector) Snapshot() []domain.Container {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]domain.Container, len(d.containers))
	copy(out, d.containers)
	return out
}

func (d *DockerCollector) refresh() {
	raw, err := d.get("/containers/json?all=true&size=true")
	if err != nil {
		log.Printf("docker: list containers: %v", err)
		return
	}
	var items []containerListItem
	if err := json.Unmarshal(raw, &items); err != nil {
		log.Printf("docker: decode container list: %v", err)
		return
	}

	if time.Since(d.lastVolFetch) > 15*time.Second {
		d.refreshVolumeSizes()
	}

	now := time.Now()
	out := make([]domain.Container, 0, len(items))
	for _, item := range items {
		out = append(out, d.buildContainer(item, now))
	}

	d.mu.Lock()
	d.containers = out
	d.mu.Unlock()
}

func (d *DockerCollector) buildContainer(item containerListItem, now time.Time) domain.Container {
	name := item.name()
	ports := make([]domain.Port, 0, len(item.Ports))
	for _, p := range item.Ports {
		ports = append(ports, domain.Port{
			HostIP: p.IP, HostPort: p.PublicPort, ContainerPort: p.PrivatePort,
			Proto: p.Type, Published: p.PublicPort != 0,
		})
	}
	deduped := dedupePorts(ports)

	c := domain.Container{
		ID: item.ID, Name: name, Image: item.Image,
		State: item.State, Status: item.Status,
		Ports: deduped, PortChips: buildPortChips(ports),
		SizeRwBytes: item.SizeRw,
	}

	if meta, ok := d.inspectCache[item.ID]; ok {
		c.RestartCount = meta.restartCount
		c.Health = meta.health
	}
	if item.State == "running" {
		d.attachLiveStats(&c, item.ID, now)
	}

	var volBytes int64
	var hasVol bool
	for _, m := range item.Mounts {
		if m.Type == "volume" {
			if size, ok := d.volSizes[m.Name]; ok {
				volBytes += size
				hasVol = true
			}
		}
	}
	if hasVol {
		c.VolumeUsageBytes = volBytes
	}

	if time.Since(d.lastInspect[item.ID]) > 15*time.Second {
		d.inspectOne(item.ID)
	}

	return c
}

func (d *DockerCollector) attachLiveStats(c *domain.Container, id string, now time.Time) {
	raw, err := d.get("/containers/" + id + "/stats?stream=false")
	if err != nil {
		return
	}
	var s containerStatsResp
	if err := json.Unmarshal(raw, &s); err != nil {
		return
	}

	blkRead, blkWrite := s.blkioTotals()
	var netRx, netTx uint64
	for _, n := range s.Networks {
		netRx += n.RxBytes
		netTx += n.TxBytes
	}

	prev, ok := d.prevStats[id]
	d.prevStats[id] = dockerPrevSample{
		cpuTotal: s.CPUStats.CPUUsage.TotalUsage, systemTotal: s.CPUStats.SystemCPUUsage,
		blkRead: blkRead, blkWrite: blkWrite, netRx: netRx, netTx: netTx, at: now,
	}

	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - prevOr(ok, prev.cpuTotal))
	sysDelta := float64(s.CPUStats.SystemCPUUsage - prevOr(ok, prev.systemTotal))
	onlineCPUs := s.CPUStats.OnlineCPUs
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}
	if ok && sysDelta > 0 && cpuDelta >= 0 {
		c.CPUPct = (cpuDelta / sysDelta) * float64(onlineCPUs) * 100
	}

	c.MemUsedBytes = s.MemoryStats.Usage - s.MemoryStats.cacheBytes()
	c.MemLimitBytes = s.MemoryStats.Limit
	if c.MemLimitBytes > 0 {
		c.MemPct = 100 * float64(c.MemUsedBytes) / float64(c.MemLimitBytes)
	}

	if ok {
		dSeconds := now.Sub(prev.at).Seconds()
		if dSeconds > 0 {
			if blkRead >= prev.blkRead {
				c.BlockReadBps = float64(blkRead-prev.blkRead) / dSeconds
			}
			if blkWrite >= prev.blkWrite {
				c.BlockWriteBps = float64(blkWrite-prev.blkWrite) / dSeconds
			}
			if netRx >= prev.netRx {
				c.NetRxBps = float64(netRx-prev.netRx) / dSeconds
			}
			if netTx >= prev.netTx {
				c.NetTxBps = float64(netTx-prev.netTx) / dSeconds
			}
		}
	}
}

func prevOr(ok bool, v uint64) uint64 {
	if !ok {
		return 0
	}
	return v
}

func (d *DockerCollector) inspectOne(id string) {
	d.lastInspect[id] = time.Now()
	raw, err := d.get("/containers/" + id + "/json")
	if err != nil {
		return
	}
	var resp inspectResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return
	}
	meta := inspectMeta{restartCount: resp.RestartCount}
	if resp.State.Health != nil {
		meta.health = resp.State.Health.Status
	}
	d.inspectCache[id] = meta
}

func (d *DockerCollector) refreshVolumeSizes() {
	d.lastVolFetch = time.Now()
	raw, err := d.get("/system/df")
	if err != nil {
		return
	}
	var resp systemDFResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return
	}
	sizes := map[string]int64{}
	for _, v := range resp.Volumes {
		if v.UsageData != nil {
			sizes[v.Name] = v.UsageData.Size
		}
	}
	d.volSizes = sizes
}

// ---- Docker Engine API response shapes (only the fields we use) ----

type containerListItem struct {
	ID      string        `json:"Id"`
	Names   []string      `json:"Names"`
	Image   string        `json:"Image"`
	State   string        `json:"State"`
	Status  string        `json:"Status"`
	Ports   []dockerPort  `json:"Ports"`
	SizeRw  int64         `json:"SizeRw"`
	Mounts  []dockerMount `json:"Mounts"`
}

func (c containerListItem) name() string {
	if len(c.Names) == 0 {
		if len(c.ID) > 12 {
			return c.ID[:12]
		}
		return c.ID
	}
	n := c.Names[0]
	if len(n) > 0 && n[0] == '/' {
		return n[1:]
	}
	return n
}

type dockerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type dockerMount struct {
	Type string `json:"Type"`
	Name string `json:"Name"`
}

type containerStatsResp struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	MemoryStats containerMemoryStats `json:"memory_stats"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

type containerMemoryStats struct {
	Usage uint64 `json:"usage"`
	Limit uint64 `json:"limit"`
	Stats struct {
		Cache             uint64 `json:"cache"`               // cgroup v1 fallback
		TotalInactiveFile uint64 `json:"total_inactive_file"` // cgroup v1
		InactiveFile      uint64 `json:"inactive_file"`       // cgroup v2
	} `json:"stats"`
}

// cacheBytes returns the reclaimable page-cache portion of Usage, mirroring
// the Docker CLI's own `docker stats` calculation: usage - inactive file
// cache, since raw cgroup memory.usage includes page cache from disk I/O
// (huge for something like a torrent client) that isn't "real" memory
// pressure. Checks cgroup v1's "total_inactive_file" first, then cgroup v2's
// "inactive_file", falling back to the older "cache" field, in that order.
func (m containerMemoryStats) cacheBytes() uint64 {
	switch {
	case m.Stats.TotalInactiveFile > 0 && m.Stats.TotalInactiveFile < m.Usage:
		return m.Stats.TotalInactiveFile
	case m.Stats.InactiveFile > 0 && m.Stats.InactiveFile < m.Usage:
		return m.Stats.InactiveFile
	case m.Stats.Cache > 0 && m.Stats.Cache < m.Usage:
		return m.Stats.Cache
	default:
		return 0
	}
}

func (s containerStatsResp) blkioTotals() (read, write uint64) {
	for _, e := range s.BlkioStats.IOServiceBytesRecursive {
		switch e.Op {
		case "Read", "read":
			read += e.Value
		case "Write", "write":
			write += e.Value
		}
	}
	return read, write
}

type inspectResp struct {
	RestartCount int `json:"RestartCount"`
	State        struct {
		Health *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
}

type systemDFResp struct {
	Volumes []struct {
		Name      string `json:"Name"`
		UsageData *struct {
			Size int64 `json:"Size"`
		} `json:"UsageData"`
	} `json:"Volumes"`
}
