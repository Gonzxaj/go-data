// Package eventlog provides a small, capped, disk-persisted append log for
// transient events (things worth reviewing later even though they're not a
// numeric time series). It's domain-agnostic — used by the SSH watcher, the
// connection watcher, and the network monitor alike.
package eventlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Log is a small, capped, append-only-on-disk log of recent events of type
// T. It mirrors the pattern used by internal/uptime.Log: rewritten whole on
// every append (cheap at this size), so it survives process restarts
// without needing a real database.
type Log[T any] struct {
	path string
	cap  int

	mu     sync.Mutex
	events []T
}

func New[T any](dataDir, filename string, capacity int) *Log[T] {
	if dataDir == "" {
		dataDir = "./data"
	}
	os.MkdirAll(dataDir, 0o700)
	l := &Log[T]{path: filepath.Join(dataDir, filename), cap: capacity}
	l.events = l.load()
	return l
}

// Append adds events and rewrites the file once for the whole batch:
// callers pass everything from one poll together, so a burst (port scan,
// SSH brute force) costs one rewrite instead of one per event.
func (l *Log[T]) Append(es ...T) {
	if len(es) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, es...)
	if len(l.events) > l.cap {
		l.events = l.events[len(l.events)-l.cap:]
	}
	l.persist()
}

// Recent returns up to n events, most recent first.
func (l *Log[T]) Recent(n int) []T {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.events) {
		n = len(l.events)
	}
	out := make([]T, n)
	for i := 0; i < n; i++ {
		out[i] = l.events[len(l.events)-1-i]
	}
	return out
}

func (l *Log[T]) load() []T {
	f, err := os.Open(l.path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e T
		if err := json.Unmarshal(scanner.Bytes(), &e); err == nil {
			out = append(out, e)
		}
	}
	return out
}

func (l *Log[T]) persist() {
	tmp := l.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, e := range l.events {
		b, err := json.Marshal(e)
		if err != nil {
			continue
		}
		w.Write(b)
		w.WriteByte('\n')
	}
	w.Flush()
	f.Close()
	os.Rename(tmp, l.path)
}
