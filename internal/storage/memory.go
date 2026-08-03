package storage

import (
	"go-data/internal/domain"
	"sync"
)

type MemoryStore struct {
	data []domain.Metrics
	max  int
	mu   sync.RWMutex
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{max: 300}
}

func (m *MemoryStore) Save(p domain.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.data) >= m.max {
		m.data = m.data[1:]
	}
	m.data = append(m.data, p)
	return nil
}

func (m *MemoryStore) GetAll() []domain.Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data
}
