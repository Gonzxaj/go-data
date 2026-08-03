package domain

import "time"

// Metrics is the fast, per-tick (1s) system snapshot.
type Metrics struct {
	Time      time.Time      `json:"time"`
	CPU       CPU            `json:"cpu"`
	RAM       RAM            `json:"ram"`
	Swap      Swap           `json:"swap"`
	Load      Load           `json:"load"`
	Disks     []Disk         `json:"disks"`
	DiskIO    []DiskIO       `json:"disk_io"`
	Net       []NetIface     `json:"net"`
	Temp      []Temp         `json:"temp"`
	Sockets   Sockets        `json:"sockets"`
	Processes []ProcessInfo  `json:"processes"`
}

type CPU struct {
	UsagePct  float64   `json:"usage_pct"`
	UserPct   float64   `json:"user_pct"`
	SystemPct float64   `json:"system_pct"`
	IOWaitPct float64   `json:"iowait_pct"`
	StealPct  float64   `json:"steal_pct"`
	IdlePct   float64   `json:"idle_pct"`
	PerCore   []float64 `json:"per_core"`
}

type RAM struct {
	TotalKB      uint64  `json:"total_kb"`
	UsedKB       uint64  `json:"used_kb"`
	AvailableKB  uint64  `json:"available_kb"`
	FreeKB       uint64  `json:"free_kb"`
	CachedKB     uint64  `json:"cached_kb"`
	BuffersKB    uint64  `json:"buffers_kb"`
	UsedPct      float64 `json:"used_pct"`
	AvailablePct float64 `json:"available_pct"`
	FreePct      float64 `json:"free_pct"`
	CachePct     float64 `json:"cache_pct"`
}

type Swap struct {
	TotalKB uint64  `json:"total_kb"`
	UsedKB  uint64  `json:"used_kb"`
	UsedPct float64 `json:"used_pct"`
	Present bool    `json:"present"`
}

type Load struct {
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	NormalizedPct float64 `json:"normalized_pct"`
}

type Disk struct {
	Mount      string  `json:"mount"`
	Device     string  `json:"device"`
	FSType     string  `json:"fs_type"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	AvailBytes uint64  `json:"avail_bytes"`
	UsedPct    float64 `json:"used_pct"`
}

type DiskIO struct {
	Device    string  `json:"device"`
	ReadBps   float64 `json:"read_bps"`
	WriteBps  float64 `json:"write_bps"`
	ReadIOPS  float64 `json:"read_iops"`
	WriteIOPS float64 `json:"write_iops"`
	LatencyMs float64 `json:"latency_ms"`
}

type NetIface struct {
	Name      string  `json:"name"`
	RxBps     float64 `json:"rx_bps"`
	TxBps     float64 `json:"tx_bps"`
	PacketsPS float64 `json:"packets_ps"`
	ErrorsPS  float64 `json:"errors_ps"`
	DropsPS   float64 `json:"drops_ps"`
}

type Temp struct {
	Zone         string  `json:"zone"`
	Label        string  `json:"label"`
	Celsius      float64 `json:"celsius"`
	IsPrimaryCPU bool    `json:"is_primary_cpu"`
}

type Sockets struct {
	TCPInUse    int `json:"tcp_in_use"`
	TCPOrphan   int `json:"tcp_orphan"`
	TCPTimeWait int `json:"tcp_time_wait"`
	UDPInUse    int `json:"udp_in_use"`
	TotalUsed   int `json:"total_used"`
}

type ProcessInfo struct {
	PID    int     `json:"pid"`
	Name   string  `json:"name"`
	CPUPct float64 `json:"cpu_pct"`
}
