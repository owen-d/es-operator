package util

import (
	"fmt"
	"strings"
)

// This configures the required headless service used in statefulsets
func StatefulSetService(cluster, setName string) string {
	return strings.Join([]string{cluster, setName, "headless"}, "-")
}

func StatefulSetName(cluster, poolName string) string {
	return PoolName(cluster, poolName)
}

func PoolName(cluster, poolName string) string {
	return strings.Join([]string{cluster, poolName}, "-")
}

func QuorumName(cluster string) string {
	return cluster
}

// ConfigMapName refers to the default configmap used by all nodes (contains minMasters & discovery)
func QuorumConfigMapName(cluster string) string {
	return strings.Join([]string{cluster, "quorum"}, "-")
}

// PoolSchedulableConfigMapName refers to the default configmap used by all nodes (contains minMasters & discovery)
func PoolSchedulableConfigMapName(cluster, pool string) string {
	return strings.Join([]string{PoolName(cluster, pool), "schedulable"}, "-")
}

func MasterDiscoveryServiceName(cluster string) string {
	return strings.Join([]string{cluster, "master", "discovery"}, "-")
}

// ClusterServiceName is the default service used for a cluster and includes all pods
func ClusterServiceName(cluster string) string {
	return cluster
}

func QuorumLabels(clusterName, quorumName string) map[string]string {
	return map[string]string{
		QuorumLabelKey:  quorumName,
		ClusterLabelKey: clusterName,
	}
}

func DataVolumeNameTemplate(cluster, pool string) string {
	return strings.Join([]string{cluster, pool, "data"}, "-")
}

func PodName(cluster string, pool string, idx int) string {
	return fmt.Sprintf("%s-%d", PoolName(cluster, pool), idx)
}

func UniqueStrings(xs ...string) []string {
	cache := make(map[string]struct{})
	for _, x := range xs {
		cache[x] = struct{}{}
	}

	res := make([]string, len(cache))
	for k, _ := range cache {
		res = append(res, k)
	}

	return res
}

func StringIn(s string, xs ...string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func CatImage(image, tag string) string {
	return fmt.Sprintf("%s:%s", image, tag)
}

func DiscoveryServiceDNS(clusterName, nameSpace string) string {
	return strings.Join([]string{
		MasterDiscoveryServiceName(clusterName),
		nameSpace,
		"svc",
		"cluster",
		"local",
	}, ".")
}
