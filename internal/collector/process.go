package collector

import (
	"go-data/internal/domain"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const clockTicksPerSec = 100 // USER_HZ, effectively always 100 on Linux

type procSample struct {
	ticks uint64
	at    time.Time
}

// ProcessCollector tracks per-PID CPU ticks across calls to compute %CPU,
// mirroring what apps.plugin/top do. It is meant to be called once per
// system-metrics tick (same cadence as SystemCollector).
//
// procPath is "/proc" when unmounted, or a mounted host /proc (e.g.
// "/hostproc"). Unlike /proc/net/*, /proc/<pid>/stat isn't scoped per
// network namespace, so reading it straight through the mounted host /proc
// (no PID-1 indirection needed) already gives the host's real process list
// instead of just this container's own PID namespace.
type ProcessCollector struct {
	procPath string
	prev     map[int]procSample
}

func NewProcessCollector(procPath string) *ProcessCollector {
	return &ProcessCollector{procPath: procPath, prev: map[int]procSample{}}
}

// Top returns the n processes with the highest CPU% since the previous call.
func (p *ProcessCollector) Top(n int) []domain.ProcessInfo {
	entries, err := os.ReadDir(p.procPath)
	if err != nil {
		return nil
	}
	now := time.Now()
	cur := map[int]procSample{}
	var out []domain.ProcessInfo

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		name, ticks, ok := readProcStat(p.procPath, pid)
		if !ok {
			continue
		}
		cur[pid] = procSample{ticks: ticks, at: now}

		prev, ok := p.prev[pid]
		if !ok || ticks < prev.ticks {
			continue // no history yet, or PID reused (ticks can't decrease) — skip this tick
		}
		dSeconds := now.Sub(prev.at).Seconds()
		if dSeconds <= 0 {
			continue
		}
		cpuPct := 100 * float64(ticks-prev.ticks) / clockTicksPerSec / dSeconds
		out = append(out, domain.ProcessInfo{PID: pid, Name: name, CPUPct: cpuPct})
	}

	p.prev = cur // self-cleaning: exited PIDs simply aren't carried forward

	sort.Slice(out, func(i, j int) bool { return out[i].CPUPct > out[j].CPUPct })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// readProcStat parses /proc/<pid>/stat, returning comm and utime+stime ticks.
func readProcStat(procPath string, pid int) (name string, ticks uint64, ok bool) {
	data, err := os.ReadFile(filepath.Join(procPath, strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", 0, false
	}
	line := string(data)
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close < 0 || close < open {
		return "", 0, false
	}
	name = line[open+1 : close]
	rest := strings.Fields(line[close+1:])
	// rest[0]=state, [1]=ppid, ... [11]=utime, [12]=stime (0-indexed from state)
	if len(rest) < 13 {
		return "", 0, false
	}
	utime, _ := strconv.ParseUint(rest[11], 10, 64)
	stime, _ := strconv.ParseUint(rest[12], 10, 64)
	return name, utime + stime, true
}
