package collector

import (
	"go-data/internal/domain"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// readSockets parses /proc/net/sockstat, e.g.:
//   sockets: used 1234
//   TCP: inuse 12 orphan 0 tw 34 alloc 56 mem 78
//   UDP: inuse 5 mem 6
//
// /proc/net/* is scoped per network namespace, so inside a container this
// would only ever show the container's own (near-empty) socket table unless
// read through PID 1's namespace — the host's real one when procPath is a
// mounted host /proc (see tcpFiles in connections.go for the same trick).
func readSockets(procPath string) domain.Sockets {
	data, err := os.ReadFile(filepath.Join(procPath, "1", "net", "sockstat"))
	if err != nil {
		return domain.Sockets{}
	}
	var s domain.Sockets
	for _, l := range strings.Split(string(data), "\n") {
		fields := strings.Fields(l)
		if len(fields) < 2 {
			continue
		}
		kv := map[string]int{}
		for i := 1; i+1 < len(fields); i += 2 {
			v, _ := strconv.Atoi(fields[i+1])
			kv[fields[i]] = v
		}
		switch fields[0] {
		case "sockets:":
			s.TotalUsed = kv["used"]
		case "TCP:":
			s.TCPInUse = kv["inuse"]
			s.TCPOrphan = kv["orphan"]
			s.TCPTimeWait = kv["tw"]
		case "UDP:":
			s.UDPInUse = kv["inuse"]
		}
	}
	return s
}
