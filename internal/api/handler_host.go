package api

import "net/http"

func (h *Handler) HostInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.Host)
}
