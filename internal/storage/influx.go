package storage

import (
	"context"
	"go-data/internal/domain"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

type InfluxStore struct {
	write api.WriteAPIBlocking
}

func NewInfluxStore(url, token, org, bucket string) *InfluxStore {
	client := influxdb2.NewClient(url, token)
	return &InfluxStore{
		write: client.WriteAPIBlocking(org, bucket),
	}
}

// WritePoint writes an arbitrary point through the same already-configured
// client — for callers (like internal/netmon) whose data doesn't fit
// domain.Metrics's fixed shape, so they don't need a second Influx
// connection of their own.
func (i *InfluxStore) WritePoint(measurement string, tags map[string]string, fields map[string]interface{}, t time.Time) error {
	return i.write.WritePoint(context.Background(), influxdb2.NewPoint(measurement, tags, fields, t))
}

func (i *InfluxStore) Save(p domain.Metrics) error {
	var diskUsedPct float64
	for _, d := range p.Disks {
		if d.Mount == "/" {
			diskUsedPct = d.UsedPct
			break
		}
	}
	point := influxdb2.NewPoint(
		"system",
		nil,
		map[string]interface{}{
			"cpu_usage_pct": p.CPU.UsagePct,
			"ram_used_pct":  p.RAM.UsedPct,
			"load1":         p.Load.Load1,
			"disk_used_pct": diskUsedPct,
		},
		p.Time,
	)
	return i.write.WritePoint(context.Background(), point)
}
