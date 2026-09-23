package netmon

import (
	"net"
	"strings"
)

// internalIfacePrefixes names the virtual interfaces whose whole subnet is
// host-internal: container bridges and overlay/CNI networks. A peer on one
// of these (e.g. a container's 172.17.0.x) is never a real remote client —
// it's the host talking to itself through a veth. Physical/LAN interfaces
// are deliberately NOT here: a 192.168.x client on the LAN is a real peer.
var internalIfacePrefixes = []string{
	"docker", "br-", "veth", "cni", "flannel", "cali", "weave", "virbr", "lxc", "lxdbr", "podman", "kube",
}

// localNet is a snapshot of which addresses belong to this host: its own
// interface IPs (any interface) plus the full subnets of loopback and
// container-bridge interfaces. Built from net.Interfaces(), which reflects
// the same network namespace conntrack's netlink socket reads (the host's,
// under network_mode: host).
type localNet struct {
	own      map[string]bool
	internal []*net.IPNet
}

func discoverLocalNet() localNet {
	ln := localNet{own: map[string]bool{}}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ln
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		internal := iface.Flags&net.FlagLoopback != 0 || hasInternalPrefix(iface.Name)
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ln.own[ipnet.IP.String()] = true
			if internal {
				ln.internal = append(ln.internal, &net.IPNet{IP: ipnet.IP.Mask(ipnet.Mask), Mask: ipnet.Mask})
			}
		}
	}
	return ln
}

func hasInternalPrefix(name string) bool {
	for _, p := range internalIfacePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// isLocal reports whether ip is this host's own address or lives on a
// host-internal network (loopback, container bridge).
func (ln localNet) isLocal(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return true
	}
	if ln.own[ip.String()] {
		return true
	}
	for _, n := range ln.internal {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
