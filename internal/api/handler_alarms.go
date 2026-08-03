package api

import (
	"go-data/internal/alarm"
	"net/http"
)

type alarmsResponse struct {
	Alarms []alarm.Alarm `json:"alarms"`
	Count  int           `json:"count"`
}

func (h *Handler) AlarmsList(w http.ResponseWriter, r *http.Request) {
	alarms := h.Alarms.Latest()
	writeJSON(w, alarmsResponse{Alarms: alarms, Count: len(alarms)})
}
