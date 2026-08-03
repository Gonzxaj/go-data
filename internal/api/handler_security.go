package api

import (
	"go-data/internal/domain"
	"net/http"
)

type sshEventsResponse struct {
	Events  []domain.SSHEvent `json:"events"`
	Enabled bool              `json:"enabled"`
}

func (h *Handler) SSHEvents(w http.ResponseWriter, r *http.Request) {
	if h.SSH == nil {
		writeJSON(w, sshEventsResponse{Events: []domain.SSHEvent{}, Enabled: false})
		return
	}
	writeJSON(w, sshEventsResponse{Events: h.SSH.Recent(200), Enabled: true})
}

type connectionEventsResponse struct {
	Events  []domain.ConnectionEvent `json:"events"`
	Enabled bool                     `json:"enabled"`
}

func (h *Handler) ConnectionEvents(w http.ResponseWriter, r *http.Request) {
	if h.Conn == nil {
		writeJSON(w, connectionEventsResponse{Events: []domain.ConnectionEvent{}, Enabled: false})
		return
	}
	writeJSON(w, connectionEventsResponse{Events: h.Conn.Recent(200), Enabled: true})
}
