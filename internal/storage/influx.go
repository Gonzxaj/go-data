package storage

import (
	"go-data/internal/domain"
	"log"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

type InfluxStore struct {
	write api.WriteAPI
}

// NewInfluxStore uses the client's async, batching writer, so a slow or
// down Influx never stalls the collector tick (its retry buffer is bounded),
// and write failures are logged instead of silently dropped.
func NewInfluxStore(url, token, org, bucket string) *InfluxStore {
	client := influxdb2.NewClient(url, token)
	w := client.WriteAPI(org, bucket)
	go func() {
		for err := range w.Errors() {
			log.Printf("influx write: %v", err)
		}
	}()
	return &InfluxStore{write: w}
}

// WritePoint writes an arbitrary point through the same already-configured
// client — for callers (like internal/netmon) whose data doesn't fit
// domain.Metrics's fixed shape, so they don't need a second Influx
// connection of their own. Errors surface asynchronously in the log.
func (i *InfluxStore) WritePoint(measurement string, tags map[string]string, fields map[string]interface{}, t time.Time) error {
	i.write.WritePoint(influxdb2.NewPoint(measurement, tags, fields, t))
	return nil
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
	i.write.WritePoint(point)
	return nil
}
