// Package netmon watches the server's network connections in general —
// TCP and UDP, any port, either direction — independent of which service
// handles them. It is intentionally decoupled from internal/security's
// port-curated ConnWatcher (built for "alert on my game server port") and
// from internal/collector (the 1Hz domain.Metrics tick).
package netmon

import (
	"context"
	"fmt"
	"go-data/internal/domain"
	"go-data/internal/eventlog"
	"log"
	"sync"
	"time"
)

const (
	CapabilityConntrack = "conntrack"
	CapabilityProcTCP   = "proc_tcp_only"
)

// PointWriter is the subset of *storage.InfluxStore netmon needs — kept as
// a tiny interface so netmon doesn't depend on the concrete Influx client
// type, and reuses the app's single already-configured Influx connection
// rather than opening a second one.
type PointWriter interface {
	WritePoint(measurement string, tags map[string]string, fields map[string]interface{}, t time.Time) error
}

type Config struct {
	ProcPath      string // cfg.HostProcPath
	DataDir       string
	ForceFallback bool
	IPHistoryCap  int // 0 → default
	// IncludeOutbound also tracks flows this host initiates (updates,
	// DNS, cloudflared's own tunnel to Cloudflare, ...). Off by default:
	// the point of netmon is who connects TO the server.
	IncludeOutbound bool
}

type Monitor struct {
	cfg    Config
	influx PointWriter

	ct         *conntrackSource // nil once/if running in fallback mode
	capability string

	mu    sync.RWMutex
	flows []domain.NetFlow

	events  *eventlog.Log[domain.NetFlowEvent]
	ipStore *ipHistoryStore
	history *sampleHistory

	prevActive map[string]bool

	logDegradeOnce sync.Once
}

func New(cfg Config, influx PointWriter) *Monitor {
	if cfg.IPHistoryCap <= 0 {
		cfg.IPHistoryCap = 10000
	}
	m := &Monitor{
		cfg:        cfg,
		influx:     influx,
		capability: CapabilityConntrack,
		events:     eventlog.New[domain.NetFlowEvent](cfg.DataDir, "netmon_events.jsonl", 1000),
		ipStore:    newIPHistoryStore(cfg.DataDir, cfg.IPHistoryCap),
		history:    newSampleHistory(360),
		prevActive: map[string]bool{},
	}

	if !cfg.ForceFallback {
		ct, err := openConntrack()
		if err != nil {
			log.Printf("netmon: conntrack unavailable, falling back to TCP-only visibility: %v", err)
			m.capability = CapabilityProcTCP
		} else {
			m.ct = ct
		}
	} else {
		m.capability = CapabilityProcTCP
	}
	m.ipStore.Prune(discoverLocalNet().isLocal)
	return m
}

func (m *Monitor) Run(ctx context.Context, interval time.Duration) {
	defer func() {
		if m.ct != nil {
			m.ct.Close()
		}
		m.ipStore.Close()
	}()
	m.poll()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.poll()
		}
	}
}

func (m *Monitor) poll() {
	raw, capability := m.collect()
	boundTCP, boundUDP := boundPorts(m.cfg.ProcPath)
	flows := resolveDirection(raw, discoverLocalNet(), boundTCP, boundUDP)
	if !m.cfg.IncludeOutbound {
		flows = inboundOnly(flows)
	}
	now := time.Now()

	active := make(map[string]bool, len(flows))
	sample := domain.NetSample{Time: now}
	ips := map[string]bool{}
	// batched: a port scan opens thousands of flows in one poll, and each
	// Append rewrites the whole event file
	var events []domain.NetFlowEvent

	for _, f := range flows {
		key := fmt.Sprintf("%s|%d|%d|%s", f.RemoteIP, f.RemotePort, f.LocalPort, f.Proto)
		active[key] = true
		ips[f.RemoteIP] = true

		sample.ActiveTotal++
		if f.Proto == "tcp" {
			sample.ActiveTCP++
		} else {
			sample.ActiveUDP++
		}
		if f.Direction == "inbound" {
			sample.Inbound++
		} else {
			sample.Outbound++
		}

		isNew := !m.prevActive[key]
		m.ipStore.Observe(f.RemoteIP, now, isNew)
		if isNew {
			events = append(events, domain.NetFlowEvent{
				Time: now, RemoteIP: f.RemoteIP, RemotePort: f.RemotePort,
				LocalPort: f.LocalPort, Proto: f.Proto, Direction: f.Direction,
			})
		}
	}
	m.events.Append(events...)
	sample.UniqueIPs = len(ips)
	m.prevActive = active

	m.mu.Lock()
	m.flows = flows
	m.capability = capability
	m.mu.Unlock()

	m.history.Append(sample)
	m.writeInflux(sample)
}

// collect tries conntrack first (if it was opened successfully at
// startup); a Dump failure permanently downgrades this Monitor to the
// proc/net/tcp fallback for the rest of its lifetime, mirroring how
// DockerCollector degrades once and logs it, rather than flapping every
// poll.
func (m *Monitor) collect() ([]rawFlow, string) {
	if m.ct != nil {
		raw, err := m.ct.dump()
		if err == nil {
			return raw, CapabilityConntrack
		}
		m.logDegradeOnce.Do(func() {
			log.Printf("netmon: conntrack read failed, falling back to TCP-only visibility: %v", err)
		})
		m.ct.Close()
		m.ct = nil
	}
	fb := newProcTCPFallback(m.cfg.ProcPath)
	return fb.dump(), CapabilityProcTCP
}

