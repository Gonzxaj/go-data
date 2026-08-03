package collector

import "testing"

func TestContainerMemoryStatsCacheBytes(t *testing.T) {
	cases := []struct {
		name string
		m    containerMemoryStats
		want uint64
	}{
		{
			name: "cgroup v2 (inactive_file) — a torrent client with heavy disk I/O shouldn't look like it's using all its cache as real memory",
			m: containerMemoryStats{
				Usage: 8_900_000_000,
				Stats: struct {
					Cache             uint64 `json:"cache"`
					TotalInactiveFile uint64 `json:"total_inactive_file"`
					InactiveFile      uint64 `json:"inactive_file"`
				}{InactiveFile: 8_600_000_000},
			},
			want: 8_600_000_000,
		},
		{
			name: "cgroup v1 (total_inactive_file) takes priority over a stale cache field",
			m: containerMemoryStats{
				Usage: 1000,
				Stats: struct {
					Cache             uint64 `json:"cache"`
					TotalInactiveFile uint64 `json:"total_inactive_file"`
					InactiveFile      uint64 `json:"inactive_file"`
				}{TotalInactiveFile: 400, Cache: 999},
			},
			want: 400,
		},
		{
			name: "no cache stats available at all",
			m:    containerMemoryStats{Usage: 1000},
			want: 0,
		},
	}

	for _, c := range cases {
		if got := c.m.cacheBytes(); got != c.want {
			t.Errorf("%s: cacheBytes() = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestAttachLiveStatsMemoryMatchesDockerStatsFormula(t *testing.T) {
	// Reproduces the real qbittorrent case: docker stats reported ~287MiB
	// used, but the raw cgroup usage (inflated by disk-write page cache from
	// torrenting) was ~8.9GB — used_bytes must subtract the cache, not report
	// the raw usage.
	m := containerMemoryStats{
		Usage: 8_914_313_216,
		Limit: 16_511_410_176,
	}
	m.Stats.InactiveFile = 8_613_000_000 // page cache from heavy disk I/O

	got := m.Usage - m.cacheBytes()
	want := uint64(301_313_216) // ~287MiB, in the same ballpark as `docker stats`
	if got != want {
		t.Fatalf("used bytes = %d, want %d", got, want)
	}
}
