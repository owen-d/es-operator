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

package cluster

import (
	"context"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	"github.com/owen-d/es-operator/pkg/controller/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new Cluster Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCluster{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Cluster
	err = c.Watch(&source.Kind{Type: &elasticsearchv1beta1.Cluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch for changes to pools created by cluster
	err = c.Watch(&source.Kind{Type: &elasticsearchv1beta1.Pool{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1beta1.Cluster{},
	})
	if err != nil {
		return err
	}

	// watch for changes to quorum created by cluster
	err = c.Watch(&source.Kind{Type: &elasticsearchv1beta1.Quorum{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1beta1.Cluster{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCluster{}

// ReconcileCluster reconciles a Cluster object
type ReconcileCluster struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Cluster object and makes changes based on the state read
// and what is in the Cluster.Spec
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=quorums,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=quorums/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=pools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=pools/status,verbs=get;update;patch
func (r *ReconcileCluster) Reconcile(request reconcile.Request) (res reconcile.Result, err error) {
	// Fetch the Cluster instance
	instance := &elasticsearchv1beta1.Cluster{}
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

	if res, err = r.ReconcileStatus(instance); err != nil {
		return res, err
	}

	if res, err = r.ReconcileService(instance); err != nil {
		return res, err
	}

	if res, err := r.ReconcilePools(instance); err != nil {
		return res, err
	}

	if res, err = r.ReconcileQuorum(instance); err != nil {
		return res, err
	}

	return res, nil

}

func (r *ReconcileCluster) ReconcileQuorum(cluster *elasticsearchv1beta1.Cluster) (
	res reconcile.Result,
	err error,
) {
	quorumName := util.QuorumName(cluster.Name)
	masterPools, _ := cluster.Spec.Pools()

	quorum := &elasticsearchv1beta1.Quorum{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quorumName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				util.ClusterLabelKey: cluster.Name,
			},
		},
		Spec: elasticsearchv1beta1.QuorumSpec{
			NodePools: masterPools,
		},
	}

	if err := controllerutil.SetControllerReference(cluster, quorum, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &elasticsearchv1beta1.Quorum{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: quorum.Name, Namespace: quorum.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Quorum", "namespace", quorum.Namespace, "name", quorum.Name)
		err = r.Create(context.TODO(), quorum)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(quorum.Spec, found.Spec) {
		found.Spec = quorum.Spec
		log.Info("Updating Quorum", "namespace", quorum.Namespace, "name", quorum.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil

}

func (r *ReconcileCluster) ReconcilePools(cluster *elasticsearchv1beta1.Cluster) (
	res reconcile.Result,
	err error,
) {
	_, dronePools := cluster.Spec.Pools()

	labels := util.DroneLabels(cluster.Name)

	desired := elasticsearchv1beta1.DesiredReplicas(dronePools)

	specs, forDeletion, err := util.ResolvePools(
		r,
		cluster.Name,
		cluster.Namespace,
		dronePools,
		cluster.Status.DronePools,
		desired,
	)

	if err != nil {
		return reconcile.Result{}, err
	}

	err = util.EnsurePoolsDeleted(
		r,
		log,
		cluster.Name,
		cluster.Namespace,
		forDeletion,
	)

	if err != nil {
		return reconcile.Result{}, err
	}

	return util.ReconcilePools(
		r,
		r.scheme,
		log,
		cluster,
		cluster.Name,
		cluster.Namespace,
		specs,
		labels,
	)
}

func (r *ReconcileCluster) ReconcileStatus(cluster *elasticsearchv1beta1.Cluster) (reconcile.Result, error) {
	var err error
	pools := &elasticsearchv1beta1.PoolList{}

	fetchOpts := client.
		InNamespace(cluster.Namespace).
		MatchingLabels(map[string]string{
			util.ClusterLabelKey: cluster.Name,
		})

	noQuorum, err := labels.NewRequirement(util.QuorumLabelKey, selection.DoesNotExist, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	fetchOpts.LabelSelector = fetchOpts.LabelSelector.Add(*noQuorum)

	err = r.List(context.TODO(), fetchOpts, pools)

	if err != nil {
		return reconcile.Result{}, err
	}

	current := make(map[string]*elasticsearchv1beta1.PoolSetMetrics)
	for _, pool := range pools.Items {
		poolMetrics := elasticsearchv1beta1.PoolSetMetrics{
			ResolvedName: pool.Name,
		}
		for _, set := range pool.Status.StatefulSets {
			poolMetrics.Replicas += set.Replicas
			poolMetrics.Ready += set.Ready
		}
		current[pool.Spec.Name] = &poolMetrics
	}

	newStatus := cluster.Status.DeepCopy()
	newStatus.DronePools = current

	if !reflect.DeepEqual(newStatus, cluster.Status) {
		cluster.Status = *newStatus
		if err = r.Status().Update(context.TODO(), cluster); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// TODO: implement ingest, coordinator endpoints as well
func (r *ReconcileCluster) ReconcileService(cluster *elasticsearchv1beta1.Cluster) (
	reconcile.Result,
	error,
) {
	var err error

	name := util.ClusterServiceName(cluster.Name)
	labels := map[string]string{
		util.ClusterLabelKey: cluster.Name,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{Port: 9200},
			},
			Selector: labels,
		},
	}

	if err := controllerutil.SetControllerReference(cluster, svc, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &corev1.Service{}
	err = r.Get(context.TODO(), types.NamespacedName{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}, found)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Service", "namespace", svc.Namespace, "name", svc.Name)
		err = r.Create(context.TODO(), svc)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	} else {
		// Since we do not specify a clusterIP, it is automatically assigned.
		// Therefore we don't want to try to update this immutable field, so align them
		svc.Spec.ClusterIP = found.Spec.ClusterIP
	}

	if !reflect.DeepEqual(svc.Spec, found.Spec) {
		found.Spec = svc.Spec
		log.Info("Updating Svc", "namespace", svc.Namespace, "name", svc.Name)

		err = r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
