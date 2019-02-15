package util

import (
	"strings"
)

func VolumeNameTemplate(cluster string) string {
	return strings.Join([]string{cluster, "data"}, "-")
}

// This configures the required headless service used in statefulsets
func StatefulSetService(cluster, setName string) string {
	return strings.Join([]string{cluster, setName, "headless"}, "-")
}

func StatefulSetName(cluster, poolName string, isQuorum bool) string {
	var args []string
	if isQuorum {
		args = []string{cluster, "quorum", poolName}
	} else {
		args = []string{cluster, "drone", poolName}
	}
	return strings.Join(args, "-")
}

func PoolName(cluster, poolName string) string {
	return strings.Join([]string{cluster, poolName}, "-")
}

func QuorumName(cluster string) string {
	return cluster
}
