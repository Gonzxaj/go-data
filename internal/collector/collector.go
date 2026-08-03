package collector

import "go-data/internal/domain"

// Collector reads the current system state into a snapshot.
type Collector interface {
	Collect() domain.Metrics
}
