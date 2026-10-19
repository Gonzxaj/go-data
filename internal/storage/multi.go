package storage

import (
	"errors"
	"go-data/internal/domain"
)

type Storage interface {
	Save(domain.Metrics) error
}

type MultiStore struct {
	stores []Storage
}

func NewMulti(stores ...Storage) *MultiStore {
	return &MultiStore{stores: stores}
}

// Save runs synchronously so samples land in order; every store is
// non-blocking (memory, Influx's async writer).
func (m *MultiStore) Save(p domain.Metrics) error {
	var errs []error
	for _, s := range m.stores {
		if err := s.Save(p); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
