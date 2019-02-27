package scheduler

import (
	"fmt"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	"sort"
)

type PoolStats struct {
	Name    string
	Ready   int32
	Desired int32
	// dangling denotes that a pool instance exists, but is no longer part of the
	// cluster spec
	Dangling bool
	// ScheduleReplicas denotes how many replicas should be scheduled for a node pool
	ScheduleReplicas int32
	// LastUnschedulable denotes that the last indexed node in a pool should not be schedulable for shards.
	// This is primarily used to facilitate scale-downs which should have shards drained at the application
	// level before being removed by k8s.
	LastUnschedulable bool
}

// ToStats takes a list of poolspecs and a list of metrics mapping pools -> number of ready replicas.
// This combines them into a list of PoolStats types acceptable to the scheduler.
func ToStats(specs []elasticsearchv1beta1.PoolSpec, metricsList map[string]*elasticsearchv1beta1.PoolSetMetrics) (res []PoolStats) {
	poolMap := make(map[string]*PoolStats)

	for _, spec := range specs {
		poolMap[spec.Name] = &PoolStats{
			Name:     spec.Name,
			Desired:  spec.Replicas,
			Dangling: false,
		}
	}

	for name, metrics := range metricsList {
		if stats, ok := poolMap[name]; !ok {
			// exists in status, but not in spec. This pool is a dangling reference
			poolMap[name] = &PoolStats{
				Name:     name,
				Ready:    metrics.Ready,
				Desired:  0,
				Dangling: true,
				// until overridden, schedule with the number of replicas currently ready.
				ScheduleReplicas: metrics.Ready,
			}
		} else {
			stats.Ready = metrics.Ready
			// until overridden, schedule with the number of replicas currently ready.
			stats.ScheduleReplicas = stats.Ready
		}
	}

	for _, stats := range poolMap {
		res = append(res, *stats)
	}
	return res
}

func NumReady(xs []PoolStats) (ct int32) {
	for _, stats := range xs {
		ct += stats.Ready
	}
	return ct
}

// LessThanDesired returns true if one of the non-danngling pools wants to scale up
func LessThanDesired(xs []PoolStats) bool {
	for _, x := range xs {
		if !x.Dangling && x.Ready < x.Desired {
			return true
		}
	}
	return false
}

// PoolsForScheduling returns the list of desired pool stats for the next round of scheduling
func PoolsForScheduling(
	desired int32,
	xs []PoolStats,
) ([]PoolStats, error) {
	ready := NumReady(xs)

	if desired > ready {
		return scaleUp(xs)
	} else if desired < ready {
		return scaleDown(xs)
	} else if LessThanDesired(xs) {
		// we have alive=desired, but they're distributed across different node pools
		// than we'd like. Trigger a scale-up, which will allocate a node
		// to a pool we want and afterwards it'll trigger a
		// scale down, removing from a pool we don't want allocated.
		return scaleUp(xs)
	}

	return xs, nil
}

func sortPoolBy(xs []PoolStats, lessFn func(PoolStats, PoolStats) bool) {
	sortFn := func(i, j int) bool {
		return lessFn(xs[i], xs[j])
	}
	sort.SliceStable(xs, sortFn)
}

func scaleUp(xs []PoolStats) ([]PoolStats, error) {

	if len(xs) == 0 {
		return xs, fmt.Errorf("0 len PoolStats")
	}
	// sort pools with smallest ready/replicas ratio to the front
	ratio := func(x PoolStats) float64 {
		return float64(x.Ready) / float64(x.Desired)
	}
	lessFn := func(a, b PoolStats) bool {
		// always sort dangling refs to the back
		if b.Dangling {
			return true
		}

		return ratio(a) < ratio(b)
	}

	sortPoolBy(xs, lessFn)
	first := &xs[0]
	first.ScheduleReplicas += 1
	return xs, nil
}

func scaleDown(xs []PoolStats) ([]PoolStats, error) {
	if len(xs) == 0 {
		return xs, fmt.Errorf("0 len PoolStats")
	}
	// sort pools with largest ready/replicas ratio to the front
	ratio := func(x PoolStats) float64 {
		return float64(x.Ready) / float64(x.Desired)
	}
	lessFn := func(a, b PoolStats) bool {
		// always sort dangling refs to the front to be scaled down first
		if b.Dangling {
			return false
		}

		return ratio(a) > ratio(b)
	}

	sortPoolBy(xs, lessFn)
	first := &xs[0]
	// adjusting the schedulereplicas down by one will be handled after the ready count has dropped
	// by the controlling handler process on the pod once the node is marked unschedulable.
	first.LastUnschedulable = true
	return xs, nil
}
