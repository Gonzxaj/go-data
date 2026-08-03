package collector

import (
	"go-data/internal/domain"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var realDiskRe = regexp.MustCompile(`^(sd[a-z]+|nvme\d+n\d+|vd[a-z]+|xvd[a-z]+)$`)

const sectorBytes = 512

type diskIOSample struct {
	reads, writes         uint64
	sectorsRead, sectorsWritten uint64
	ioTimeMs              uint64
	at                    time.Time
}

// readDiskStats reads /proc/diskstats and returns raw samples for every real
// (non-partition, non-loop/dm) block device.
func readDiskStats() map[string]diskIOSample {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil
	}
	now := time.Now()
	out := map[string]diskIOSample{}
	for _, l := range strings.Split(string(data), "\n") {
		fields := strings.Fields(l)
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if !realDiskRe.MatchString(name) {
			continue
		}
		get := func(i int) uint64 {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			return v
		}
		out[name] = diskIOSample{
			reads:          get(3),
			sectorsRead:    get(5),
			writes:         get(7),
			sectorsWritten: get(9),
			ioTimeMs:       get(12),
			at:             now,
		}
	}
	return out
}

func diskIODeltas(prev, cur map[string]diskIOSample) []domain.DiskIO {
	var out []domain.DiskIO
	for name, c := range cur {
		p, ok := prev[name]
		if !ok {
			continue
		}
		dSeconds := c.at.Sub(p.at).Seconds()
		if dSeconds <= 0 || c.reads < p.reads || c.writes < p.writes {
			continue
		}
		dReads := float64(c.reads - p.reads)
		dWrites := float64(c.writes - p.writes)
		dReadSectors := float64(c.sectorsRead - p.sectorsRead)
		dWriteSectors := float64(c.sectorsWritten - p.sectorsWritten)
		dIOTime := float64(c.ioTimeMs - p.ioTimeMs)

		io := domain.DiskIO{
			Device:    name,
			ReadBps:   dReadSectors * sectorBytes / dSeconds,
			WriteBps:  dWriteSectors * sectorBytes / dSeconds,
			ReadIOPS:  dReads / dSeconds,
			WriteIOPS: dWrites / dSeconds,
		}
		if ops := dReads + dWrites; ops > 0 {
			io.LatencyMs = dIOTime / ops
		}
		out = append(out, io)
	}
	return out
}
