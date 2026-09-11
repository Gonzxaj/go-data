package netmon

import (
	"go-data/internal/procnet"
	"net"
	"path/filepath"
)

// rawFlow is the common shape both collection mechanisms (conntrack and
// the proc/net/tcp fallback) produce: the two ends of a flow, as first
// observed, with no notion yet of which end is "local" vs "remote" — that
// gets resolved uniformly by resolveDirection against the currently-bound
// local ports, regardless of which mechanism produced the raw data.
type rawFlow struct {
	SrcIP   net.IP
	SrcPort int
	DstIP   net.IP
	DstPort int
	Proto   string // "tcp" | "udp"
	State   string
}

// procTCPFallback provides TCP-only flow visibility via /proc/net/tcp{,6}
// when conntrack isn't available. UDP gets no per-remote-IP visibility in
// this mode (see boundPorts, which fallbackFlows also relies on for the
// "which ports do we even have services on" half of the picture) — a
// /proc/net/udp bind-only socket has no per-peer entries to read.
type procTCPFallback struct {
	procPath string
}

func newProcTCPFallback(procPath string) *procTCPFallback {
	return &procTCPFallback{procPath: procPath}
}

func (p *procTCPFallback) dump() []rawFlow {
	var flows []rawFlow
	for _, path := range procFiles(p.procPath, "tcp") {
		for _, e := range procnet.ReadEntries(path) {
			if e.State != procnet.TCPEstablished {
				continue
			}
			flows = append(flows, rawFlow{
				SrcIP: e.RemoteIP, SrcPort: e.RemotePort,
				DstIP: e.LocalIP, DstPort: e.LocalPort,
				Proto: "tcp", State: "established",
			})
		}
	}
	return flows
}

func procFiles(procPath, proto string) []string {
	base := filepath.Join(procPath, "1", "net")
	return []string{filepath.Join(base, proto), filepath.Join(base, proto+"6")}
}

// boundPorts returns the set of local ports currently bound/listening,
// split by protocol — the same "auto-discover watched ports" idea already
// used by security.ConnWatcher (any port in LISTEN state, not on
// loopback), generalized to udp (any bound socket, since UDP has no LISTEN
// state) and to every port rather than a curated few. This is what lets
// both collection mechanisms classify inbound vs outbound identically.
func boundPorts(procPath string) (tcp map[int]bool, udp map[int]bool) {
	tcp, udp = map[int]bool{}, map[int]bool{}
	for _, path := range procFiles(procPath, "tcp") {
		for _, e := range procnet.ReadEntries(path) {
			if e.State == procnet.TCPListen && !e.LocalIP.IsLoopback() {
				tcp[e.LocalPort] = true
			}
		}
	}
	for _, path := range procFiles(procPath, "udp") {
		for _, e := range procnet.ReadEntries(path) {
			if e.RemotePort == 0 && !e.LocalIP.IsLoopback() {
				udp[e.LocalPort] = true
			}
		}
	}
	return tcp, udp
}
