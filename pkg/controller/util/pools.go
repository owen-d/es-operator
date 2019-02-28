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

// ToPools will merge a list of PoolSpecs and PoolStats, fetching from the api if necessary
func ToPools(
	client client.Client,
	clusterName string,
	namespace string,
	poolSpecs []elasticsearchv1beta1.PoolSpec,
	statsList []scheduler.PoolStats,
) (res, forDeletion []elasticsearchv1beta1.PoolSpec, err error) {

	injectUnschedulableNodes := func(stats scheduler.PoolStats, spec *elasticsearchv1beta1.PoolSpec) {
		// pods are zero-indexed while replica requests are 1-indexed. align them.
		idx := int(stats.ScheduleReplicas - 1)

		if stats.LastUnschedulable && !spec.ContainsUnschedulable(idx) {
			spec.Unschedulable = append(spec.Unschedulable, idx)
		}
	}

	for _, stats := range statsList {
		spec, inSpec := SpecByName(stats.Name, poolSpecs)

		if inSpec {
			spec.Replicas = stats.ScheduleReplicas
			injectUnschedulableNodes(stats, &spec)
			res = append(res, spec)
		} else if stats.Dangling && stats.ScheduleReplicas == 0 {
			// is a dangling pool with no replicas; deletable
			forDeletion = append(forDeletion, elasticsearchv1beta1.PoolSpec{
				Name: stats.Name,
			})

		} else {
			// need load from api fetch
			found := &elasticsearchv1beta1.Pool{}
			err = client.Get(context.TODO(), types.NamespacedName{
				Name:      PoolName(clusterName, stats.Name),
				Namespace: namespace,
			}, found)

			if err != nil {
				// may have just been deleted, or another err
				return nil, nil, err
			}

			found.Spec.Replicas = stats.ScheduleReplicas
			injectUnschedulableNodes(stats, &found.Spec)
			res = append(res, found.Spec)
		}

	}
	return res, forDeletion, err
}

func EnsurePoolsDeleted(
	client client.Client,
	log logr.Logger,
	clusterName string,
	namespace string,
	specs []elasticsearchv1beta1.PoolSpec,
) error {
	var err error
	for _, spec := range specs {
		name := PoolName(clusterName, spec.Name)

		pool := &elasticsearchv1beta1.Pool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}

		log.Info("Deleting Pool", "namespace", pool.Namespace, "name", pool.Name)
		err = client.Delete(context.TODO(), pool)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// ResolvePools will determine what the desired replica counts are as well as any possible unpruned pool deletions.
// It also returns the next desired pod disruption MinAvailable number
func ResolvePools(
	client client.Client,
	clusterName string,
	namespace string,
	specs []elasticsearchv1beta1.PoolSpec,
	metrics map[string]*elasticsearchv1beta1.PoolSetMetrics,
	desired int32,
) (
	[]elasticsearchv1beta1.PoolSpec,
	[]elasticsearchv1beta1.PoolSpec,
	error,
) {
	stats, err := scheduler.PoolsForScheduling(
		desired,
		scheduler.ToStats(specs, metrics),
	)

	if err != nil {
		return nil, nil, err
	}

	res, forDeletion, err := ToPools(
		client,
		clusterName,
		namespace,
		specs,
		stats,
	)

	if err != nil {
		return nil, nil, err
	}

	return res, forDeletion, nil
}
