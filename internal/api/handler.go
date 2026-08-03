package api

import (
	"encoding/json"
	"go-data/internal/alarm"
	"go-data/internal/collector"
	"go-data/internal/domain"
	"go-data/internal/security"
	"go-data/internal/storage"
	"go-data/internal/uptime"
	"net/http"
)

type Handler struct {
	Mem    *storage.MemoryStore
	Host   domain.HostInfo
	Docker *collector.DockerCollector // nil if Docker is unavailable/disabled
	Alarms *alarm.Evaluator
	Uptime *uptime.Log
	SSH    *security.SSHWatcher  // nil if disabled/log not found
	Conn   *security.ConnWatcher // nil if disabled
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/stats", h.Stats)
	mux.HandleFunc("/api/history", h.History)
	mux.HandleFunc("/api/host", h.HostInfo)
	mux.HandleFunc("/api/containers", h.Containers)
	mux.HandleFunc("/api/alarms", h.AlarmsList)
	mux.HandleFunc("/api/uptime/sessions", h.UptimeSessions)
	mux.HandleFunc("/api/security/ssh", h.SSHEvents)
	mux.HandleFunc("/api/security/connections", h.ConnectionEvents)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
