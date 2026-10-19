package uptime

import (
	"testing"
	"time"
)

func TestRebootClosesSessionAtLastSeen(t *testing.T) {
	l := NewLog(t.TempDir())
	boot1 := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	l.Observe(boot1.Add(time.Minute), boot1)
	l.Observe(boot1.Add(2*time.Hour), boot1.Add(time.Second)) // jitter: same boot

	boot2 := boot1.Add(5 * time.Hour) // host was off from +2h to +5h
	l.Observe(boot2.Add(time.Minute), boot2)

	s := l.Sessions()
	if len(s) != 2 {
		t.Fatalf("got %d sessions, want 2", len(s))
	}
	if !s[1].End.Equal(boot1.Add(2 * time.Hour)) {
		t.Fatalf("previous session end = %v, want last sighting", s[1].End)
	}
	if !s[0].Ongoing() {
		t.Fatalf("current session should be ongoing")
	}

	// last_seen survives a restart of go-data itself
	l.Observe(boot2.Add(2*time.Minute), boot2)
	l.persist()
	if got := NewLog(l.path[:len(l.path)-len("/uptime_sessions.jsonl")]).Sessions()[0].LastSeen; got == nil {
		t.Fatalf("last_seen not persisted")
	}
}
