package util

import (
	"context"
	logr "github.com/go-logr/logr"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	"github.com/owen-d/es-operator/pkg/controller/util/scheduler"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func ReconcilePools(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	owner metav1.Object,
	clusterName string,
	namespace string,
	pools []elasticsearchv1beta1.PoolSpec,
	extraLabels map[string]string,
) (reconcile.Result, error) {
	var err error
	for _, spec := range pools {
		name := PoolName(clusterName, spec.Name)

		pool := &elasticsearchv1beta1.Pool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    extraLabels,
			},
			Spec: spec,
		}

		if owner != nil {
			if err := controllerutil.SetControllerReference(owner, pool, scheme); err != nil {
				return reconcile.Result{}, err
			}
		}

		found := &elasticsearchv1beta1.Pool{}
		err = client.Get(context.TODO(), types.NamespacedName{
			Name:      pool.Name,
			Namespace: pool.Namespace,
		}, found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating Pool", "namespace", pool.Namespace, "name", pool.Name)
			err = client.Create(context.TODO(), pool)
			return reconcile.Result{}, err
		} else if err != nil {
			return reconcile.Result{}, err
		}

		if !reflect.DeepEqual(pool.Spec, found.Spec) {
			found.Spec = pool.Spec
			log.Info("Updating Pool", "namespace", pool.Namespace, "name", pool.Name)
			err = client.Update(context.TODO(), found)
			if err != nil {

				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

func SpecByName(
	name string,
	specs []elasticsearchv1beta1.PoolSpec,
) (res elasticsearchv1beta1.PoolSpec, exists bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return res, false
}

func ToPools(
	client client.Client,
	namespace string,
	poolSpecs []elasticsearchv1beta1.PoolSpec,
	statsList []scheduler.PoolStats,
) (res []elasticsearchv1beta1.PoolSpec, err error) {
	for _, stats := range statsList {
		spec, inSpec := SpecByName(stats.Name, poolSpecs)

		if inSpec {
			spec.Replicas = stats.ScheduleReplicas
			res = append(res, spec)
		} else {
			// need load from api fetch
			found := &elasticsearchv1beta1.Pool{}
			err = client.Get(context.TODO(), types.NamespacedName{
				Name:      stats.Name,
				Namespace: namespace,
			}, found)

			if err != nil {
				// may have just been deleted, or another err
				return nil, err
			}

			found.Spec.Replicas = stats.ScheduleReplicas
			res = append(res, found.Spec)
		}

	}
	return res, err
}
