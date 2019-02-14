package util

import (
	"strings"
)

func VolumeNameTemplate(cluster string) string {
	return strings.Join([]string{"elasticsearch", cluster, "data"}, "-")
}

// This configures the required headless service used in statefulsets
func StatefulSetService(cluster, setName string) string {
	return strings.Join([]string{"elasticsearch", cluster, setName, "headless"}, "-")
}

func StatefulSetName(cluster, poolName string) string {
	return strings.Join([]string{"elasticsearch", cluster, poolName, "statefulset"}, "-")
}

func PoolName(cluster, poolName string) string {
	return strings.Join([]string{cluster, poolName, "pool"}, "-")
}

func QuorumName(cluster string) string {
	return strings.Join([]string{cluster, "quorum"}, "-")
}
