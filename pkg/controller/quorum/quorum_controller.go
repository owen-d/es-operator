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

package quorum

import (
	"context"
	"fmt"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	"github.com/owen-d/es-operator/pkg/controller/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// Add creates a new Quorum Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileQuorum{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("quorum-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Quorum
	err = c.Watch(&source.Kind{Type: &elasticsearchv1beta1.Quorum{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch for changes to StatefulSet created by quorum
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1beta1.Quorum{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileQuorum{}

// ReconcileQuorum reconciles a Quorum object
type ReconcileQuorum struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Quorum object and makes changes based on the state read
// and what is in the Quorum.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=quorums,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=quorums/status,verbs=get;update;patch
func (r *ReconcileQuorum) Reconcile(request reconcile.Request) (res reconcile.Result, err error) {
	instance := &elasticsearchv1beta1.Quorum{}
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

	if res, err := r.ReconcileServices(instance); err != nil {
		return res, err
	}

	var nextReplicaSize, newQuorum int32
	/* scale downs are done stepwise (one at a time) to avoid two problems:
	1) dropping new min masters much lower and then seeing a network partition could cause a split brain
	2) master eligible nodes may also be data nodes. Therefore we don't want to carelessly delete shards
	   and bring the cluster to a red state
	*/
	if alive := instance.Status.Alive(); instance.Spec.DesiredReplicas() < alive {
		nextReplicaSize = alive - 1
		newQuorum = util.ComputeQuorum(nextReplicaSize)

	} else if instance.Spec.DesiredReplicas() == alive {
		nextReplicaSize = alive
		newQuorum = util.ComputeQuorum(alive)
	} else {
		nextReplicaSize = alive + 1
		newQuorum = util.ComputeQuorum(alive)
	}

	log.Info("calculated", "nextReplicaSize", nextReplicaSize, "newQuorum", newQuorum)

	if res, err = r.ReconcileMinMasters(instance, newQuorum); err != nil {
		return res, err
	}

	if res, err = r.ReconcileStatefulSets(instance, nextReplicaSize); err != nil {
		return res, err
	}

	return reconcile.Result{}, nil

}

// ReconcileMinMasters first updates the configmap that is responsible for setting min_masters
// and then pings the elasticsearch api to set it dynamically on already-provisioned nodes,
// avoiding an otherwise necessary restart.
// TODO(owen): implement
func (r *ReconcileQuorum) ReconcileMinMasters(quorum *elasticsearchv1beta1.Quorum, minMasters int32) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (r *ReconcileQuorum) ReconcileStatus(quorum *elasticsearchv1beta1.Quorum) (reconcile.Result, error) {
	var err error
	aliveMap := make(map[string]int32)

	for _, pool := range quorum.Spec.NodePools {

		name := util.StatefulSetName(quorum.Spec.ClusterName, pool.Name)
		found := &appsv1.StatefulSet{}
		err = r.Get(context.TODO(), types.NamespacedName{
			Name:      name,
			Namespace: quorum.Namespace,
		}, found)
		if err != nil && errors.IsNotFound(err) {
			aliveMap[name] = 0
		} else if err != nil {
			return reconcile.Result{}, err
		} else {
			aliveMap[name] = found.Status.ReadyReplicas
		}
	}

	quorum.Status.AliveReplicas = aliveMap

	err = r.Status().Update(context.TODO(), quorum)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

// SplitOverPools will round-robin replica allocation, but not overfill each pool past their
// desired replica count. It errors if supplied 0 capacity (pools with a combined 0 replicas)
//  or if the requests exceed capacity.
// Returns an updated set of pools with replicas adjusted to a total of [requests] across all pools.
func SplitOverPools(
	requests int32,
	pools []elasticsearchv1beta1.NodePool,
) ([]elasticsearchv1beta1.NodePool, error) {
	var counts []struct {
		int32
		elasticsearchv1beta1.NodePool
	}
	// initialize counts
	for _, pool := range pools {
		counts = append(counts, struct {
			int32
			elasticsearchv1beta1.NodePool
		}{
			0,
			pool,
		})
	}

	allocateNext := func(
		cursor int,
		counts []struct {
			int32
			elasticsearchv1beta1.NodePool
		},
	) (int, error) {

		// allow up to one entire loop over pools
		for i := 0; i < len(counts); i++ {
			cur := &counts[cursor]
			var nextCursor int

			if cursor == len(counts)-1 {
				// at end of slice, circle back
				nextCursor = 0
			} else {
				nextCursor = cursor + 1
			}

			// current slot has capacity, increment
			if cur.int32 < cur.NodePool.Replicas {
				cur.int32 += 1
				return nextCursor, nil

			}
			cursor = nextCursor
		}

		return cursor, fmt.Errorf("no capacity for replica in Spec")
	}

	for i, cursor := 0, 0; i < int(requests); i++ {
		var err error
		cursor, err = allocateNext(cursor, counts)
		if err != nil {
			return nil, err
		}
	}

	var results []elasticsearchv1beta1.NodePool
	for _, x := range counts {
		adjusted := x.NodePool.DeepCopy()
		adjusted.Replicas = x.int32
		results = append(results, *adjusted)
	}

	return results, nil
}

func (r *ReconcileQuorum) ReconcileServices(
	quorum *elasticsearchv1beta1.Quorum,
) (reconcile.Result, error) {
	var err error

	for _, pool := range quorum.Spec.NodePools {
		name := util.StatefulSetService(quorum.Spec.ClusterName, pool.Name)
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: quorum.Namespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Ports: []corev1.ServicePort{
					corev1.ServicePort{Port: 9200},
				},
				Selector: map[string]string{
					"statefulSet": util.StatefulSetName(quorum.Spec.ClusterName, pool.Name),
				},
			},
		}

		if err = controllerutil.SetControllerReference(quorum, svc, r.scheme); err != nil {
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
		}

		if !reflect.DeepEqual(svc.Spec, found.Spec) {
			found.Spec = svc.Spec
			log.Info("Updating Svc", "namespace", svc.Namespace, "name", svc.Name)
			err = r.Update(context.TODO(), found)
			if err != nil {
				return reconcile.Result{}, err
			}
		}

	}

	return reconcile.Result{}, nil
}

func (r *ReconcileQuorum) ReconcileStatefulSets(
	quorum *elasticsearchv1beta1.Quorum,
	nextReplicaSize int32,
) (reconcile.Result, error) {
	var err error

	adjustedPools, err := SplitOverPools(nextReplicaSize, quorum.Spec.NodePools)
	if err != nil {
		return reconcile.Result{}, err
	}

	return util.ReconcileStatefulSets(
		r,
		r.scheme,
		log,
		quorum,
		quorum.Spec.ClusterName,
		quorum.Namespace,
		adjustedPools,
	)
}
