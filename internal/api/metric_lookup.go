package api

import "go-data/internal/domain"

// metricValue resolves a fixed, backend-defined metric key (plus an optional
// selector for per-mount/per-device/per-interface metrics) against a
// snapshot. This is intentionally a closed enum, not open-ended path
// parsing, so the frontend never has to know about internal JSON shape.
func metricValue(m domain.Metrics, metric, selector string) (float64, bool) {
	switch metric {
	case "cpu.usage_pct":
		return m.CPU.UsagePct, true
	case "cpu.user_pct":
		return m.CPU.UserPct, true
	case "cpu.system_pct":
		return m.CPU.SystemPct, true
	case "cpu.iowait_pct":
		return m.CPU.IOWaitPct, true
	case "cpu.steal_pct":
		return m.CPU.StealPct, true
	case "cpu.idle_pct":
		return m.CPU.IdlePct, true

	case "load.load1":
		return m.Load.Load1, true
	case "load.load5":
		return m.Load.Load5, true
	case "load.load15":
		return m.Load.Load15, true
	case "load.normalized_pct":
		return m.Load.NormalizedPct, true

	case "ram.used_pct":
		return m.RAM.UsedPct, true
	case "ram.available_pct":
		return m.RAM.AvailablePct, true
	case "ram.free_pct":
		return m.RAM.FreePct, true
	case "ram.cache_pct":
		return m.RAM.CachePct, true

	case "swap.used_pct":
		return m.Swap.UsedPct, true

	case "disk.used_pct", "disk.avail_bytes", "disk.total_bytes":
		mount := selector
		if mount == "" {
			mount = "/"
		}
		for _, d := range m.Disks {
			if d.Mount == mount {
				switch metric {
				case "disk.used_pct":
					return d.UsedPct, true
				case "disk.avail_bytes":
					return float64(d.AvailBytes), true
				case "disk.total_bytes":
					return float64(d.TotalBytes), true
				}
			}
		}
		return 0, false

	case "disk_io.read_bps", "disk_io.write_bps", "disk_io.read_iops", "disk_io.write_iops", "disk_io.latency_ms":
		return diskIOValue(m.DiskIO, metric, selector)

	case "net.rx_bps", "net.tx_bps", "net.packets_ps", "net.errors_ps", "net.drops_ps":
		return netValue(m.Net, metric, selector)

	case "temp.celsius":
		return tempValue(m.Temp, selector)

	default:
		return 0, false
	}
}

func diskIOValue(all []domain.DiskIO, metric, device string) (float64, bool) {
	if device != "" {
		for _, io := range all {
			if io.Device == device {
				return diskIOField(io, metric), true
			}
		}
		return 0, false
	}
	if len(all) == 0 {
		return 0, false
	}
	var sum, max float64
	for _, io := range all {
		v := diskIOField(io, metric)
		sum += v
		if v > max {
			max = v
		}
	}
	if metric == "disk_io.latency_ms" {
		return max, true
	}
	return sum, true
}

func diskIOField(io domain.DiskIO, metric string) float64 {
	switch metric {
	case "disk_io.read_bps":
		return io.ReadBps
	case "disk_io.write_bps":
		return io.WriteBps
	case "disk_io.read_iops":
		return io.ReadIOPS
	case "disk_io.write_iops":
		return io.WriteIOPS
	case "disk_io.latency_ms":
		return io.LatencyMs
	}
	return 0
}

func netValue(all []domain.NetIface, metric, iface string) (float64, bool) {
	if iface != "" && iface != "total" {
		for _, n := range all {
			if n.Name == iface {
				return netField(n, metric), true
			}
		}
		return 0, false
	}
	if len(all) == 0 {
		return 0, false
	}
	var sum float64
	for _, n := range all {
		sum += netField(n, metric)
	}
	return sum, true
}

func netField(n domain.NetIface, metric string) float64 {
	switch metric {
	case "net.rx_bps":
		return n.RxBps
	case "net.tx_bps":
		return n.TxBps
	case "net.packets_ps":
		return n.PacketsPS
	case "net.errors_ps":
		return n.ErrorsPS
	case "net.drops_ps":
		return n.DropsPS
	}
	return 0
}

func tempValue(all []domain.Temp, zone string) (float64, bool) {
	if zone != "" {
		for _, t := range all {
			if t.Zone == zone {
				return t.Celsius, true
			}
		}
		return 0, false
	}
	for _, t := range all {
		if t.IsPrimaryCPU {
			return t.Celsius, true
		}
	}
	if len(all) > 0 {
		return all[0].Celsius, true
	}
	return 0, false
}
