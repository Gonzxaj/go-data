package domain

import "time"

// HostInfo is static-ish host information, read once at startup.
type HostInfo struct {
	Hostname       string    `json:"hostname"`
	OSName         string    `json:"os_name"`
	OSVersion      string    `json:"os_version"`
	KernelVersion  string    `json:"kernel_version"`
	Architecture   string    `json:"architecture"`
	Virtualization string    `json:"virtualization"`
	CoresTotal     int       `json:"cores_total"`
	BootTime       time.Time `json:"boot_time"`

	CPUVendor string  `json:"cpu_vendor"`
	CPUModel  string  `json:"cpu_model"`
	CPUMHz    float64 `json:"cpu_mhz"`
	CPUMHzMax bool    `json:"cpu_mhz_is_max"` // true if CPUMHz is the max rated clock, false if it's just the current reading

	RAMTotalKB      uint64  `json:"ram_total_kb"`
	RAMFrequencyMHz float64 `json:"ram_frequency_mhz"` // best-effort (dmidecode); 0 if unknown

	Disks      []DiskInfo  `json:"disks"`
	USBDevices []USBDevice `json:"usb_devices"`
}

type DiskInfo struct {
	Device     string          `json:"device"`
	Model      string          `json:"model"`
	SizeBytes  uint64          `json:"size_bytes"`
	Kind       string          `json:"kind,omitempty"` // "HDD" or "SSD" when the driver exposes it, else ""
	Partitions []PartitionInfo `json:"partitions,omitempty"`
}

// PartitionInfo is a mounted, data-bearing partition on a physical disk —
// technical partitions (EFI/ESP, a standalone /boot, swap, MSR, ...) are
// filtered out before this ever gets built. See readDiskInfo in
// internal/collector/host.go.
type PartitionInfo struct {
	Device     string  `json:"device"`
	FSType     string  `json:"fs_type"`
	Mount      string  `json:"mount"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	AvailBytes uint64  `json:"avail_bytes"`
	UsedPct    float64 `json:"used_pct"`
}

type USBDevice struct {
	VendorID     string `json:"vendor_id"`
	ProductID    string `json:"product_id"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
}
