package domain

// Container is a Docker container snapshot, refreshed on its own (slower)
// cadence than the system Metrics tick.
type Container struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`

	Ports     []Port     `json:"ports"`
	PortChips []PortChip `json:"port_chips"`

	SizeRwBytes      int64 `json:"size_rw_bytes"`
	VolumeUsageBytes int64 `json:"volume_usage_bytes"`

	RestartCount int    `json:"restart_count"`
	Health       string `json:"health"` // healthy/unhealthy/starting/none

	CPUPct       float64 `json:"cpu_pct"`
	MemUsedBytes uint64  `json:"mem_used_bytes"`
	MemLimitBytes uint64 `json:"mem_limit_bytes"`
	MemPct       float64 `json:"mem_pct"`

	BlockReadBps  float64 `json:"block_read_bps"`
	BlockWriteBps float64 `json:"block_write_bps"`
	NetRxBps      float64 `json:"net_rx_bps"`
	NetTxBps      float64 `json:"net_tx_bps"`
}

type Port struct {
	HostIP        string `json:"host_ip,omitempty"`
	HostPort      int    `json:"host_port,omitempty"`
	ContainerPort int    `json:"container_port"`
	Proto         string `json:"proto"`
	Published     bool   `json:"published"`
}

type PortChip struct {
	Label     string `json:"label"`
	Published bool   `json:"published"`
}
