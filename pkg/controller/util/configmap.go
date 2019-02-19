package util

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const configTemplate = `
node.name: ${POD_NAME}
node.master: ${NODE_MASTER}
node.data: ${NODE_DATA}
node.ingest: ${NODE_INGEST}
network.host: 0.0.0.0
discovery:
  zen:
    ping.unicast.hosts: ${DISCOVERY_URL}
    minimum_master_nodes: %d
xpack.security.enabled: false
`

func QuorumConfigMap(
	clusterName string,
	namespace string,
	minMasters int32,
	owner metav1.Object,
	scheme *runtime.Scheme,
) (*corev1.ConfigMap, error) {
	cMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(clusterName),
			Namespace: namespace,
		},
		Data: map[string]string{
			"elasticsearch.yml": generateConfig(minMasters),
		},
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, cMap, scheme); err != nil {
			return nil, err
		}
	}

	return cMap, nil
}

func generateConfig(minMasters int32) string {
	return fmt.Sprintf(configTemplate, minMasters)
}
