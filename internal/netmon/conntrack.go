package netmon

import (
	"syscall"

	conntrack "github.com/florianl/go-conntrack"
)

// tcpStateNames maps the raw nf_conntrack_tcp state byte to a human name
// (kernel's enum tcp_conntrack, include/uapi/linux/netfilter/nf_conntrack_tcp.h).
var tcpStateNames = map[uint8]string{
	0: "none",
	1: "syn_sent",
	2: "syn_recv",
	3: "established",
	4: "fin_wait",
	5: "close_wait",
	6: "last_ack",
	7: "time_wait",
	8: "close",
	9: "listen",
}

// ipsAssuredBit is bit 2 of nf_conntrack's Status bitmask (IPS_ASSURED),
// include/uapi/linux/netfilter/nf_conntrack_common.h — set once a
// connectionless (UDP) flow has seen traffic in both directions, the
// closest UDP analogue to "established".
const ipsAssuredBit = 1 << 2

type conntrackSource struct {
	nfct *conntrack.Nfct
}

func openConntrack() (*conntrackSource, error) {
	nfct, err := conntrack.Open(&conntrack.Config{})
	if err != nil {
		return nil, err
	}
	return &conntrackSource{nfct: nfct}, nil
}

func (s *conntrackSource) Close() {
	s.nfct.Close()
}

// dump returns every currently-tracked TCP/UDP flow as a rawFlow — just the
// two ends of the flow as the kernel first saw it (the "original" tuple,
// the packet that created the entry; the post-DNAT "reply" tuple is
// ignored). Which end is "local" vs "remote" is resolved uniformly by the
// caller (resolveDirection), the same way for this source and the
// proc/net/tcp fallback alike.
func (s *conntrackSource) dump() ([]rawFlow, error) {
	var flows []rawFlow
	for _, fam := range []conntrack.Family{conntrack.IPv4, conntrack.IPv6} {
		sessions, err := s.nfct.Dump(conntrack.Conntrack, fam)
		if err != nil {
			return flows, err
		}
		for _, c := range sessions {
			f, ok := rawFlowFromCon(c)
			if ok {
				flows = append(flows, f)
			}
		}
	}
	return flows, nil
}

func rawFlowFromCon(c conntrack.Con) (rawFlow, bool) {
	if c.Origin == nil || c.Origin.Src == nil || c.Origin.Dst == nil || c.Origin.Proto == nil {
		return rawFlow{}, false
	}
	proto := c.Origin.Proto
	if proto.Number == nil || proto.SrcPort == nil || proto.DstPort == nil {
		return rawFlow{}, false
	}

	var protoName, state string
	switch *proto.Number {
	case syscall.IPPROTO_TCP:
		protoName = "tcp"
		state = "established"
		if c.ProtoInfo != nil && c.ProtoInfo.TCP != nil && c.ProtoInfo.TCP.State != nil {
			if name, ok := tcpStateNames[*c.ProtoInfo.TCP.State]; ok {
				state = name
			}
		}
	case syscall.IPPROTO_UDP:
		protoName = "udp"
		state = "active"
		if c.Status != nil && *c.Status&ipsAssuredBit != 0 {
			state = "assured"
		}
	default:
		return rawFlow{}, false // icmp, sctp, etc. — out of scope
	}

	return rawFlow{
		SrcIP: *c.Origin.Src, SrcPort: int(*proto.SrcPort),
		DstIP: *c.Origin.Dst, DstPort: int(*proto.DstPort),
		Proto: protoName, State: state,
	}, true
}
