package collector

import (
	"go-data/internal/domain"
	"os"
	"strconv"
	"strings"
)

type cpuSample struct {
	user, nice, system, idle, iowait, irq, softirq, steal uint64
}

func (s cpuSample) total() uint64 {
	return s.user + s.nice + s.system + s.idle + s.iowait + s.irq + s.softirq + s.steal
}

// readCPUSamples reads /proc/stat and returns the aggregate "cpu" line and
// every per-core "cpuN" line, in core-number order.
func readCPUSamples() (aggregate cpuSample, perCore []cpuSample, ok bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSample{}, nil, false
	}
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) < 8 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		s := parseCPUFields(fields[1:])
		if fields[0] == "cpu" {
			aggregate = s
			ok = true
		} else {
			perCore = append(perCore, s)
		}
	}
	return aggregate, perCore, ok
}

func parseCPUFields(f []string) cpuSample {
	get := func(i int) uint64 {
		if i >= len(f) {
			return 0
		}
		v, _ := strconv.ParseUint(f[i], 10, 64)
		return v
	}
	return cpuSample{
		user: get(0), nice: get(1), system: get(2), idle: get(3),
		iowait: get(4), irq: get(5), softirq: get(6), steal: get(7),
	}
}

// cpuPct computes usage%, user%, system%, iowait%, steal%, idle% between two samples.
func cpuDeltaPct(prev, cur cpuSample) domain.CPU {
	dTotal := float64(cur.total() - prev.total())
	if dTotal <= 0 {
		return domain.CPU{}
	}
	dUser := float64(cur.user - prev.user)
	dSystem := float64(cur.system - prev.system)
	dIOWait := float64(cur.iowait - prev.iowait)
	dSteal := float64(cur.steal - prev.steal)
	dIdle := float64(cur.idle - prev.idle)
	return domain.CPU{
		UsagePct:  100 * (1 - (dIdle+dIOWait)/dTotal),
		UserPct:   100 * dUser / dTotal,
		SystemPct: 100 * dSystem / dTotal,
		IOWaitPct: 100 * dIOWait / dTotal,
		StealPct:  100 * dSteal / dTotal,
		IdlePct:   100 * dIdle / dTotal,
	}
}
