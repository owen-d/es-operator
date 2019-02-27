package util

import (
	"context"
	"fmt"
	logr "github.com/go-logr/logr"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const configTemplate = `
cluster.name: %s
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
bootstrap.memory_lock: true
`

type SchedulableConfig struct {
	SchedulableNodes []string `yaml:"schedulableNodes"`
}

func QuorumConfigMap(
	clusterName string,
	namespace string,
	minMasters int32,
	owner metav1.Object,
	scheme *runtime.Scheme,
) (*corev1.ConfigMap, error) {
	cMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      QuorumConfigMapName(clusterName),
			Namespace: namespace,
		},
		Data: map[string]string{
			"elasticsearch.yml": generateConfig(clusterName, minMasters),
		},
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, cMap, scheme); err != nil {
			return nil, err
		}
	}

	return cMap, nil
}

func generateConfig(clusterName string, minMasters int32) string {
	return fmt.Sprintf(configTemplate, clusterName, minMasters)
}

func SchedulableConfigMap(
	clusterName string,
	namespace string,
	pool elasticsearchv1beta1.PoolSpec,
	owner metav1.Object,
	scheme *runtime.Scheme,
) (*corev1.ConfigMap, error) {
	data, err := generateShardDrainConfig(pool, clusterName)
	if err != nil {
		return nil, err
	}

	cMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PoolSchedulableConfigMapName(clusterName, pool.Name),
			Namespace: namespace,
		},
		Data: map[string]string{
			"schedulable.yml": data,
		},
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, cMap, scheme); err != nil {
			return nil, err
		}
	}

	return cMap, nil
}

func generateShardDrainConfig(pool elasticsearchv1beta1.PoolSpec, clusterName string) (string, error) {
	var conf SchedulableConfig
	for i := 0; i < int(pool.Replicas); i++ {
		if !pool.ContainsUnschedulable(i) {
			conf.SchedulableNodes = append(conf.SchedulableNodes, PodName(clusterName, pool.Name, i))
		}
	}
	b, err := yaml.Marshal(conf)
	return string(b), err

}

func ReconcileConfigMap(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	owner metav1.Object,
	cMap *corev1.ConfigMap,
) error {
	var err error

	found := &corev1.ConfigMap{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      cMap.Name,
		Namespace: cMap.Namespace,
	}, found)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating ConfigMap", "namespace", cMap.Namespace, "name", cMap.Name)
		err = client.Create(context.TODO(), cMap)
		return err
	} else if err != nil {
		return err
	}

	if !reflect.DeepEqual(cMap.Data, found.Data) {
		found.Data = cMap.Data
		log.Info("Updating ConfigMap", "namespace", cMap.Namespace, "name", cMap.Name)

		err = client.Update(context.TODO(), found)
		if err != nil {
			return err
		}
	}
	return nil
}
