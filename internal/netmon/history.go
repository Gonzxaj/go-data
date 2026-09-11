package netmon

import (
	"go-data/internal/domain"
	"sync"
)

// sampleHistory is a small capped in-memory ring buffer of NetSample,
// mirroring storage.MemoryStore's shape but kept separate — /api/history
// is intentionally closed to domain.Metrics, and netmon's samples run on
// their own cadence, not the 1Hz tick.
type sampleHistory struct {
	mu   sync.RWMutex
	cap  int
	data []domain.NetSample
}

func newSampleHistory(cap int) *sampleHistory {
	return &sampleHistory{cap: cap}
}

func (h *sampleHistory) Append(s domain.NetSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = append(h.data, s)
	if len(h.data) > h.cap {
		h.data = h.data[len(h.data)-h.cap:]
	}
}

func (h *sampleHistory) GetAll() []domain.NetSample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]domain.NetSample, len(h.data))
	copy(out, h.data)
	return out
}
