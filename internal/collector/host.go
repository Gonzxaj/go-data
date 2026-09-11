package collector

import (
	"go-data/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CollectHostInfo gathers static-ish host info once, at process start.
// hostProcPath and hostRootPath are only needed for partition detection
// (see readDiskInfo) — every other field is read straight from this
// process's own /proc and /sys, which already reflect the host's real
// values regardless of containerization.
func CollectHostInfo(hostProcPath, hostRootPath string) domain.HostInfo {
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

		Disks:      readDiskInfo(hostProcPath, hostRootPath),
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
// device set diskio.go tracks), from sysfs — no privileges required — and
// attaches the relevant mounted partitions found on each one. hostProcPath
// and hostRootPath are only used to see past container isolation — see
// readDiskPartitions.
func readDiskInfo(hostProcPath, hostRootPath string) []domain.DiskInfo {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	var diskNames []string
	for _, e := range entries {
		if realDiskRe.MatchString(e.Name()) {
			diskNames = append(diskNames, e.Name())
		}
	}
	parents := buildPartitionParents(diskNames)
	byDisk := readDiskPartitions(parents, hostProcPath, hostRootPath)

	var out []domain.DiskInfo
	for _, name := range diskNames {
		model := readSysfsTrimmed(filepath.Join("/sys/block", name, "device", "model"))
		vendor := readSysfsTrimmed(filepath.Join("/sys/block", name, "device", "vendor"))
		label := strings.TrimSpace(vendor + " " + model)

		var sizeBytes uint64
		if data, err := os.ReadFile(filepath.Join("/sys/block", name, "size")); err == nil {
			if sectors, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
				sizeBytes = sectors * 512
			}
		}
		out = append(out, domain.DiskInfo{
			Device:     name,
			Model:      label,
			SizeBytes:  sizeBytes,
			Kind:       readDiskKind(name),
			Partitions: byDisk[name],
		})
	}
	return out
}

// readDiskKind reports "HDD" or "SSD" from sysfs's rotational flag, when the
// driver exposes it — "" if unknown (e.g. some virtualized/passthrough disks).
func readDiskKind(name string) string {
	data, err := os.ReadFile(filepath.Join("/sys/block", name, "queue", "rotational"))
	if err != nil {
		return ""
	}
	switch strings.TrimSpace(string(data)) {
	case "1":
		return "HDD"
	case "0":
		return "SSD"
	default:
		return ""
	}
}

// buildPartitionParents maps every real block-device name — each disk's
// partitions, and the disk itself for the no-partition-table case — to the
// physical disk it belongs to. It's read straight from sysfs's own
// disk/partition hierarchy, the same parent/child relationship `lsblk`
// reports: a device only counts as "sda's partition" here because the
// kernel itself says so (a "partition" file exists under it in sysfs), never
// because its name merely looks like one.
func buildPartitionParents(diskNames []string) map[string]string {
	parents := map[string]string{}
	for _, disk := range diskNames {
		parents[disk] = disk // covers a disk mounted directly, with no partition table
		entries, err := os.ReadDir(filepath.Join("/sys/block", disk))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if _, err := os.Stat(filepath.Join("/sys/block", disk, name, "partition")); err != nil {
				continue // not a kernel-recognized partition of this disk
			}
			parents[name] = disk
		}
	}
	return parents
}

// partitionParentDisk maps a block-device name (as it appears under /dev,
// without the leading path) to the physical disk it lives on, using the
// disk/partition map buildPartitionParents derived from sysfs. A device not
// found there directly is an LVM/dm-mapper volume or an md RAID device, so
// it's resolved by following /sys/class/block/<name>/slaves one level down
// to the underlying partition and looking that up instead. Returns "" if it
// can't be attributed to exactly one physical disk.
func partitionParentDisk(devName string, parents map[string]string) string {
	if p, ok := parents[devName]; ok {
		return p
	}
	slaves, err := os.ReadDir(filepath.Join("/sys/class/block", devName, "slaves"))
	if err != nil || len(slaves) != 1 {
		return "" // no slaves, or spans multiple disks (e.g. striped RAID) — can't attribute to one
	}
	return partitionParentDisk(slaves[0].Name(), parents)
}

