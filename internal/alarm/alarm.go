package alarm

import (
	"go-data/internal/domain"
	"sync"
	"time"
)

type Thresholds struct {
	CPUWarn, CPUCrit     float64
	RAMCrit              float64
	DiskWarn, DiskCrit   float64
	IOWaitCrit           float64
	StealWarn, StealCrit float64
	DiskLatencyCrit      float64
	TempWarn, TempCrit   float64
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		CPUWarn: 80, CPUCrit: 90,
		RAMCrit:  90,
		DiskWarn: 85, DiskCrit: 95,
		IOWaitCrit:      20,
		StealWarn:       5,
		StealCrit:       15,
		DiskLatencyCrit: 50,
		TempWarn:        70,
		TempCrit:        85,
	}
}

type Alarm struct {
	ID       string    `json:"id"`
	Metric   string    `json:"metric"`
	Severity string    `json:"severity"` // "warn" | "crit"
	Message  string    `json:"message"`
	Value    float64   `json:"value"`
	Since    time.Time `json:"since"`
}

// Evaluator runs each collector tick against the latest snapshot and keeps
// track of how long each active alarm has been firing.
type Evaluator struct {
	th     Thresholds
	active map[string]Alarm

	mu   sync.RWMutex
	last []Alarm
}

func NewEvaluator(th Thresholds) *Evaluator {
	return &Evaluator{th: th, active: map[string]Alarm{}}
}

// Latest returns the alarm list computed on the most recent Evaluate call,
// safe for concurrent reads from an HTTP handler.
func (e *Evaluator) Latest() []Alarm {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Alarm, len(e.last))
	copy(out, e.last)
	return out
}

// Evaluate raises system-level alarms only (CPU, RAM, disk, temperature) —
// container state is already shown visually in the Docker tab, not duplicated
// here as an alarm.
func (e *Evaluator) Evaluate(m domain.Metrics) []Alarm {
	now := m.Time
	next := map[string]Alarm{}

	raise := func(id, metric, severity, message string, value float64) {
		if a, ok := e.active[id]; ok {
			a.Severity, a.Message, a.Value = severity, message, value
			next[id] = a
		} else {
			next[id] = Alarm{ID: id, Metric: metric, Severity: severity, Message: message, Value: value, Since: now}
		}
	}

	if m.CPU.UsagePct >= e.th.CPUCrit {
		raise("cpu.usage", "cpu.usage_pct", "crit", "CPU muy alto", m.CPU.UsagePct)
	} else if m.CPU.UsagePct >= e.th.CPUWarn {
		raise("cpu.usage", "cpu.usage_pct", "warn", "CPU alto", m.CPU.UsagePct)
	}

	if m.RAM.UsedPct >= e.th.RAMCrit {
		raise("ram.used", "ram.used_pct", "crit", "RAM muy alta", m.RAM.UsedPct)
	}

	if m.CPU.IOWaitPct >= e.th.IOWaitCrit {
		raise("cpu.iowait", "cpu.iowait_pct", "crit", "iowait alto (posible cuello de botella de disco)", m.CPU.IOWaitPct)
	}

	if m.CPU.StealPct >= e.th.StealCrit {
		raise("cpu.steal", "cpu.steal_pct", "crit", "CPU robada por el hypervisor", m.CPU.StealPct)
	} else if m.CPU.StealPct >= e.th.StealWarn {
		raise("cpu.steal", "cpu.steal_pct", "warn", "CPU robada por el hypervisor", m.CPU.StealPct)
	}

	for _, d := range m.Disks {
		id := "disk." + d.Mount + ".used"
		if d.UsedPct >= e.th.DiskCrit {
			raise(id, "disk.used_pct", "crit", "Disco "+d.Mount+" casi lleno", d.UsedPct)
		} else if d.UsedPct >= e.th.DiskWarn {
			raise(id, "disk.used_pct", "warn", "Disco "+d.Mount+" alto", d.UsedPct)
		}
	}

	for _, io := range m.DiskIO {
		if io.LatencyMs >= e.th.DiskLatencyCrit {
			raise("diskio."+io.Device+".latency", "disk_io.latency_ms", "crit", "Latencia de disco alta en "+io.Device, io.LatencyMs)
		}
	}

	for _, t := range m.Temp {
		if !t.IsPrimaryCPU {
			continue
		}
		if t.Celsius >= e.th.TempCrit {
			raise("temp."+t.Zone, "temp.celsius", "crit", "Temperatura de CPU muy alta", t.Celsius)
		} else if t.Celsius >= e.th.TempWarn {
			raise("temp."+t.Zone, "temp.celsius", "warn", "Temperatura de CPU elevada", t.Celsius)
		}
	}

	e.active = next

	out := make([]Alarm, 0, len(next))
	for _, a := range next {
		out = append(out, a)
	}

	e.mu.Lock()
	e.last = out
	e.mu.Unlock()

	return out
}
