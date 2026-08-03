package collector

import (
	"go-data/internal/domain"
	"os"
	"strconv"
	"strings"
)

func readMemInfo() (domain.RAM, domain.Swap) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return domain.RAM{}, domain.Swap{}
	}
	vals := map[string]uint64{}
	for _, l := range strings.Split(string(data), "\n") {
		fields := strings.Fields(l)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		vals[key] = v
	}

	total := vals["MemTotal"]
	free := vals["MemFree"]
	buffers := vals["Buffers"]
	cached := vals["Cached"]
	sreclaimable := vals["SReclaimable"]
	shmem := vals["Shmem"]
	var cache uint64
	if cached+sreclaimable > shmem {
		cache = cached + sreclaimable - shmem
	}
	available := vals["MemAvailable"]
	if available == 0 {
		available = free + buffers + cache
	}
	if available > total {
		available = total
	}
	used := total - available

	ram := domain.RAM{
		TotalKB: total, UsedKB: used, AvailableKB: available,
		FreeKB: free, CachedKB: cache, BuffersKB: buffers,
	}
	if total > 0 {
		ram.UsedPct = 100 * float64(used) / float64(total)
		ram.AvailablePct = 100 * float64(available) / float64(total)
		ram.FreePct = 100 * float64(free) / float64(total)
		ram.CachePct = 100 * float64(cache) / float64(total)
	}

	swapTotal := vals["SwapTotal"]
	swapFree := vals["SwapFree"]
	swapUsed := swapTotal - swapFree
	swap := domain.Swap{TotalKB: swapTotal, UsedKB: swapUsed, Present: swapTotal > 0}
	if swapTotal > 0 {
		swap.UsedPct = 100 * float64(swapUsed) / float64(swapTotal)
	}

	return ram, swap
}
