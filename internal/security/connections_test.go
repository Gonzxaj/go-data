package security

import (
	"os"
	"path/filepath"
	"testing"
)

const tcpHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

func writeFakeProc(t *testing.T, tcpBody string) string {
	t.Helper()
	dir := t.TempDir()
	netDir := filepath.Join(dir, "1", "net")
	if err := os.MkdirAll(netDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netDir, "tcp"), []byte(tcpHeader+tcpBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netDir, "tcp6"), []byte(tcpHeader), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestConnWatcherAutoDiscoversPublicListenersOnly verifies that in auto
// mode, a port LISTENing on 0.0.0.0 gets watched (and its established
// connections logged), while a port LISTENing only on 127.0.0.1 (an
// internal-only service, e.g. a local Postgres) is ignored even though it
// also has an established connection.
func TestConnWatcherAutoDiscoversPublicListenersOnly(t *testing.T) {
	body := "" +
		"   0: 00000000:63DD 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0\n" + // LISTEN 0.0.0.0:25565
		"   1: 0100007F:1538 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 100 0 0 10 0\n" + // LISTEN 127.0.0.1:5432
		"   2: 00000000:63DD 04030201:D431 01 00000000:00000000 00:00000000 00000000     0        0 12347 1 0000000000000000 100 0 0 10 0\n" + // ESTABLISHED :25565 from 1.2.3.4
		"   3: 0100007F:1538 0100007F:C350 01 00000000:00000000 00:00000000 00000000     0        0 12348 1 0000000000000000 100 0 0 10 0\n" // ESTABLISHED :5432 from 127.0.0.1 (loopback service, must be ignored)
	dir := writeFakeProc(t, body)

	w := NewConnWatcher(nil, true, dir, t.TempDir())
	w.poll()

	got := w.Recent(10)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].IP != "1.2.3.4" || got[0].LocalPort != 25565 {
		t.Fatalf("event = %+v, want ip=1.2.3.4 port=25565", got[0])
	}

	// a second poll of the same still-established connection must not duplicate it
	w.poll()
	if got := w.Recent(10); len(got) != 1 {
		t.Fatalf("after no-op poll: got %d events, want 1", len(got))
	}
}

// TestConnWatcherExplicitPortsWithoutAuto verifies that with auto disabled,
// only explicitly configured ports are watched — even a public LISTENer on
// another port is ignored.
func TestConnWatcherExplicitPortsWithoutAuto(t *testing.T) {
	body := "" +
		"   0: 00000000:63DD 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0\n" + // LISTEN 0.0.0.0:25565
		"   1: 00000000:63DD 04030201:D431 01 00000000:00000000 00:00000000 00000000     0        0 12347 1 0000000000000000 100 0 0 10 0\n" // ESTABLISHED :25565 from 1.2.3.4
	dir := writeFakeProc(t, body)

	w := NewConnWatcher([]int{7777}, false, dir, t.TempDir())
	w.poll()

	if got := w.Recent(10); len(got) != 0 {
		t.Fatalf("got %d events, want 0 (port 25565 wasn't explicitly configured): %+v", len(got), got)
	}
}
