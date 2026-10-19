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
			line:   `Jul 31 17:35:01 host sshd[1234]: Failed password for invalid user admin from 45.83.66.10 port 51000 ssh2`,
			wantOK: true, ip: "45.83.66.10", user: "admin", action: "failed", success: false,
		},
		{
			line:   `Jul 31 17:35:01 host sshd[1234]: Accepted publickey for gonzxa from 192.168.0.5 port 51000 ssh2: ED25519 SHA256:abc`,
			wantOK: true, ip: "192.168.0.5", user: "gonzxa", action: "accepted", success: true,
		},
		{
			line:   `Jul 31 17:35:01 host sshd[1234]: Accepted password for gonzxa from 192.168.0.5 port 22001 ssh2`,
			wantOK: true, ip: "192.168.0.5", user: "gonzxa", action: "accepted", success: true,
		},
		{
			line:   `Jul 31 17:35:01 host sshd[1234]: Invalid user root from 45.83.66.10 port 51000`,
			wantOK: true, ip: "45.83.66.10", user: "root", action: "invalid_user", success: false,
		},
		{
			line:   `Jul 31 17:35:01 host sshd[1234]: Connection closed by authenticating user root 45.83.66.10 port 51000 [preauth]`,
			wantOK: true, ip: "45.83.66.10", action: "closed", success: false,
		},
		{
			line:   `Jul 31 17:35:01 host sudo: gonzxa : TTY=pts/0 ; PWD=/home/gonzxa ; USER=root ; COMMAND=/bin/ls`,
			wantOK: false,
		},
		{ // OpenSSH ≥ 9.8 logs auth from sshd-session
			line:   `2026-10-19T10:00:02.1+00:00 host sshd-session[42]: Accepted publickey for gonzxa from 192.0.2.7 port 5050 ssh2: ED25519 SHA256:abc`,
			wantOK: true, ip: "192.0.2.7", user: "gonzxa", action: "accepted", success: true,
		},
		{
			line:   `Jul 31 17:35:01 host sshd[7]: Failed password for root from 2001:db8::1 port 22 ssh2`,
			wantOK: true, ip: "2001:db8::1", user: "root", action: "failed", success: false,
		},
		// --- spoofing via attacker-chosen usernames: the event must keep the real IP and never become "accepted"
		{ // username: "Accepted password for root from 6.6.6.6 port 22"
			line:   `Jul 31 17:35:01 host sshd[999]: Invalid user Accepted password for root from 6.6.6.6 port 22 from 203.0.113.9 port 41234`,
			wantOK: true, ip: "203.0.113.9", user: "Accepted password for root from 6.6.6.6 port 22", action: "invalid_user", success: false,
		},
		{ // same, with a fake sshd tag inside the username
			line:   `Jul 31 17:35:01 host sshd[999]: Invalid user sshd[1]: Accepted password for root from 6.6.6.6 port 22 from 203.0.113.9 port 41234`,
			wantOK: true, ip: "203.0.113.9", action: "invalid_user", success: false,
		},
		{ // username: "x from 10.0.0.5 port 1"
			line:   `Jul 31 17:35:01 host sshd[999]: Failed password for invalid user x from 10.0.0.5 port 1 from 203.0.113.9 port 41235 ssh2`,
			wantOK: true, ip: "203.0.113.9", user: "x from 10.0.0.5 port 1", action: "failed", success: false,
		},
		{
			line:   `Jul 31 17:35:01 host sshd[999]: Connection closed by invalid user x 10.0.0.5 port 1 203.0.113.9 port 4 [preauth]`,
			wantOK: true, ip: "203.0.113.9", action: "closed", success: false,
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

// TestSSHWatcherWaitsForCompleteLines: a poll landing mid-write must not
// parse (and lose) the half line; it's read whole on the next poll.
func TestSSHWatcherWaitsForCompleteLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "auth.log")
	os.WriteFile(logPath, nil, 0o644)

	w := NewSSHWatcher(logPath, dir)
	w.poll()

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("Jul 31 10:00:00 h sshd[1]: Failed password for root from 1.2.3.4 ")
	w.poll()
	if got := w.Recent(10); len(got) != 0 {
		t.Fatalf("parsed a partial line: %+v", got)
	}
	f.WriteString("port 22 ssh2\n")
	w.poll()
	if got := w.Recent(10); len(got) != 1 || got[0].IP != "1.2.3.4" {
		t.Fatalf("got %+v, want one event from 1.2.3.4", got)
	}
}
