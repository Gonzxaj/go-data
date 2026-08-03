package storage

import "go-data/internal/domain"

type Storage interface {
	Save(domain.Metrics) error
}

type MultiStore struct {
	stores []Storage
}

func NewMulti(stores ...Storage) *MultiStore {
	return &MultiStore{stores: stores}
}

func (m *MultiStore) Save(p domain.Metrics) error {
	for _, s := range m.stores {
		go s.Save(p)
	}
	return nil
}
