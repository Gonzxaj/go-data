package collector

import (
	"go-data/internal/domain"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type netSample struct {
	rxBytes, rxPackets, rxErrs, rxDrop uint64
	txBytes, txPackets, txErrs, txDrop uint64
	at                                 time.Time
}

// readNetDev reads /proc/net/dev, skipping the loopback interface.
//
// /proc/net/dev is scoped per network namespace: inside a container it would
// only ever show the container's own virtual interface, not the host's real
// NIC. Reading it through PID 1's namespace (the host's, when procPath is a
// mounted host /proc) gives the host's actual interfaces instead.
func readNetDev(procPath string) map[string]netSample {
	data, err := os.ReadFile(filepath.Join(procPath, "1", "net", "dev"))
	if err != nil {
		return nil
	}
	now := time.Now()
	out := map[string]netSample{}
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.Contains(l, ":") {
			continue
		}
		parts := strings.SplitN(l, ":", 2)
		name := strings.TrimSpace(parts[0])
		if name == "" || name == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		get := func(i int) uint64 {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			return v
		}
		out[name] = netSample{
			rxBytes: get(0), rxPackets: get(1), rxErrs: get(2), rxDrop: get(3),
			txBytes: get(8), txPackets: get(9), txErrs: get(10), txDrop: get(11),
			at: now,
		}
	}
	return out
}

func netDeltas(prev, cur map[string]netSample) []domain.NetIface {
	var out []domain.NetIface
	for name, c := range cur {
		p, ok := prev[name]
		if !ok {
			continue
		}
		dSeconds := c.at.Sub(p.at).Seconds()
		if dSeconds <= 0 || c.rxBytes < p.rxBytes || c.txBytes < p.txBytes {
			continue
		}
		out = append(out, domain.NetIface{
			Name:      name,
			RxBps:     float64(c.rxBytes-p.rxBytes) / dSeconds,
			TxBps:     float64(c.txBytes-p.txBytes) / dSeconds,
			PacketsPS: float64((c.rxPackets - p.rxPackets) + (c.txPackets - p.txPackets)) / dSeconds,
			ErrorsPS:  float64((c.rxErrs - p.rxErrs) + (c.txErrs - p.txErrs)) / dSeconds,
			DropsPS:   float64((c.rxDrop - p.rxDrop) + (c.txDrop - p.txDrop)) / dSeconds,
		})
	}
	return out
}
