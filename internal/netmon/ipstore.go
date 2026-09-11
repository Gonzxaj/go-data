package netmon

import (
	"bufio"
	"encoding/json"
	"go-data/internal/domain"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ipHistoryStore tracks first/last-seen + sighting count per remote IP.
// Unlike eventlog.Log, entries are upserted in place, not appended — so it
// needs its own, different persistence shape: a full dump of the current
// map, not a growing list.
//
// Persistence is write-throttled: a brand-new IP (or an eviction) persists
// immediately (the durability-worthy event — losing "we've never seen this
// IP" is bad); an already-tracked IP just bumps last_seen/count in memory
// and marks the store dirty, flushed by a slow periodic ticker instead of
// on every single observation. Losing a few tens of seconds of last_seen
// staleness for a still-open connection is fine — live state always comes
// from Monitor.Snapshot() regardless of what's on disk.
type ipHistoryStore struct {
	path string
	cap  int

	mu      sync.Mutex
	entries map[string]*domain.NetIPHistory
	dirty   bool
}

func newIPHistoryStore(dataDir string, capacity int) *ipHistoryStore {
	if dataDir == "" {
		dataDir = "./data"
	}
	os.MkdirAll(dataDir, 0o755)
	s := &ipHistoryStore{
		path:    filepath.Join(dataDir, "netmon_ip_history.jsonl"),
		cap:     capacity,
		entries: map[string]*domain.NetIPHistory{},
	}
	s.load()
	go s.flushLoop()
	return s
}

func (s *ipHistoryStore) Observe(ip string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, existed := s.entries[ip]
	if !existed {
		e = &domain.NetIPHistory{IP: ip, FirstSeen: t}
		s.entries[ip] = e
	}
	e.LastSeen = t
	e.Count++

	if !existed {
		s.evictIfOverCapLocked()
		s.persistLocked()
	} else {
		s.dirty = true
	}
}

func (s *ipHistoryStore) evictIfOverCapLocked() {
	if len(s.entries) <= s.cap {
		return
	}
	var oldestIP string
	var oldest time.Time
	for ip, e := range s.entries {
		if oldestIP == "" || e.LastSeen.Before(oldest) {
			oldestIP, oldest = ip, e.LastSeen
		}
	}
	if oldestIP != "" {
		delete(s.entries, oldestIP)
	}
}

func (s *ipHistoryStore) flushLoop() {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		if s.dirty {
			s.persistLocked()
		}
		s.mu.Unlock()
	}
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
			cp := e
			s.entries[e.IP] = &cp
		}
	}
}

func (s *ipHistoryStore) persistLocked() {
	s.dirty = false
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
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
