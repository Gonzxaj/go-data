package api

import (
	"net/http"
	"time"
)

type sessionResponse struct {
	Start           time.Time  `json:"start"`
	End             *time.Time `json:"end"`
	DurationSeconds float64    `json:"duration_seconds"`
	Ongoing         bool       `json:"ongoing"`
}

type uptimeSessionsResponse struct {
	Sessions []sessionResponse `json:"sessions"`
}

func (h *Handler) UptimeSessions(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	sessions := h.Uptime.Sessions()
	out := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionResponse{
			Start: s.Start, End: s.End,
			DurationSeconds: s.Duration(now).Seconds(),
			Ongoing:         s.Ongoing(),
		})
	}
	writeJSON(w, uptimeSessionsResponse{Sessions: out})
}