// resolveDirection classifies each raw flow as inbound/outbound and splits
// its two ends into remote/local — the same way regardless of which
// collection mechanism produced the raw data.
//
// Direction comes from addresses first: the original tuple's initiator is
// the source, so an external source hitting a local destination is inbound
// and a local (or container, via NAT) source reaching out is outbound.
// Flows where both ends are local — loopback, host↔container,
// container↔container — are host-internal and dropped: they're never a
// real remote peer. Only when neither end is recognisably local (addresses
// changed between polls, routed traffic) does it fall back to the
// bound-port heuristic.
func resolveDirection(raw []rawFlow, ln localNet, boundTCP, boundUDP map[int]bool) []domain.NetFlow {
	out := make([]domain.NetFlow, 0, len(raw))
	for _, f := range raw {
		srcLocal, dstLocal := ln.isLocal(f.SrcIP), ln.isLocal(f.DstIP)
		if srcLocal && dstLocal {
			continue
		}

		inbound := dstLocal
		if !srcLocal && !dstLocal {
			bound := boundTCP
			if f.Proto == "udp" {
				bound = boundUDP
			}
			inbound = bound[f.DstPort] || !bound[f.SrcPort]
		}

		var remoteIP string
		var remotePort, localPort int
		var direction string
		if inbound {
			direction, remoteIP, remotePort, localPort = "inbound", f.SrcIP.String(), f.SrcPort, f.DstPort
		} else {
			direction, remoteIP, remotePort, localPort = "outbound", f.DstIP.String(), f.DstPort, f.SrcPort
		}

		out = append(out, domain.NetFlow{
			RemoteIP: remoteIP, RemotePort: remotePort, LocalPort: localPort,
			Proto: f.Proto, Direction: direction, State: f.State,
		})
	}
	return out
}

// inboundOnly filters flows in place down to the inbound ones.
func inboundOnly(flows []domain.NetFlow) []domain.NetFlow {
	out := flows[:0]
	for _, f := range flows {
		if f.Direction == "inbound" {
			out = append(out, f)
		}
	}
	return out
}

func (m *Monitor) writeInflux(s domain.NetSample) {
	if m.influx == nil {
		return
	}
	m.influx.WritePoint("network", nil, map[string]interface{}{
		"active_total": s.ActiveTotal,
		"active_tcp":   s.ActiveTCP,
		"active_udp":   s.ActiveUDP,
		"unique_ips":   s.UniqueIPs,
		"inbound":      s.Inbound,
		"outbound":     s.Outbound,
	}, s.Time)
}

// Snapshot returns the current, live connection table.
func (m *Monitor) Snapshot() []domain.NetFlow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.NetFlow, len(m.flows))
	copy(out, m.flows)
	return out
}

// Capability reports which collection mechanism is currently active.
func (m *Monitor) Capability() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.capability
}

// HostWide reports whether the underlying /proc view reaches beyond this
// process's own container (i.e. HostProcPath is a mounted host /proc).
func (m *Monitor) HostWide() bool {
	return m.cfg.ProcPath == "/hostproc"
}

// IncludesOutbound reports whether outbound flows are tracked at all.
func (m *Monitor) IncludesOutbound() bool {
	return m.cfg.IncludeOutbound
}

// SampleHistory returns the buffered NetSample series for /api/network/history.
func (m *Monitor) SampleHistory() []domain.NetSample {
	return m.history.GetAll()
}

func (m *Monitor) RecentEvents(n int) []domain.NetFlowEvent {
	return m.events.Recent(n)
}

func (m *Monitor) IPHistory(n int) []domain.NetIPHistory {
	return m.ipStore.Recent(n)
}

// Top returns the top-n (ip|port, count) pairs from the current snapshot,
// computed on demand — cheap enough not to warrant persisting separately.
func (m *Monitor) Top(by string, n int) []domain.NetCount {
	m.mu.RLock()
	flows := make([]domain.NetFlow, len(m.flows))
	copy(flows, m.flows)
	m.mu.RUnlock()

	counts := map[string]int{}
	for _, f := range flows {
		var key string
		switch by {
		case "port":
			key = fmt.Sprintf("%d/%s", f.LocalPort, f.Proto)
		default:
			key = f.RemoteIP
		}
		counts[key]++
	}
	return topN(counts, n)
}

func topN(counts map[string]int, n int) []domain.NetCount {
	out := make([]domain.NetCount, 0, len(counts))
	for k, c := range counts {
		out = append(out, domain.NetCount{Key: k, Count: c})
	}
	// simple insertion sort descending by count — n is always small (a
	// handful to a few hundred distinct keys), no need for sort.Slice's
	// overhead considerations here, but using it anyway for clarity:
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Count > out[j-1].Count; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if n > 0 && n < len(out) {
		out = out[:n]
	}
	return out
}
