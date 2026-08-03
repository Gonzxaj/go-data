// Package security watches host-level signals the mini monitoring system
// doesn't get from /proc alone: SSH authentication attempts (from the auth
// log) and incoming TCP connections to configured ports (e.g. game servers),
// so both can be reviewed later even though they're transient events, not
// numeric time series.
package security

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// EventLog is a small, capped, append-only-on-disk log of recent events of
// type T. It mirrors the pattern used by internal/uptime.Log: rewritten
// whole on every append (cheap at this size), so it survives process
// restarts without needing a real database.
type EventLog[T any] struct {
	path string
	cap  int

	mu     sync.Mutex
	events []T
}

func NewEventLog[T any](dataDir, filename string, capacity int) *EventLog[T] {
	if dataDir == "" {
		dataDir = "./data"
	}
	os.MkdirAll(dataDir, 0o755)
	l := &EventLog[T]{path: filepath.Join(dataDir, filename), cap: capacity}
	l.events = l.load()
	return l
}

func (l *EventLog[T]) Append(e T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
	if len(l.events) > l.cap {
		l.events = l.events[len(l.events)-l.cap:]
	}
	l.persist()
}

// Recent returns up to n events, most recent first.
func (l *EventLog[T]) Recent(n int) []T {
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

func (l *EventLog[T]) load() []T {
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

func (l *EventLog[T]) persist() {
	tmp := l.path + ".tmp"
	f, err := os.Create(tmp)
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
