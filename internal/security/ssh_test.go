package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSSHLine(t *testing.T) {
	cases := []struct {
		line    string
		wantOK  bool
		ip      string
		user    string
		action  string
		success bool
	}{
		{
			line:    `Jul 31 17:35:01 host sshd[1234]: Failed password for invalid user admin from 45.83.66.10 port 51000 ssh2`,
			wantOK:  true, ip: "45.83.66.10", user: "admin", action: "failed", success: false,
		},
		{
			line:    `Jul 31 17:35:01 host sshd[1234]: Accepted publickey for gonzxa from 192.168.0.5 port 51000 ssh2: ED25519 SHA256:abc`,
			wantOK:  true, ip: "192.168.0.5", user: "gonzxa", action: "accepted", success: true,
		},
		{
			line:    `Jul 31 17:35:01 host sshd[1234]: Accepted password for gonzxa from 192.168.0.5 port 22001 ssh2`,
			wantOK:  true, ip: "192.168.0.5", user: "gonzxa", action: "accepted", success: true,
		},
		{
			line:    `Jul 31 17:35:01 host sshd[1234]: Invalid user root from 45.83.66.10 port 51000`,
			wantOK:  true, ip: "45.83.66.10", user: "root", action: "invalid_user", success: false,
		},
		{
			line:   `Jul 31 17:35:01 host sshd[1234]: Connection closed by authenticating user root 45.83.66.10 port 51000 [preauth]`,
			wantOK: true, ip: "45.83.66.10", action: "closed", success: false,
		},
		{
			line:   `Jul 31 17:35:01 host sudo: gonzxa : TTY=pts/0 ; PWD=/home/gonzxa ; USER=root ; COMMAND=/bin/ls`,
			wantOK: false,
		},
	}

	for _, c := range cases {
		e, ok := parseSSHLine(c.line)
		if ok != c.wantOK {
			t.Fatalf("parseSSHLine(%q) ok=%v want %v", c.line, ok, c.wantOK)
		}
		if !ok {
			continue
		}
		if e.IP != c.ip {
			t.Errorf("line %q: IP=%q want %q", c.line, e.IP, c.ip)
		}
		if c.user != "" && e.User != c.user {
			t.Errorf("line %q: User=%q want %q", c.line, e.User, c.user)
		}
		if e.Action != c.action {
			t.Errorf("line %q: Action=%q want %q", c.line, e.Action, c.action)
		}
		if e.Success != c.success {
			t.Errorf("line %q: Success=%v want %v", c.line, e.Success, c.success)
		}
	}
}

func TestDecodeHexAddr(t *testing.T) {
	if got := decodeHexAddr("0100007F"); got == nil || got.String() != "127.0.0.1" {
		t.Fatalf("decodeHexAddr(IPv4 loopback) = %v", got)
	}
	if got := decodeHexAddr("bogus"); got != nil {
		t.Fatalf("decodeHexAddr(invalid) = %v, want nil", got)
	}
}

func TestDecodeHexPort(t *testing.T) {
	if got := decodeHexPort("0050"); got != 80 {
		t.Fatalf("decodeHexPort(0050) = %d, want 80", got)
	}
}

func TestSSHWatcherPollDetectsNewLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "auth.log")
	if err := os.WriteFile(logPath, []byte("old line, should be ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewSSHWatcher(logPath, dir)
	w.poll() // resolves the path lazily and seeks to the current end, like Run()'s first tick

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("Jul 31 10:00:00 h sshd[1]: Failed password for invalid user admin from 1.2.3.4 port 4444 ssh2\n")
	f.Close()

	w.poll()

	got := w.Recent(10)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].IP != "1.2.3.4" || got[0].Action != "failed" {
		t.Fatalf("event = %+v", got[0])
	}

	// a second poll with no new data should not duplicate the event
	w.poll()
	if got := w.Recent(10); len(got) != 1 {
		t.Fatalf("after no-op poll: got %d events, want 1", len(got))
	}
}

// TestSSHWatcherSelfHealsWhenLogAppearsLater covers the real-world case: the
// container starts before the host has any auth log (e.g. rsyslog gets
// installed afterwards) — the watcher must pick it up on its own on a later
// poll instead of staying disabled until the container is restarted.
func TestSSHWatcherSelfHealsWhenLogAppearsLater(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "auth.log")

	w := NewSSHWatcher(logPath, dir)

	w.poll() // log file doesn't exist yet
	if got := w.Recent(10); len(got) != 0 {
		t.Fatalf("before the log exists: got %d events, want 0", len(got))
	}

	// the log file shows up later (e.g. rsyslog just got installed)
	if err := os.WriteFile(logPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	w.poll() // should now resolve the path and seek to its (empty) end

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("Jul 31 10:00:00 h sshd[1]: Accepted publickey for root from 5.6.7.8 port 22 ssh2\n")
	f.Close()

	w.poll()

	got := w.Recent(10)
	if len(got) != 1 || got[0].IP != "5.6.7.8" {
		t.Fatalf("got %+v, want one event from 5.6.7.8", got)
	}
}
