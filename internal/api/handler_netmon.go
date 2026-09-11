package api

import (
	"go-data/internal/domain"
	"net/http"
	"strconv"
)

type netSummaryResponse struct {
	Enabled     bool   `json:"enabled"`
	Capability  string `json:"capability,omitempty"`
	HostWide    bool   `json:"host_wide,omitempty"`
	ActiveTotal int    `json:"active_total"`
	ActiveTCP   int    `json:"active_tcp"`
	ActiveUDP   int    `json:"active_udp"`
	UniqueIPs   int    `json:"unique_ips"`
	Inbound     int    `json:"inbound"`
	Outbound    int    `json:"outbound"`
}

func (h *Handler) NetSummary(w http.ResponseWriter, r *http.Request) {
	if h.NetMon == nil {
		writeJSON(w, netSummaryResponse{Enabled: false})
		return
	}
	flows := h.NetMon.Snapshot()
	resp := netSummaryResponse{
		Enabled:    true,
		Capability: h.NetMon.Capability(),
		HostWide:   h.NetMon.HostWide(),
	}
	ips := map[string]bool{}
	for _, f := range flows {
		resp.ActiveTotal++
		ips[f.RemoteIP] = true
		if f.Proto == "tcp" {
			resp.ActiveTCP++
		} else {
			resp.ActiveUDP++
		}
		if f.Direction == "inbound" {
			resp.Inbound++
		} else {
			resp.Outbound++
		}
	}
	resp.UniqueIPs = len(ips)
	writeJSON(w, resp)
}

type netConnectionsResponse struct {
	Enabled     bool             `json:"enabled"`
	Capability  string           `json:"capability,omitempty"`
	Connections []domain.NetFlow `json:"connections"`
}

func (h *Handler) NetConnections(w http.ResponseWriter, r *http.Request) {
	if h.NetMon == nil {
		writeJSON(w, netConnectionsResponse{Enabled: false, Connections: []domain.NetFlow{}})
		return
	}
	writeJSON(w, netConnectionsResponse{Enabled: true, Capability: h.NetMon.Capability(), Connections: h.NetMon.Snapshot()})
}

type netTopResponse struct {
	Enabled bool              `json:"enabled"`
	By      string            `json:"by"`
	Items   []domain.NetCount `json:"items"`
}

func (h *Handler) NetTop(w http.ResponseWriter, r *http.Request) {
	by := r.URL.Query().Get("by")
	if by != "port" {
		by = "ip"
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 10
	}
	if h.NetMon == nil {
		writeJSON(w, netTopResponse{Enabled: false, By: by, Items: []domain.NetCount{}})
		return
	}
	writeJSON(w, netTopResponse{Enabled: true, By: by, Items: h.NetMon.Top(by, n)})
}

type netEventsResponse struct {
	Enabled bool                  `json:"enabled"`
	Events  []domain.NetFlowEvent `json:"events"`
}

func (h *Handler) NetEvents(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 200
	}
	if h.NetMon == nil {
		writeJSON(w, netEventsResponse{Enabled: false, Events: []domain.NetFlowEvent{}})
		return
	}
	writeJSON(w, netEventsResponse{Enabled: true, Events: h.NetMon.RecentEvents(n)})
}

type netIPHistoryResponse struct {
	Enabled bool                  `json:"enabled"`
	Entries []domain.NetIPHistory `json:"entries"`
}

func (h *Handler) NetIPHistory(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 200
	}
	if h.NetMon == nil {
		writeJSON(w, netIPHistoryResponse{Enabled: false, Entries: []domain.NetIPHistory{}})
		return
	}
	writeJSON(w, netIPHistoryResponse{Enabled: true, Entries: h.NetMon.IPHistory(n)})
}

// netMetricValue is netmon's own small closed enum, kept separate from
// metric_lookup.go's (which is intentionally hard-wired to domain.Metrics).
func netMetricValue(s domain.NetSample, metric string) (float64, bool) {
	switch metric {
	case "active_total":
		return float64(s.ActiveTotal), true
	case "active_tcp":
		return float64(s.ActiveTCP), true
	case "active_udp":
		return float64(s.ActiveUDP), true
	case "unique_ips":
		return float64(s.UniqueIPs), true
	case "inbound":
		return float64(s.Inbound), true
	case "outbound":
		return float64(s.Outbound), true
	default:
		return 0, false
	}
}

// NetHistory serves sparkline data for netmon's own metrics, bucket-averaged
// the same way handler_stats.go's History() does for /api/history.
func (h *Handler) NetHistory(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	points, _ := strconv.Atoi(r.URL.Query().Get("points"))
	if points <= 0 {
		points = 60
	}

	if h.NetMon == nil {
		writeJSON(w, historyResponse{Metric: metric, Points: []historyPoint{}})
		return
	}

	all := h.NetMon.SampleHistory()
	type sample struct{ t, v float64 }
	var samples []sample
	for _, s := range all {
		if v, ok := netMetricValue(s, metric); ok {
			samples = append(samples, sample{t: float64(s.Time.Unix()), v: v})
		}
	}

	var out []historyPoint
	if len(samples) > 0 {
		bucketSize := len(samples) / points
		if bucketSize < 1 {
			bucketSize = 1
		}
		for i := 0; i < len(samples); i += bucketSize {
			end := i + bucketSize
			if end > len(samples) {
				end = len(samples)
			}
			var sumV, sumT float64
			for _, s := range samples[i:end] {
				sumV += s.v
				sumT += s.t
			}
			n := float64(end - i)
			out = append(out, historyPoint{Time: int64(sumT / n), Value: sumV / n})
		}
	}

	writeJSON(w, historyResponse{Metric: metric, Points: out})
}
