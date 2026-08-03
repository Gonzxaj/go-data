package collector

import (
	"fmt"
	"go-data/internal/domain"
	"sort"
)

// dedupePorts removes the duplicate host/port entries Docker reports once per
// IP family (0.0.0.0 and ::) for the same published mapping.
func dedupePorts(ports []domain.Port) []domain.Port {
	type key struct {
		hostPort, containerPort int
		proto                   string
	}
	seen := map[key]bool{}
	var out []domain.Port
	for _, p := range ports {
		k := key{p.HostPort, p.ContainerPort, p.Proto}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ContainerPort != out[j].ContainerPort {
			return out[i].ContainerPort < out[j].ContainerPort
		}
		return out[i].Proto < out[j].Proto
	})
	return out
}

// buildPortChips collapses runs of >=3 consecutive sequential ports (same
// proto, same host/container offset) into a single range chip, mirroring
// what the frontend used to do in JS against raw Docker port arrays.
func buildPortChips(ports []domain.Port) []domain.PortChip {
	uniq := dedupePorts(ports)
	var chips []domain.PortChip

	offsetOf := func(p domain.Port) (int, bool) {
		if p.HostPort == 0 {
			return 0, false
		}
		return p.HostPort - p.ContainerPort, true
	}

	i := 0
	for i < len(uniq) {
		j := i
		offset, hasOffset := offsetOf(uniq[i])
		for j+1 < len(uniq) {
			next := uniq[j+1]
			nextOffset, nextHasOffset := offsetOf(next)
			if next.Proto != uniq[i].Proto || next.ContainerPort != uniq[j].ContainerPort+1 || nextHasOffset != hasOffset {
				break
			}
			if hasOffset && nextOffset != offset {
				break
			}
			j++
		}
		runLen := j - i + 1

		if runLen >= 3 {
			a, b := uniq[i], uniq[j]
			if hasOffset {
				chips = append(chips, domain.PortChip{
					Label:     fmt.Sprintf("%d-%d→%d-%d/%s", a.HostPort, b.HostPort, a.ContainerPort, b.ContainerPort, a.Proto),
					Published: true,
				})
			} else {
				chips = append(chips, domain.PortChip{
					Label:     fmt.Sprintf("%d-%d/%s", a.ContainerPort, b.ContainerPort, a.Proto),
					Published: false,
				})
			}
		} else {
			for k := i; k <= j; k++ {
				p := uniq[k]
				if p.Published {
					chips = append(chips, domain.PortChip{
						Label:     fmt.Sprintf("%d→%d/%s", p.HostPort, p.ContainerPort, p.Proto),
						Published: true,
					})
				} else {
					chips = append(chips, domain.PortChip{
						Label:     fmt.Sprintf("%d/%s", p.ContainerPort, p.Proto),
						Published: false,
					})
				}
			}
		}
		i = j + 1
	}
	return chips
}
