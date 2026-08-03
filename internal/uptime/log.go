package uptime

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxSessions = 200

type Session struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end"`
}

func (s Session) Ongoing() bool { return s.End == nil }

func (s Session) Duration(now time.Time) time.Duration {
	if s.End != nil {
		return s.End.Sub(s.Start)
	}
	return now.Sub(s.Start)
}

// Log tracks host reboot sessions in a small append-only-on-disk file,
// independent of whichever Storage backends (memory/influx) are enabled.
// A "session" is a contiguous stretch where the host's boot time didn't
// change; a new session means the host actually rebooted.
type Log struct {
	path string

	mu           sync.Mutex
	sessions     []Session
	lastBootTime time.Time
}

func NewLog(dataDir string) *Log {
	if dataDir == "" {
		dataDir = "./data"
	}
	os.MkdirAll(dataDir, 0o755)
	l := &Log{path: filepath.Join(dataDir, "uptime_sessions.jsonl")}
	l.sessions = l.load()
	if n := len(l.sessions); n > 0 {
		l.lastBootTime = l.sessions[n-1].Start
	}
	return l
}

// Observe should be called once per collector tick with the current time and
// the host's current boot time (now - /proc/uptime). It detects reboots by
// noticing the boot time changed since the last call.
func (l *Log) Observe(now time.Time, bootTime time.Time) {
	bootTime = bootTime.Round(time.Second)

	l.mu.Lock()
	defer l.mu.Unlock()

	switch {
	case len(l.sessions) == 0:
		l.sessions = append(l.sessions, Session{Start: bootTime})
		l.lastBootTime = bootTime
		l.persist()
	case bootTime.Equal(l.lastBootTime):
		// same session, nothing to persist every tick
	default:
		// boot time changed: close the previous session, open a new one
		end := now
		l.sessions[len(l.sessions)-1].End = &end
		l.sessions = append(l.sessions, Session{Start: bootTime})
		l.lastBootTime = bootTime
		if len(l.sessions) > maxSessions {
			l.sessions = l.sessions[len(l.sessions)-maxSessions:]
		}
		l.persist()
	}
}

// Sessions returns the known sessions, most recent first.
func (l *Log) Sessions() []Session {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Session, len(l.sessions))
	for i, s := range l.sessions {
		out[len(l.sessions)-1-i] = s
	}
	return out
}

func (l *Log) load() []Session {
	f, err := os.Open(l.path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []Session
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var s Session
		if err := json.Unmarshal(scanner.Bytes(), &s); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// persist rewrites the whole file — sessions are rare events (reboots), so
// the file stays tiny (at most maxSessions lines) and a full rewrite is
// simpler and safer than patching a specific line in place.
func (l *Log) persist() {
	tmp := l.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, s := range l.sessions {
		b, err := json.Marshal(s)
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

// ReadBootTime reads /proc/uptime and returns now - uptime, i.e. the moment
// the host booted.
func ReadBootTime(now time.Time) time.Time {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return now
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return now
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return now
	}
	return now.Add(-time.Duration(secs * float64(time.Second)))
}
