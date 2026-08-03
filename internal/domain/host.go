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
	Device    string `json:"device"`
	Model     string `json:"model"`
	SizeBytes uint64 `json:"size_bytes"`
}

type USBDevice struct {
	VendorID     string `json:"vendor_id"`
	ProductID    string `json:"product_id"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
}
