package security

import (
	"context"
	"go-data/internal/domain"
	"go-data/internal/eventlog"
	"go-data/internal/procnet"
	"path/filepath"
	"strconv"
	"time"
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
	log           *eventlog.Log[domain.ConnectionEvent]

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
		log:           eventlog.New[domain.ConnectionEvent](dataDir, "connection_events.jsonl", 500),
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
	var entries []procnet.Entry
	for _, path := range w.tcpFiles() {
		entries = append(entries, procnet.ReadEntries(path)...)
	}

	watched := map[int]bool{}
	for p := range w.explicitPorts {
		watched[p] = true
	}
	if w.auto {
		for _, e := range entries {
			if e.State == procnet.TCPListen && !e.LocalIP.IsLoopback() {
				watched[e.LocalPort] = true
			}
		}
	}

	active := map[string]bool{}
	var events []domain.ConnectionEvent
	for _, e := range entries {
		if e.State != procnet.TCPEstablished || !watched[e.LocalPort] {
			continue
		}
		key := e.RemoteIP.String() + "|" + strconv.Itoa(e.LocalPort)
		active[key] = true
		if !w.prevActive[key] {
			events = append(events, domain.ConnectionEvent{Time: time.Now(), IP: e.RemoteIP.String(), LocalPort: e.LocalPort})
		}
	}
	w.log.Append(events...)
	w.prevActive = active
}

func (w *ConnWatcher) Recent(n int) []domain.ConnectionEvent {
	return w.log.Recent(n)
}
