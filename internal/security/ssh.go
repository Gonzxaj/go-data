package security

import (
	"context"
	"go-data/internal/domain"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	reFailed      = regexp.MustCompile(`Failed password for (?:invalid user )?(\S+) from ([0-9a-fA-F.:]+) port \d+`)
	reAccepted    = regexp.MustCompile(`Accepted \S+ for (\S+) from ([0-9a-fA-F.:]+) port \d+`)
	reInvalidUser = regexp.MustCompile(`Invalid user (\S+) from ([0-9a-fA-F.:]+) port \d+`)
	reClosed      = regexp.MustCompile(`Connection closed by (?:(?:invalid|authenticating) user \S+ )?([0-9a-fA-F.:]+) port \d+(?: \[preauth\])?`)
)

// candidateLogPaths is tried in order when no explicit path is configured.
var candidateLogPaths = []string{"/var/log/auth.log", "/var/log/secure"}

// SSHWatcher tails the sshd auth log, parsing new lines as they're appended.
// It is resilient to log rotation (detected via a shrinking file size or a
// changed inode) by reopening the file from the start in that case, and to
// the log not existing *yet* — e.g. rsyslog getting installed on the host
// after this container already started — by retrying path resolution on
// every poll instead of giving up permanently at construction time.
type SSHWatcher struct {
	explicitPath string // "" means auto-detect from candidateLogPaths
	dataDir      string

	path   string // resolved lazily, once found
	log    *EventLog[domain.SSHEvent]
	offset int64
	inode  uint64
}

// NewSSHWatcher always succeeds — path resolution happens lazily on each
// poll, so the feature stays "enabled" and self-heals if the log file shows
// up later (e.g. rsyslog gets installed after this container already
// started), instead of requiring a container restart.
func NewSSHWatcher(path, dataDir string) *SSHWatcher {
	return &SSHWatcher{
		explicitPath: path,
		dataDir:      dataDir,
		log:          NewEventLog[domain.SSHEvent](dataDir, "ssh_events.jsonl", 500),
	}
}

// Run polls for new lines until ctx is cancelled. Once the log file is
// found, it starts from the current end of it — only events from now on are
// reported, so a restart doesn't replay the whole history as "new" alerts.
func (w *SSHWatcher) Run(ctx context.Context, interval time.Duration) {
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

func (w *SSHWatcher) poll() {
	if w.path == "" {
		if !w.resolvePath() {
			return // log file not there yet — try again next tick
		}
	}

	info, err := os.Stat(w.path)
	if err != nil {
		w.path = "" // it existed before and doesn't anymore — re-resolve next time
		return
	}
	if inode := inodeOf(info); inode != 0 && inode != w.inode {
		w.offset = 0 // log rotated: a new file was created at this path
		w.inode = inode
	} else if info.Size() < w.offset {
		w.offset = 0 // truncated in place
	}
	if info.Size() <= w.offset {
		return
	}

	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(w.offset, 0); err != nil {
		return
	}
	buf := make([]byte, info.Size()-w.offset)
	n, _ := f.Read(buf)
	w.offset += int64(n)

	for _, line := range strings.Split(string(buf[:n]), "\n") {
		if e, ok := parseSSHLine(line); ok {
			w.log.Append(e)
		}
	}
}

// resolvePath finds the log file (explicit path or auto-detected) and, if
// found, seeks to its current end so only future lines are reported.
func (w *SSHWatcher) resolvePath() bool {
	path := w.explicitPath
	if path == "" {
		for _, c := range candidateLogPaths {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}
	info, err := os.Stat(path)
	if path == "" || err != nil {
		return false
	}
	w.path = path
	w.offset = info.Size()
	w.inode = inodeOf(info)
	return true
}

func (w *SSHWatcher) Recent(n int) []domain.SSHEvent {
	return w.log.Recent(n)
}

func parseSSHLine(line string) (domain.SSHEvent, bool) {
	if !strings.Contains(line, "sshd[") {
		return domain.SSHEvent{}, false
	}
	now := time.Now()

	if m := reAccepted.FindStringSubmatch(line); m != nil {
		return domain.SSHEvent{Time: now, User: m[1], IP: m[2], Action: "accepted", Success: true, Detail: line}, true
	}
	if m := reFailed.FindStringSubmatch(line); m != nil {
		return domain.SSHEvent{Time: now, User: m[1], IP: m[2], Action: "failed", Success: false, Detail: line}, true
	}
	if m := reInvalidUser.FindStringSubmatch(line); m != nil {
		return domain.SSHEvent{Time: now, User: m[1], IP: m[2], Action: "invalid_user", Success: false, Detail: line}, true
	}
	if m := reClosed.FindStringSubmatch(line); m != nil {
		return domain.SSHEvent{Time: now, IP: m[1], Action: "closed", Success: false, Detail: line}, true
	}
	return domain.SSHEvent{}, false
}
