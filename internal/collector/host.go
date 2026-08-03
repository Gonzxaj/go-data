package collector

import (
	"go-data/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// CollectHostInfo gathers static-ish host info once, at process start.
func CollectHostInfo() domain.HostInfo {
	hostname, _ := os.Hostname()
	osName, osVersion := readOSRelease()
	cpuVendor, cpuModel := readCPUIdentity()
	cpuMHz, cpuMHzIsMax := readCPUMHz()
	ram, _ := readMemInfo()

	return domain.HostInfo{
		Hostname:       hostname,
		OSName:         osName,
		OSVersion:      osVersion,
		KernelVersion:  readKernelVersion(),
		Architecture:   readArchitecture(),
		Virtualization: detectVirtualization(),
		CoresTotal:     runtime.NumCPU(),
		BootTime:       time.Now().Add(-readUptime()),

		CPUVendor: cpuVendor,
		CPUModel:  cpuModel,
		CPUMHz:    cpuMHz,
		CPUMHzMax: cpuMHzIsMax,

		RAMTotalKB:      ram.TotalKB,
		RAMFrequencyMHz: readRAMFrequencyMHz(),

		Disks:      readDiskInfo(),
		USBDevices: readUSBDevices(),
	}
}

func readOSRelease() (name, version string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", ""
	}
	vals := map[string]string{}
	for _, l := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) != 2 {
			continue
		}
		vals[parts[0]] = strings.Trim(parts[1], `"`)
	}
	if pretty := vals["PRETTY_NAME"]; pretty != "" {
		return pretty, vals["VERSION_ID"]
	}
	return vals["NAME"], vals["VERSION_ID"]
}

func readKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		return fields[2]
	}
	return ""
}

// readArchitecture reports the conventional uname -m style name. Relies on
// GOARCH matching the host since this project is never cross-compiled.
func readArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armv7l"
	default:
		return runtime.GOARCH
	}
}

func detectVirtualization() string {
	if data, err := os.ReadFile("/sys/hypervisor/type"); err == nil {
		if t := strings.TrimSpace(string(data)); t != "" {
			return t
		}
	}
	cpuinfo, err := os.ReadFile("/proc/cpuinfo")
	if err != nil || !strings.Contains(string(cpuinfo), "hypervisor") {
		return "none"
	}
	if data, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
		if p := strings.TrimSpace(string(data)); p != "" {
			return p
		}
	}
	return "virtualized"
}

func readUptime() time.Duration {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return time.Duration(secs * float64(time.Second))
}

// readCPUIdentity reads the vendor and model name of the first logical CPU
// from /proc/cpuinfo — identical across cores on any real machine.
func readCPUIdentity() (vendor, model string) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		if vendor == "" && strings.HasPrefix(l, "vendor_id") {
			vendor = cpuInfoValue(l)
		}
		if model == "" && strings.HasPrefix(l, "model name") {
			model = cpuInfoValue(l)
		}
		if vendor != "" && model != "" {
			break
		}
	}
	return vendor, model
}

func cpuInfoValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// readCPUMHz prefers the rated max clock from cpufreq (stable) and falls
// back to the current "cpu MHz" reading from /proc/cpuinfo (fluctuates with
// frequency scaling, but better than nothing when cpufreq isn't exposed).
func readCPUMHz() (mhz float64, isMax bool) {
	if data, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq"); err == nil {
		if khz, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil && khz > 0 {
			return khz / 1000, true
		}
	}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return 0, false
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "cpu MHz") {
			if v, err := strconv.ParseFloat(cpuInfoValue(l), 64); err == nil {
				return v, false
			}
		}
	}
	return 0, false
}

// readRAMFrequencyMHz is best-effort: DIMM speed isn't exposed via /proc or
// /sys, only via SMBIOS (dmidecode), which needs the binary present and
// /dev/mem access — typically unavailable in an unprivileged container, in
// which case this returns 0 and the frontend shows "N/D".
func readRAMFrequencyMHz() float64 {
	path, err := exec.LookPath("dmidecode")
	if err != nil {
		return 0
	}
	out, err := exec.Command(path, "-t", "17").Output()
	if err != nil {
		return 0
	}
	re := regexp.MustCompile(`(?:Configured Memory Speed|Speed):\s*(\d+)\s*(?:MHz|MT/s)`)
	for _, m := range re.FindAllStringSubmatch(string(out), -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 {
			return v
		}
	}
	return 0
}

// readDiskInfo reads model + capacity for every real block device (same
// device set diskio.go tracks), from sysfs — no privileges required.
func readDiskInfo() []domain.DiskInfo {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	var out []domain.DiskInfo
	for _, e := range entries {
		name := e.Name()
		if !realDiskRe.MatchString(name) {
			continue
		}
		model := readSysfsTrimmed(filepath.Join("/sys/block", name, "device", "model"))
		vendor := readSysfsTrimmed(filepath.Join("/sys/block", name, "device", "vendor"))
		label := strings.TrimSpace(vendor + " " + model)

		var sizeBytes uint64
		if data, err := os.ReadFile(filepath.Join("/sys/block", name, "size")); err == nil {
			if sectors, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
				sizeBytes = sectors * 512
			}
		}
		out = append(out, domain.DiskInfo{Device: name, Model: label, SizeBytes: sizeBytes})
	}
	return out
}

func readSysfsTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// usbHubClass is the USB device class code for hubs — filtered out so the
// list only shows actual peripherals, not internal root/external hubs.
const usbHubClass = "09"

// readUSBDevices enumerates /sys/bus/usb/devices — informational sysfs
// metadata that stays visible inside unprivileged containers even though the
// actual /dev/bus/usb device nodes are not.
func readUSBDevices() []domain.USBDevice {
	entries, err := os.ReadDir("/sys/bus/usb/devices")
	if err != nil {
		return nil
	}
	var out []domain.USBDevice
	for _, e := range entries {
		dir := filepath.Join("/sys/bus/usb/devices", e.Name())
		vendorID := readSysfsTrimmed(filepath.Join(dir, "idVendor"))
		productID := readSysfsTrimmed(filepath.Join(dir, "idProduct"))
		if vendorID == "" || productID == "" {
			continue // not a device node (e.g. an interface like "1-1:1.0")
		}
		if readSysfsTrimmed(filepath.Join(dir, "bDeviceClass")) == usbHubClass {
			continue
		}
		out = append(out, domain.USBDevice{
			VendorID:     vendorID,
			ProductID:    productID,
			Manufacturer: readSysfsTrimmed(filepath.Join(dir, "manufacturer")),
			Product:      readSysfsTrimmed(filepath.Join(dir, "product")),
		})
	}
	return out
}
