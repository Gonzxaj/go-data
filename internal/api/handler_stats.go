package api

import (
	"go-data/internal/domain"
	"net/http"
	"strconv"
)

// Stats returns the latest snapshot.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	all := h.Mem.GetAll()
	if len(all) == 0 {
		writeJSON(w, domain.Metrics{})
		return
	}
	writeJSON(w, all[len(all)-1])
}

type historyPoint struct {
	Time  int64   `json:"time"` // unix seconds
	Value float64 `json:"value"`
}

type historyResponse struct {
	Metric string         `json:"metric"`
	Points []historyPoint `json:"points"`
}

// History serves sparkline data by bucket-averaging over the in-memory ring
// buffer (~300 samples / 5 minutes) — no Influx read path required.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	selector := r.URL.Query().Get("of")
	points, _ := strconv.Atoi(r.URL.Query().Get("points"))
	if points <= 0 {
		points = 60
	}

	all := h.Mem.GetAll()
	type sample struct {
		t float64
		v float64
	}
	var samples []sample
	for _, m := range all {
		if v, ok := metricValue(m, metric, selector); ok {
			samples = append(samples, sample{t: float64(m.Time.Unix()), v: v})
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
