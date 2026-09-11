// Package procnet parses Linux's /proc/net/{tcp,tcp6,udp,udp6} socket
// tables. It's stateless, pure parsing — no policy (which ports matter,
// what counts as "watched") lives here, so it can be shared by anything
// that needs a raw view of the kernel's socket tables.
package procnet

import (
	"bufio"
	"encoding/hex"
	"net"
	"os"
	"strconv"
	"strings"
)

// TCP connection states, as encoded in /proc/net/tcp's "st" column.
const (
	TCPEstablished = "01"
	TCPListen      = "0A"
)

// Entry is one row of /proc/net/{tcp,tcp6,udp,udp6}.
type Entry struct {
	LocalIP    net.IP
	LocalPort  int
	RemoteIP   net.IP
	RemotePort int
	State      string // uppercase hex state code; meaningless for udp rows
}

// ReadEntries parses every row of the given /proc/net/{tcp,tcp6,udp,udp6}
// file. Malformed rows are silently skipped (this file is kernel-generated
// and well-formed in practice; skipping is just defensive).
func ReadEntries(path string) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []Entry
	scanner := bufio.NewScanner(f)
	scanner.Scan() // header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		localParts := strings.SplitN(fields[1], ":", 2)
		remoteParts := strings.SplitN(fields[2], ":", 2)
		if len(localParts) != 2 || len(remoteParts) != 2 {
			continue
		}
		localIP := DecodeHexAddr(localParts[0])
		localPort := DecodeHexPort(localParts[1])
		remoteIP := DecodeHexAddr(remoteParts[0])
		remotePort := DecodeHexPort(remoteParts[1])
		if localIP == nil || localPort == 0 || remoteIP == nil {
			continue
		}
		out = append(out, Entry{
			LocalIP: localIP, LocalPort: localPort,
			RemoteIP: remoteIP, RemotePort: remotePort,
			State: strings.ToUpper(fields[3]),
		})
	}
	return out
}

func DecodeHexPort(h string) int {
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0
	}
	return int(v)
}

// DecodeHexAddr decodes /proc/net/{tcp,udp}{,6}'s address encoding: each
// 4-byte group is stored little-endian, so within every 8-hex-char chunk
// the byte order must be reversed (IPv6 has 4 such chunks concatenated).
func DecodeHexAddr(h string) net.IP {
	if len(h) != 8 && len(h) != 32 {
		return nil
	}
	raw := make([]byte, 0, 16)
	for i := 0; i < len(h); i += 8 {
		chunk, err := hex.DecodeString(h[i : i+8])
		if err != nil || len(chunk) != 4 {
			return nil
		}
		raw = append(raw, chunk[3], chunk[2], chunk[1], chunk[0])
	}
	if len(raw) == 4 {
		return net.IPv4(raw[0], raw[1], raw[2], raw[3])
	}
	return net.IP(raw)
}
