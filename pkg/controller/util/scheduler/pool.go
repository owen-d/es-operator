package scheduler

import (
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
)

type PoolStats struct {
	Name    string
	Ready   int32
	Desired int32
	// dangling denotes that a pool instance exists, but is no longer part of the
	// cluster spec
	Dangling bool
}

// ToStats takes a list of poolspecs and a list of metrics mapping pools -> number of ready replicas.
// This combines them into a list of PoolStats types acceptable to the scheduler.
func ToStats(specs []elasticsearchv1beta1.PoolSpec, metricsList map[string]elasticsearchv1beta1.PoolSetMetrics) (res []PoolStats) {
	poolMap := make(map[string]PoolStats)

	for _, spec := range specs {
		poolMap[spec.Name] = PoolStats{
			Name:     spec.Name,
			Desired:  spec.Replicas,
			Dangling: true,
		}
	}

	for name, metrics := range metricsList {
		if stats, ok := poolMap[name]; !ok {
			// exists in status, but not in spec. This pool is a dangling reference
			poolMap[name] = PoolStats{
				Name:     name,
				Ready:    metrics.Ready,
				Desired:  0,
				Dangling: false,
			}
		} else {
			stats.Ready = metrics.Ready
		}
	}

	for _, stats := range poolMap {
		res = append(res, stats)
	}
	return res
}

func PoolsForScheduling(
	numReplicas int32,
	poolStats []PoolStats,
) {
}
