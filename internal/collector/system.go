package collector

import (
	"go-data/internal/config"
	"go-data/internal/domain"
	"os"
	"strconv"
	"strings"
	"time"
)

// SystemCollector reads /proc and /sys each tick to build a full domain.Metrics
// snapshot. It holds previous samples for every rate-based metric (CPU,
// disk IO, network) so it can compute deltas. It is only ever called from a
// single background goroutine, so no locking is needed.
type SystemCollector struct {
	prevCPU     cpuSample
	havePrevCPU bool
	prevCores   []cpuSample

	prevDiskIO map[string]diskIOSample
	prevNet    map[string]netSample

	cores        int
	diskMounts   []mountEntry
	thermal      []thermalZone
	hostProcPath string // "/proc", or a mounted host /proc (e.g. "/hostproc") — see readSockets/readNetDev
}

func NewSystemCollector(cfg config.Config) *SystemCollector {
	return &SystemCollector{
		cores:        readCoreCount(),
		diskMounts:   resolveDiskMounts(cfg.DiskMounts),
		thermal:      resolveThermalZones(),
		hostProcPath: cfg.HostProcPath,
	}
}

func (c *SystemCollector) Collect() domain.Metrics {
	m := domain.Metrics{Time: time.Now()}

	agg, perCore, ok := readCPUSamples()
	if ok {
		if c.havePrevCPU {
			m.CPU = cpuDeltaPct(c.prevCPU, agg)
			m.CPU.PerCore = perCoreDeltaPct(c.prevCores, perCore)
		}
		c.prevCPU = agg
		c.prevCores = perCore
		c.havePrevCPU = true
	}

	m.RAM, m.Swap = readMemInfo()
	m.Load = readLoad(c.cores)
	m.Disks = readDisks(c.diskMounts)
	m.Temp = readTemps(c.thermal)
	m.Sockets = readSockets(c.hostProcPath)

	curDiskIO := readDiskStats()
	if c.prevDiskIO != nil {
		m.DiskIO = diskIODeltas(c.prevDiskIO, curDiskIO)
	}
	c.prevDiskIO = curDiskIO

	curNet := readNetDev(c.hostProcPath)
	if c.prevNet != nil {
		m.Net = netDeltas(c.prevNet, curNet)
	}
	c.prevNet = curNet

	return m
}

func perCoreDeltaPct(prev, cur []cpuSample) []float64 {
	n := len(cur)
	if len(prev) < n {
		n = len(prev)
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = cpuDeltaPct(prev[i], cur[i]).UsagePct
	}
	return out
}

func readCoreCount() int {
	_, perCore, ok := readCPUSamples()
	if !ok || len(perCore) == 0 {
		return 1
	}
	return len(perCore)
}

func readLoad(cores int) domain.Load {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return domain.Load{}
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return domain.Load{}
	}
	l1, _ := strconv.ParseFloat(fields[0], 64)
	l5, _ := strconv.ParseFloat(fields[1], 64)
	l15, _ := strconv.ParseFloat(fields[2], 64)
	load := domain.Load{Load1: l1, Load5: l5, Load15: l15}
	if cores > 0 {
		load.NormalizedPct = 100 * l1 / float64(cores)
	}
	return load
}
