package security

import (
	"bytes"
	"context"
	"go-data/internal/domain"
	"go-data/internal/eventlog"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// The username is attacker-controlled and may contain spaces, so a client
// can log in as "Accepted password for root from 6.6.6.6 port 22". Hence
// the message is matched only after the line's first (real) sshd tag,
// anchored at both ends, with the IP taken from the line's end where sshd
// itself writes it. sshd-session is OpenSSH ≥ 9.8's per-connection process.
var (
	reSSHTag      = regexp.MustCompile(`sshd(?:-session)?\[\d+\]: `)
	reFailed      = regexp.MustCompile(`^Failed password for (?:invalid user )?(.*) from ([0-9a-fA-F.:]+) port \d+ ssh2$`)
	reAccepted    = regexp.MustCompile(`^Accepted \S+ for (\S+) from ([0-9a-fA-F.:]+) port \d+`)
	reInvalidUser = regexp.MustCompile(`^Invalid user (.*) from ([0-9a-fA-F.:]+) port \d+$`)
	reClosed      = regexp.MustCompile(`^Connection closed by (?:(?:invalid|authenticating) user .* )?([0-9a-fA-F.:]+) port \d+(?: \[preauth\])?$`)
)

// maxSSHRead caps one poll's read, so a rotated multi-GB log isn't
// slurped into memory at once; the rest is picked up on later polls.
const maxSSHRead = 4 << 20

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
	log    *eventlog.Log[domain.SSHEvent]
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
		log:          eventlog.New[domain.SSHEvent](dataDir, "ssh_events.jsonl", 500),
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
	buf := make([]byte, min(info.Size()-w.offset, maxSSHRead))
	n, _ := io.ReadFull(f, buf)
	data := buf[:n]
	if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
		data = data[:i+1] // a trailing partial line is re-read whole next poll
	} else if n < maxSSHRead {
		return // still mid-line
	} // else: a single >4MB "line" is garbage; skip it

	w.offset += int64(len(data))

	var events []domain.SSHEvent
	for _, line := range strings.Split(string(data), "\n") {
		if e, ok := parseSSHLine(line); ok {
			events = append(events, e)
		}
	}
	w.log.Append(events...)
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
	line = strings.TrimSuffix(line, "\r")
	loc := reSSHTag.FindStringIndex(line) // leftmost = the real tag; the username comes after it
	if loc == nil {
		return domain.SSHEvent{}, false
	}
	msg := line[loc[1]:]
	now := time.Now()

	if m := reAccepted.FindStringSubmatch(msg); m != nil {
		return domain.SSHEvent{Time: now, User: m[1], IP: m[2], Action: "accepted", Success: true, Detail: line}, true
	}
	if m := reFailed.FindStringSubmatch(msg); m != nil {
		return domain.SSHEvent{Time: now, User: m[1], IP: m[2], Action: "failed", Success: false, Detail: line}, true
	}
	if m := reInvalidUser.FindStringSubmatch(msg); m != nil {
		return domain.SSHEvent{Time: now, User: m[1], IP: m[2], Action: "invalid_user", Success: false, Detail: line}, true
	}
	if m := reClosed.FindStringSubmatch(msg); m != nil {
		return domain.SSHEvent{Time: now, IP: m[1], Action: "closed", Success: false, Detail: line}, true
	}
	return domain.SSHEvent{}, false
}
