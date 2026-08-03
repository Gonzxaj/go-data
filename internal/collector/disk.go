package collector

import (
	"go-data/internal/domain"
	"os"
	"strings"
	"syscall"
)

var pseudoFSTypes = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "tmpfs": true, "devpts": true,
	"cgroup": true, "cgroup2": true, "overlay": true, "squashfs": true, "autofs": true,
	"mqueue": true, "debugfs": true, "tracefs": true, "securityfs": true, "pstore": true,
	"bpf": true, "configfs": true, "fusectl": true, "hugetlbfs": true, "nsfs": true,
	"binfmt_misc": true, "efivarfs": true, "rpc_pipefs": true,
}

type mountEntry struct {
	device, mount, fstype string
}

// discoverMounts reads /proc/mounts and filters out pseudo/virtual filesystems,
// always making sure "/" is present.
func discoverMounts() []mountEntry {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return []mountEntry{{device: "/", mount: "/", fstype: "unknown"}}
	}
	var out []mountEntry
	haveRoot := false
	for _, l := range strings.Split(string(data), "\n") {
		fields := strings.Fields(l)
		if len(fields) < 3 {
			continue
		}
		device, mount, fstype := fields[0], fields[1], fields[2]
		if pseudoFSTypes[fstype] || !strings.HasPrefix(device, "/dev/") {
			continue
		}
		out = append(out, mountEntry{device: device, mount: mount, fstype: fstype})
		if mount == "/" {
			haveRoot = true
		}
	}
	if !haveRoot {
		out = append([]mountEntry{{device: "/", mount: "/", fstype: "unknown"}}, out...)
	}
	return out
}

func statMount(m mountEntry) (domain.Disk, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(m.mount, &st); err != nil {
		return domain.Disk{}, false
	}
	bsize := uint64(st.Bsize)
	total := st.Blocks * bsize
	free := st.Bfree * bsize
	avail := st.Bavail * bsize
	used := total - free

	d := domain.Disk{
		Mount: m.mount, Device: m.device, FSType: m.fstype,
		TotalBytes: total, UsedBytes: used, AvailBytes: avail,
	}
	if total > 0 {
		d.UsedPct = 100 * float64(used) / float64(total)
	}
	return d, true
}

// resolveDiskMounts decides the list of mount paths to report on: either the
// configured list, or auto-discovery, always ensuring "/" is included.
func resolveDiskMounts(configured []string) []mountEntry {
	if len(configured) == 0 {
		return discoverMounts()
	}
	var out []mountEntry
	for _, m := range configured {
		out = append(out, mountEntry{device: m, mount: m, fstype: "configured"})
	}
	return out
}

func readDisks(mounts []mountEntry) []domain.Disk {
	var out []domain.Disk
	for _, m := range mounts {
		if d, ok := statMount(m); ok {
			out = append(out, d)
		}
	}
	return out
}
