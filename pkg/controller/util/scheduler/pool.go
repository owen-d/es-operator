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
// as well as the next PodDisruptionBudget minAvailable number.
// We set the PDB to one higher than desired when scaling
// down to allow the node to be drained by it's handler after it's configmap updates and
// labels it unschedulable
func PoolsForScheduling(
	desired int32,
	xs []PoolStats,
) ([]PoolStats, int, error) {
	ready := NumReady(xs)

	if desired > ready {
		err := scaleUp(xs)
		return xs, int(ready + 1), err
	} else if desired < ready {
		err := scaleDown(xs)
		return xs, int(ready), err
	} else if LessThanDesired(xs) {
		// we have alive=desired, but they're distributed across different node pools
		// than we'd like. Trigger a scale-up, which will allocate a node
		// to a pool we want and afterwards it'll trigger a
		// scale down, removing from a pool we don't want allocated.
		err := scaleUp(xs)
		return xs, int(ready + 1), err
	}

	return xs, int(ready), nil
}

func sortPoolBy(xs []PoolStats, lessFn func(PoolStats, PoolStats) bool) {
	sortFn := func(i, j int) bool {
		return lessFn(xs[i], xs[j])
	}
	sort.SliceStable(xs, sortFn)
}

func scaleUp(xs []PoolStats) (err error) {
	if len(xs) == 0 {
		return fmt.Errorf("0 len PoolStats")
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
	return nil
}

func scaleDown(xs []PoolStats) (err error) {
	if len(xs) == 0 {
		return fmt.Errorf("0 len PoolStats")
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
	first.ScheduleReplicas -= 1
	return nil
}