// readDiskPartitions discovers mounted, data-bearing partitions and groups
// them by the physical disk they live on (via partitionParentDisk). Every
// mount is required to be a genuine whole-filesystem mount of a real
// partition/disk block device — see discoverPartitionMounts — so bind
// mounts of a subdirectory or single file (as Docker uses to expose
// /etc/hostname, /etc/hosts, /etc/resolv.conf, /var/log, volumes, ... from
// the host) are never mistaken for a partition, no matter what they're
// mounted at or what device they share. Technical partitions (EFI/ESP, a
// standalone /boot, ...) are then dropped by isTechnicalMount; swap and MSR
// partitions carry no filesystem, so they were never mountable in the first
// place and need no special-casing.
//
// hostRootPath, when set (a bind mount of the host's real "/" — see
// discoverPartitionMounts for why hostProcPath alone can't get us this),
// is where each mountpoint's actual bytes are read from; the mountpoint
// reported in the result is always the host's real one, never the
// translated path.
//
// Without hostRootPath, statfs-ing a mountpoint discovered through another
// namespace's mountinfo (hostProcPath pointing at a bind-mounted host
// /proc) would silently read *this* container's own filesystem instead —
// e.g. its overlay root just happens to statfs successfully at "/" — and
// misattribute those numbers to the host partition. Rather than risk
// reporting plausible-looking but wrong usage, this bails out to no
// partitions at all (disks themselves still show fine) unless either
// hostRootPath gives a real translated path, or hostProcPath is the plain
// "/proc" default that means we're not containerized in the first place.
func readDiskPartitions(parents map[string]string, hostProcPath, hostRootPath string) map[string][]domain.PartitionInfo {
	if hostRootPath == "" && hostProcPath != "/proc" {
		return nil
	}
	byDisk := map[string][]domain.PartitionInfo{}
	for _, m := range discoverPartitionMounts(hostProcPath) {
		statPath := m.mount
		if hostRootPath != "" {
			statPath = filepath.Join(hostRootPath, m.mount)
		}
		d, ok := statMount(mountEntry{device: m.device, mount: statPath, fstype: m.fstype})
		if !ok {
			continue // e.g. hostRootPath isn't actually mounted/reachable — skip rather than show zeros
		}
		if isTechnicalMount(m.mount, d.FSType, d.TotalBytes) {
			continue
		}
		devName := strings.TrimPrefix(m.device, "/dev/")
		parent := partitionParentDisk(devName, parents)
		if parent == "" {
			continue // couldn't attribute to exactly one physical disk (e.g. a striped RAID volume) — skip rather than guess
		}
		byDisk[parent] = append(byDisk[parent], domain.PartitionInfo{
			Device: devName, FSType: d.FSType, Mount: m.mount,
			TotalBytes: d.TotalBytes, UsedBytes: d.UsedBytes, AvailBytes: d.AvailBytes, UsedPct: d.UsedPct,
		})
	}
	for name := range byDisk {
		sort.Slice(byDisk[name], func(i, j int) bool { return byDisk[name][i].Mount < byDisk[name][j].Mount })
	}
	return byDisk
}

// discoverPartitionMounts reads PID 1's /proc/[pid]/mountinfo — richer than
// /proc/mounts, since it also reports each mount's "root": the path *within
// the source filesystem* that got mounted there — through hostProcPath, the
// same "read PID 1's namespace instead of our own" trick readSockets and
// ConnWatcher already rely on (see sockets.go): inside a container, this
// process's own mount namespace is just the container's, so /proc/self/
// mountinfo would only ever show the container's own near-irrelevant mounts.
// PID 1's is the host's real, full mount table when hostProcPath is a
// mounted host /proc (auto-detected as /hostproc), and this process's own
// otherwise.
//
// A genuine mount of an entire partition always has root "/". Anything
// else — root being some subpath — means it's a bind mount of just that
// subdirectory or file, which is exactly how Docker exposes /etc/hostname,
// /etc/hosts, /etc/resolv.conf, /var/log, and volumes from the host: same
// underlying device as the host's real partition, but not the partition's
// own mount. Filtering on root "/" is what actually tells partitions apart
// from those, rather than guessing from the mountpoint name — and, read
// this way, container-private bind mounts don't even show up in the first
// place, since they live in a mount namespace of their own that PID 1 never
// sees.
func discoverPartitionMounts(hostProcPath string) []mountEntry {
	data, err := os.ReadFile(filepath.Join(hostProcPath, "1", "mountinfo"))
	if err != nil {
		return nil
	}
	var out []mountEntry
	for _, line := range strings.Split(string(data), "\n") {
		halves := strings.SplitN(line, " - ", 2)
		if len(halves) != 2 {
			continue
		}
		left := strings.Fields(halves[0])
		right := strings.Fields(halves[1])
		if len(left) < 5 || len(right) < 2 {
			continue
		}
		root, mount := unescapeMountField(left[3]), unescapeMountField(left[4])
		fstype, device := right[0], right[1]
		if root != "/" {
			continue // bind mount of a subpath, not the filesystem's own mount
		}
		if pseudoFSTypes[fstype] || !strings.HasPrefix(device, "/dev/") {
			continue
		}
		out = append(out, mountEntry{device: device, mount: mount, fstype: fstype})
	}
	return out
}

// unescapeMountField decodes the octal escapes (e.g. \040 for a space) that
// /proc/*/mountinfo and /proc/mounts use for whitespace and backslashes
// inside paths.
func unescapeMountField(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// espMaxBytes is the largest size a real-world EFI System Partition is ever
// created at; a FAT-family filesystem this small is almost certainly one,
// wherever it happens to be mounted.
const espMaxBytes = 2 << 30 // 2 GiB

// isTechnicalMount reports whether a mounted partition is firmware/bootstrap
// storage rather than somewhere real system or user data lives, so it can be
// kept out of the dashboard's partitions list: an EFI System Partition (by
// mountpoint convention or, wherever it's mounted, a small FAT-family
// filesystem) or a standalone /boot.
func isTechnicalMount(mount, fstype string, totalBytes uint64) bool {
	if mount == "/boot" || mount == "/efi" || strings.HasPrefix(mount, "/boot/") {
		return true
	}
	switch fstype {
	case "vfat", "msdos", "exfat":
		return totalBytes > 0 && totalBytes <= espMaxBytes
	}
	return false
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
