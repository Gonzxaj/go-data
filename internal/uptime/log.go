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

const (
	maxSessions = 200
	// bootJitter absorbs /proc/uptime rounding: boot times computed a
	// second apart are the same boot, not a reboot.
	bootJitter = 5 * time.Second
	// lastSeenPersist bounds how stale last_seen can be on disk, which is
	// also the precision of the downtime measured across a reboot.
	lastSeenPersist = time.Minute
)

type Session struct {
	Start time.Time `json:"start"`
	// End is the last moment the host was seen up in this session (sessions
	// written before last_seen existed have the reboot-detection time instead).
	End *time.Time `json:"end"`
	// LastSeen is the latest observation of the ongoing session, so a
	// reboot can close it at when the host was last up, not when it came back.
	LastSeen *time.Time `json:"last_seen,omitempty"`
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
	lastPersist  time.Time
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
	case absDur(bootTime.Sub(l.lastBootTime)) <= bootJitter:
		cur := &l.sessions[len(l.sessions)-1]
		seen := now
		cur.LastSeen = &seen
		if now.Sub(l.lastPersist) >= lastSeenPersist {
			l.persist()
		}
	default:
		// boot time changed: close the previous session at its last
		// sighting (falls back to now for files without last_seen)
		prev := &l.sessions[len(l.sessions)-1]
		end := now
		if prev.LastSeen != nil {
			end = *prev.LastSeen
		}
		prev.End, prev.LastSeen = &end, nil
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
	l.lastPersist = time.Now()
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

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
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
