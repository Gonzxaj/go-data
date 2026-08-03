package security

import (
	"bufio"
	"context"
	"encoding/hex"
	"go-data/internal/domain"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	tcpStateEstablished = "01"
	tcpStateListen      = "0A"
)

// ConnWatcher periodically scans the TCP connection table for established
// connections into a set of watched local ports, logging the first time
// each remote IP is seen — useful when there's no application-level log of
// "who connected" (unlike SSH's auth log).
//
// The watched set is, by default, auto-discovered every poll from whichever
// ports are actually LISTENing and bound to something other than loopback
// (so an internal-only service like a local Postgres on 127.0.0.1 is never
// included, but a game server, a web server, etc. are) — no need to name
// ports up front. Explicit ports can still be added on top of that, e.g. to
// track a port before the service is even up.
type ConnWatcher struct {
	explicitPorts map[int]bool
	auto          bool
	procPath      string // "/proc" when unmounted, or a mounted host /proc (e.g. "/hostproc")
	log           *EventLog[domain.ConnectionEvent]

	prevActive map[string]bool
}

func NewConnWatcher(explicitPorts []int, auto bool, procPath, dataDir string) *ConnWatcher {
	m := make(map[int]bool, len(explicitPorts))
	for _, p := range explicitPorts {
		m[p] = true
	}
	return &ConnWatcher{
		explicitPorts: m,
		auto:          auto,
		procPath:      procPath,
		log:           NewEventLog[domain.ConnectionEvent](dataDir, "connection_events.jsonl", 500),
		prevActive:    map[string]bool{},
	}
}

func (w *ConnWatcher) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.poll()
		}
	}
}

// tcpFiles targets PID 1's network namespace: when procPath is this
// process's own /proc, PID 1 is in the same (container's) namespace as us;
// when procPath is a mounted *host* /proc, PID 1 is the host's real init,
// so this transparently also picks up connections into sibling
// containers/services on the host, not just this process.
func (w *ConnWatcher) tcpFiles() []string {
	base := filepath.Join(w.procPath, "1", "net")
	return []string{filepath.Join(base, "tcp"), filepath.Join(base, "tcp6")}
}

func (w *ConnWatcher) poll() {
	var entries []tcpEntry
	for _, path := range w.tcpFiles() {
		entries = append(entries, readTCPEntries(path)...)
	}

	watched := map[int]bool{}
	for p := range w.explicitPorts {
		watched[p] = true
	}
	if w.auto {
		for _, e := range entries {
			if e.state == tcpStateListen && !e.localIP.IsLoopback() {
				watched[e.localPort] = true
			}
		}
	}

	active := map[string]bool{}
	for _, e := range entries {
		if e.state != tcpStateEstablished || !watched[e.localPort] {
			continue
		}
		key := e.remoteIP + "|" + strconv.Itoa(e.localPort)
		active[key] = true
		if !w.prevActive[key] {
			w.log.Append(domain.ConnectionEvent{Time: time.Now(), IP: e.remoteIP, LocalPort: e.localPort})
		}
	}
	w.prevActive = active
}

func (w *ConnWatcher) Recent(n int) []domain.ConnectionEvent {
	return w.log.Recent(n)
}

type tcpEntry struct {
	localIP   net.IP
	localPort int
	remoteIP  string
	state     string
}

func readTCPEntries(path string) []tcpEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []tcpEntry
	scanner := bufio.NewScanner(f)
	scanner.Scan() // header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		localParts := strings.SplitN(fields[1], ":", 2)
		remoteParts := strings.SplitN(fields[2], ":", 2)
		if len(localParts) != 2 || len(remoteParts) != 2 {
			continue
		}
		localIP := decodeHexAddr(localParts[0])
		localPort := decodeHexPort(localParts[1])
		remoteIP := decodeHexAddr(remoteParts[0])
		if localIP == nil || localPort == 0 || remoteIP == nil {
			continue
		}
		out = append(out, tcpEntry{
			localIP: localIP, localPort: localPort,
			remoteIP: remoteIP.String(), state: strings.ToUpper(fields[3]),
		})
	}
	return out
}

func decodeHexPort(h string) int {
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0
	}
	return int(v)
}

// decodeHexAddr decodes /proc/net/tcp{,6}'s address encoding: each 4-byte
// group is stored little-endian, so within every 8-hex-char chunk the byte
// order must be reversed (IPv6 has 4 such chunks concatenated).
func decodeHexAddr(h string) net.IP {
	if len(h) != 8 && len(h) != 32 {
		return nil
	}
	raw := make([]byte, 0, 16)
	for i := 0; i < len(h); i += 8 {
		chunk, err := hex.DecodeString(h[i : i+8])
		if err != nil || len(chunk) != 4 {
			return nil
		}
		raw = append(raw, chunk[3], chunk[2], chunk[1], chunk[0])
	}
	if len(raw) == 4 {
		return net.IPv4(raw[0], raw[1], raw[2], raw[3])
	}
	return net.IP(raw)
}
