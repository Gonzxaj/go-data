package domain

import "time"

// NetFlow is one currently-observed connection/flow. The full set is
// rebuilt every poll — it's a live snapshot row, never persisted on its
// own (see NetFlowEvent for the persisted, edge-triggered counterpart).
type NetFlow struct {
	RemoteIP   string `json:"remote_ip"`
	RemotePort int    `json:"remote_port"`
	LocalPort  int    `json:"local_port"`
	Proto      string `json:"proto"`     // tcp | udp
	Direction  string `json:"direction"` // inbound | outbound
	State      string `json:"state"`     // tcp: established/time_wait/...; udp: assured/active
}

// NetIPHistory tracks first/last-seen and total distinct connections for a
// remote IP, across all ports and protocols. Upserted in place, not
// appended. Connections counts new flows (edge-triggered, like
// NetFlowEvent), not polls a long-lived flow stayed open for.
type NetIPHistory struct {
	IP          string    `json:"ip"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Connections int64     `json:"connections"`
}

// NetFlowEvent marks the first observation of a new flow (edge-triggered —
// logged once per flow, not on every poll it stays open).
type NetFlowEvent struct {
	Time       time.Time `json:"time"`
	RemoteIP   string    `json:"remote_ip"`
	RemotePort int       `json:"remote_port"`
	LocalPort  int       `json:"local_port"`
	Proto      string    `json:"proto"`
	Direction  string    `json:"direction"`
}

// NetSample is one aggregate snapshot per poll, buffered for
// /api/network/history — netmon's own analogue of domain.Metrics for
// /api/history, deliberately kept separate from it.
type NetSample struct {
	Time        time.Time `json:"time"`
	ActiveTotal int       `json:"active_total"`
	ActiveTCP   int       `json:"active_tcp"`
	ActiveUDP   int       `json:"active_udp"`
	UniqueIPs   int       `json:"unique_ips"`
	Inbound     int       `json:"inbound"`
	Outbound    int       `json:"outbound"`
}

// NetCount is a generic (key, count) pair for top-IPs / top-ports views.
type NetCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}
