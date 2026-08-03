package api

import (
	"go-data/internal/domain"
	"net/http"
)

type containersResponse struct {
	Containers []domain.Container `json:"containers"`
}

func (h *Handler) Containers(w http.ResponseWriter, r *http.Request) {
	containers := []domain.Container{}
	if h.Docker != nil {
		if snap := h.Docker.Snapshot(); snap != nil {
			containers = snap
		}
	}
	writeJSON(w, containersResponse{Containers: containers})
}
