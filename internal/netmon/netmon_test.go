package netmon

import (
	"net"
	"testing"
	"time"
)

func testLocalNet() localNet {
	_, bridge, _ := net.ParseCIDR("172.17.0.0/16")
	return localNet{
		own:      map[string]bool{"192.168.1.10": true, "172.17.0.1": true},
		internal: []*net.IPNet{bridge},
	}
}

func flow(src string, sport int, dst string, dport int, proto string) rawFlow {
	return rawFlow{SrcIP: net.ParseIP(src), SrcPort: sport, DstIP: net.ParseIP(dst), DstPort: dport, Proto: proto}
}

func TestResolveDirection(t *testing.T) {
	bound := map[int]bool{443: true, 25565: true}
	raw := []rawFlow{
		flow("181.1.2.3", 50000, "192.168.1.10", 25565, "tcp"), // external client → us
		flow("192.168.1.10", 40000, "8.8.8.8", 443, "tcp"),     // us → remote :443 (443 is also bound locally)
		flow("172.17.0.5", 41000, "1.1.1.1", 53, "udp"),        // container → remote via NAT
		flow("127.0.0.1", 42000, "127.0.0.1", 19246, "tcp"),    // loopback: dropped
		flow("172.17.0.1", 43000, "172.17.0.5", 8080, "tcp"),   // host → container: dropped
	}
	got := resolveDirection(raw, testLocalNet(), bound, map[int]bool{})
	if len(got) != 3 {
		t.Fatalf("got %d flows, want 3: %+v", len(got), got)
	}
	want := []struct {
		ip        string
		local     int
		direction string
	}{
		{"181.1.2.3", 25565, "inbound"},
		{"8.8.8.8", 40000, "outbound"},
		{"1.1.1.1", 41000, "outbound"},
	}
	for i, w := range want {
		if got[i].RemoteIP != w.ip || got[i].LocalPort != w.local || got[i].Direction != w.direction {
			t.Errorf("flow %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestResolveDirectionFallsBackToPorts(t *testing.T) {
	// Neither end recognised as local → bound-port heuristic.
	raw := []rawFlow{
		flow("203.0.113.9", 51000, "198.51.100.1", 25565, "tcp"),
		flow("198.51.100.1", 25565, "203.0.113.9", 51000, "tcp"),
	}
	got := resolveDirection(raw, localNet{}, map[int]bool{25565: true}, nil)
	if got[0].Direction != "inbound" || got[0].RemoteIP != "203.0.113.9" {
		t.Errorf("flow 0 = %+v", got[0])
	}
	if got[1].Direction != "outbound" || got[1].RemoteIP != "203.0.113.9" {
		t.Errorf("flow 1 = %+v", got[1])
	}
}

func TestIPStoreCountsConnectionsNotPolls(t *testing.T) {
	s := newIPHistoryStore(t.TempDir(), 100)
	defer s.Close()
	now := time.Now()
	s.Observe("181.1.2.3", now, true)
	for i := 1; i <= 5; i++ { // same flow still open on later polls
		s.Observe("181.1.2.3", now.Add(time.Duration(i)*10*time.Second), false)
	}
	s.Observe("181.1.2.3", now.Add(time.Minute), true) // a second connection
	e := s.Recent(1)[0]
	if e.Connections != 2 {
		t.Fatalf("connections = %d, want 2", e.Connections)
	}
	if !e.LastSeen.Equal(now.Add(time.Minute)) || !e.FirstSeen.Equal(now) {
		t.Fatalf("first/last seen = %v / %v", e.FirstSeen, e.LastSeen)
	}
}

func TestIPStoreEvictsOldestAndPersistsOnClose(t *testing.T) {
	dir := t.TempDir()
	s := newIPHistoryStore(dir, 10)
	base := time.Now()
	for i := 0; i < 11; i++ {
		s.Observe(net.IPv4(10, 0, 0, byte(i)).String(), base.Add(time.Duration(i)*time.Second), true)
	}
	if n := len(s.Recent(0)); n != 9 {
		t.Fatalf("after eviction have %d entries, want 9", n)
	}
	if s.Recent(0)[0].IP != "10.0.0.10" {
		t.Fatalf("newest IP was evicted")
	}
	s.Close()

	reloaded := newIPHistoryStore(dir, 10)
	defer reloaded.Close()
	if n := len(reloaded.Recent(0)); n != 9 {
		t.Fatalf("reloaded %d entries, want 9", n)
	}
}

func TestIPStorePrune(t *testing.T) {
	s := newIPHistoryStore(t.TempDir(), 100)
	defer s.Close()
	s.Observe("127.0.0.1", time.Now(), true)
	s.Observe("172.17.0.5", time.Now(), true)
	s.Observe("181.1.2.3", time.Now(), true)
	s.Prune(testLocalNet().isLocal)
	got := s.Recent(0)
	if len(got) != 1 || got[0].IP != "181.1.2.3" {
		t.Fatalf("after prune = %+v", got)
	}
}

func TestInboundOnly(t *testing.T) {
	raw := []rawFlow{
		flow("181.1.2.3", 50000, "192.168.1.10", 25565, "tcp"),
		flow("192.168.1.10", 40000, "198.41.200.13", 7844, "udp"), // cloudflared → Cloudflare edge
	}
	got := inboundOnly(resolveDirection(raw, testLocalNet(), nil, nil))
	if len(got) != 1 || got[0].RemoteIP != "181.1.2.3" {
		t.Fatalf("inboundOnly = %+v", got)
	}
}
