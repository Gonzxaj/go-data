package collector

import (
	"go-data/internal/domain"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type thermalZone struct {
	path         string // e.g. /sys/class/thermal/thermal_zone0
	label        string
	isPrimaryCPU bool
}

// resolveThermalZones discovers thermal zones and classifies them once; the
// set of zones and their labels don't change for the life of the process.
func resolveThermalZones() []thermalZone {
	paths, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	var zones []thermalZone
	for _, p := range paths {
		typeData, err := os.ReadFile(filepath.Join(p, "type"))
		if err != nil {
			continue
		}
		label := strings.TrimSpace(string(typeData))
		zones = append(zones, thermalZone{path: p, label: label})
	}

	lower := func(s string) string { return strings.ToLower(s) }
	pickIdx := -1
	for i, z := range zones {
		if strings.Contains(lower(z.label), "pkg") {
			pickIdx = i
			break
		}
	}
	if pickIdx < 0 {
		for i, z := range zones {
			if strings.Contains(lower(z.label), "core") {
				pickIdx = i
				break
			}
		}
	}
	if pickIdx < 0 {
		for i, z := range zones {
			if lower(z.label) == "acpitz" {
				pickIdx = i
				break
			}
		}
	}
	if pickIdx < 0 && len(zones) > 0 {
		pickIdx = 0 // best-effort fallback: don't leave temperature entirely absent
	}
	if pickIdx >= 0 {
		zones[pickIdx].isPrimaryCPU = true
	}
	return zones
}

func readTemps(zones []thermalZone) []domain.Temp {
	var out []domain.Temp
	for _, z := range zones {
		data, err := os.ReadFile(filepath.Join(z.path, "temp"))
		if err != nil {
			continue
		}
		milliC, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, domain.Temp{
			Zone: filepath.Base(z.path), Label: z.label,
			Celsius: float64(milliC) / 1000, IsPrimaryCPU: z.isPrimaryCPU,
		})
	}
	return out
}
