/*
Copyright 2019 Owen Diehl.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pool

import (
	"context"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	"github.com/owen-d/es-operator/pkg/controller/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new Pool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePool{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("pool-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Pool
	err = c.Watch(&source.Kind{Type: &elasticsearchv1beta1.Pool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1beta1.Pool{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1beta1.Pool{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePool{}

// ReconcilePool reconciles a Pool object
type ReconcilePool struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Pool object and makes changes based on the state read
// and what is in the Pool.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=pools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=pools/status,verbs=get;update;patch
func (r *ReconcilePool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	var err error
	instance := &elasticsearchv1beta1.Pool{}

	err = r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if res, err := r.ReconcileStatus(instance); err != nil {
		return res, err
	}

	clusterName, err := util.ExtractKey(instance.Labels, util.ClusterLabelKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	extraLabels := map[string]string{
		util.ClusterLabelKey: clusterName,
		util.PoolLabelKey:    instance.Name,
	}

	// adds quorum label to resulting statefulsets if exists (i.e. for master discovery svc)
	if quorumName, err := util.ExtractKey(instance.Labels, util.QuorumLabelKey); err == nil {
		log.Info("adding label", util.QuorumLabelKey, quorumName, "targetPool", instance.Name)
		extraLabels[util.QuorumLabelKey] = quorumName
	}

	schedulableConfigMap, err := util.SchedulableConfigMap(
		clusterName,
		instance.Namespace,
		instance.Spec,
		instance,
		r.scheme,
	)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = util.ReconcileConfigMap(
		r,
		r.scheme,
		log,
		instance,
		schedulableConfigMap,
	)
	if err != nil {
		return reconcile.Result{}, err
	}

	return util.ReconcileStatefulSet(
		r,
		r.scheme,
		log,
		instance,
		clusterName,
		instance.Namespace,
		instance.Spec,
		extraLabels,
	)
}

func (r *ReconcilePool) ReconcileStatus(pool *elasticsearchv1beta1.Pool) (reconcile.Result, error) {
	var err error
	sets := &appsv1.StatefulSetList{}

	err = r.List(context.TODO(),
		client.
			InNamespace(pool.Namespace).
			MatchingLabels(map[string]string{
				util.PoolLabelKey: pool.Name,
			}),
		sets)
	if err != nil {
		return reconcile.Result{}, err
	}

	current := make(map[string]*elasticsearchv1beta1.PoolSetMetrics)
	for _, set := range sets.Items {
		current[set.Name] = &elasticsearchv1beta1.PoolSetMetrics{
			ResolvedName: set.Name,
			Replicas:     set.Status.Replicas,
			Ready:        set.Status.ReadyReplicas,
		}
	}

	newStatus := pool.Status.DeepCopy()
	newStatus.StatefulSets = current
	newStatus.KubectlReplicasAnnotation = newStatus.FormatReplicasAnnotation()

	if !reflect.DeepEqual(newStatus, pool.Status) {
		pool.Status = *newStatus
		if err = r.Status().Update(context.TODO(), pool); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
