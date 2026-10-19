package netmon

import (
	"bufio"
	"encoding/json"
	"go-data/internal/domain"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// ipStoreNewFlush bounds how long a brand-new IP can sit unpersisted;
	// ipStoreStaleFlush does the same for mere last_seen/connection bumps
	// on already-known IPs, which matter much less.
	ipStoreNewFlush   = 10 * time.Second
	ipStoreStaleFlush = 60 * time.Second
)

// ipHistoryStore tracks first/last-seen + connection count per remote IP.
// Unlike eventlog.Log, entries are upserted in place, not appended — so it
// needs its own, different persistence shape: a full dump of the current
// map, not a growing list.
//
// Persistence is write-throttled: observations only touch memory and mark
// the store dirty; a background loop rewrites the file at most every
// ipStoreNewFlush when a new IP appeared, or every ipStoreStaleFlush for
// plain bumps. That keeps a burst of new IPs (a port scan from thousands of
// sources) to a handful of full-file rewrites instead of one per IP. Losing
// a few seconds of history on a crash is fine — live state always comes
// from Monitor.Snapshot() regardless of what's on disk.
type ipHistoryStore struct {
	path string
	cap  int

	mu        sync.Mutex
	entries   map[string]*domain.NetIPHistory
	dirty     bool
	newIPs    bool
	lastFlush time.Time

	stop chan struct{}
	done chan struct{}
}

func newIPHistoryStore(dataDir string, capacity int) *ipHistoryStore {
	if dataDir == "" {
		dataDir = "./data"
	}
	os.MkdirAll(dataDir, 0o700)
	s := &ipHistoryStore{
		path:      filepath.Join(dataDir, "netmon_ip_history.jsonl"),
		cap:       capacity,
		entries:   map[string]*domain.NetIPHistory{},
		lastFlush: time.Now(),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	s.load()
	go s.flushLoop()
	return s
}

// Observe records that ip was seen at t; newFlow is true when this sighting
// is a flow that wasn't open on the previous poll, and only then does the
// IP's connection count go up.
func (s *ipHistoryStore) Observe(ip string, t time.Time, newFlow bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, existed := s.entries[ip]
	if !existed {
		e = &domain.NetIPHistory{IP: ip, FirstSeen: t}
		s.entries[ip] = e
		s.newIPs = true
	}
	e.LastSeen = t
	if newFlow || !existed {
		e.Connections++
	}
	s.dirty = true
	if !existed {
		s.evictIfOverCapLocked()
	}
}

// Prune drops every entry drop reports true for — used at startup to clear
// out host-internal addresses (loopback, container bridges) recorded by
// older versions that didn't filter them.
func (s *ipHistoryStore) Prune(drop func(ip net.IP) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ip := range s.entries {
		if drop(net.ParseIP(ip)) {
			delete(s.entries, ip)
			s.dirty = true
		}
	}
}

// evictIfOverCapLocked trims down to 90% of cap in one pass, dropping the
// least-recently-seen IPs, so the O(n log n) sort runs once per ~cap/10 new
// IPs rather than an O(n) scan on every one past the cap.
func (s *ipHistoryStore) evictIfOverCapLocked() {
	if len(s.entries) <= s.cap {
		return
	}
	all := make([]*domain.NetIPHistory, 0, len(s.entries))
	for _, e := range s.entries {
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].LastSeen.Before(all[j].LastSeen) })
	keep := max(s.cap*9/10, 1)
	for _, e := range all[:len(all)-keep] {
		delete(s.entries, e.IP)
	}
}

func (s *ipHistoryStore) flushLoop() {
	defer close(s.done)
	t := time.NewTicker(ipStoreNewFlush)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			s.mu.Lock()
			if s.dirty {
				s.persistLocked()
			}
			s.mu.Unlock()
			return
		case now := <-t.C:
			s.mu.Lock()
			if s.dirty && (s.newIPs || now.Sub(s.lastFlush) >= ipStoreStaleFlush) {
				s.persistLocked()
			}
			s.mu.Unlock()
		}
	}
}

// Close flushes any pending changes and stops the background loop.
func (s *ipHistoryStore) Close() {
	close(s.stop)
	<-s.done
}

// Recent returns up to n entries, most-recently-seen first.
func (s *ipHistoryStore) Recent(n int) []domain.NetIPHistory {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]domain.NetIPHistory, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	if n > 0 && n < len(out) {
		out = out[:n]
	}
	return out
}

func (s *ipHistoryStore) load() {
	f, err := os.Open(s.path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e domain.NetIPHistory
		if err := json.Unmarshal(scanner.Bytes(), &e); err == nil {
			// Files written before connections replaced the per-poll
			// "count" field load with 0; every recorded IP was seen at
			// least once.
			if e.Connections == 0 {
				e.Connections = 1
			}
			cp := e
			s.entries[e.IP] = &cp
		}
	}
}

func (s *ipHistoryStore) persistLocked() {
	s.dirty = false
	s.newIPs = false
	s.lastFlush = time.Now()
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, e := range s.entries {
		b, err := json.Marshal(e)
		if err != nil {
			continue
		}
		w.Write(b)
		w.WriteByte('\n')
	}
	w.Flush()
	f.Close()
	os.Rename(tmp, s.path)
}
