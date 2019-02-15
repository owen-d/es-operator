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

	// watch for changes to pools created by quorum
	err = c.Watch(&source.Kind{Type: &elasticsearchv1beta1.Pool{}}, &handler.EnqueueRequestForOwner{
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
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=quorums,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=quorums/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=pools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=pools/status,verbs=get;update;patch
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

	// /* scale downs are done stepwise (one at a time) to avoid two problems:
	// 1) dropping new min masters much lower and then seeing a network partition could cause a split brain
	// 2) master eligible nodes may also be data nodes. Therefore we don't want to carelessly delete shards
	//    and bring the cluster to a red state
	// */
	var ready, desired, nextReplicaSize, newQuorum int32
	if ready, desired = instance.Status.ReadyReplicas(), instance.Spec.DesiredReplicas(); desired < ready {
		nextReplicaSize = ready - 1
		newQuorum = util.ComputeQuorum(nextReplicaSize)

	} else if desired == ready {
		nextReplicaSize = ready
		newQuorum = util.ComputeQuorum(ready)
	} else {
		nextReplicaSize = ready + 1
		newQuorum = util.ComputeQuorum(ready)
	}

	log.Info(
		"calculated new quorum",
		"nextReplicaSize", nextReplicaSize,
		"quorumSize", newQuorum,
		"currentlyReady", ready,
		"desired", desired,
	)

	if res, err = r.ReconcileMinMasters(instance, newQuorum); err != nil {
		return res, err
	}

	if res, err = r.ReconcilePools(instance); err != nil {
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
	pools := &elasticsearchv1beta1.PoolList{}

	err = r.List(context.TODO(),
		client.
			// TODO(owen): quorum should only update status with master eligible pools.
			// A pool could be changed from master eligible to non-master eligible.
			// This could also be gated by webhooks.
			// MatchingField("status.masterEligible", "true").
			InNamespace(quorum.Namespace).
			MatchingLabels(map[string]string{
				util.QuorumLabelKey: quorum.Name,
			}),
		pools)

	if err != nil {
		return reconcile.Result{}, err
	}

	current := make(map[string]elasticsearchv1beta1.PoolSetMetrics)
	for _, pool := range pools.Items {
		poolMetrics := elasticsearchv1beta1.PoolSetMetrics{}
		for _, set := range pool.Status.StatefulSets {
			poolMetrics.Replicas += set.Replicas
			poolMetrics.Ready += set.Ready
		}
		current[pool.Name] = poolMetrics
	}

	newStatus := quorum.Status.DeepCopy()
	newStatus.ReadyPools = current

	if !reflect.DeepEqual(newStatus, quorum.Status) {
		quorum.Status = *newStatus
		if err = r.Status().Update(context.TODO(), quorum); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// SplitOverPools will round-robin replica allocation, but not overfill each pool past their
// desired replica count. It errors if supplied 0 capacity (pools with a combined 0 replicas)
//  or if the requests exceed capacity.
// Returns an updated set of pools with replicas adjusted to a total of [requests] across all pools.
func SplitOverPools(
	requests int32,
	pools []elasticsearchv1beta1.PoolSpec,
) ([]elasticsearchv1beta1.PoolSpec, error) {
	var counts []struct {
		int32
		elasticsearchv1beta1.PoolSpec
	}
	// initialize counts
	for _, pool := range pools {
		counts = append(counts, struct {
			int32
			elasticsearchv1beta1.PoolSpec
		}{
			0,
			pool,
		})
	}

	allocateNext := func(
		cursor int,
		counts []struct {
			int32
			elasticsearchv1beta1.PoolSpec
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
			if cur.int32 < cur.PoolSpec.Replicas {
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

	var results []elasticsearchv1beta1.PoolSpec
	for _, x := range counts {
		adjusted := x.PoolSpec.DeepCopy()
		adjusted.Replicas = x.int32
		results = append(results, *adjusted)
	}

	return results, nil
}

func (r *ReconcileQuorum) ReconcilePools(
	quorum *elasticsearchv1beta1.Quorum,
) (reconcile.Result, error) {

	clusterName, err := util.ExtractKey(quorum.Labels, util.ClusterLabelKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	extraLabels := map[string]string{
		util.QuorumLabelKey:  quorum.Name,
		util.ClusterLabelKey: clusterName,
	}

	return util.ReconcilePools(
		r,
		r.scheme,
		log,
		quorum,
		clusterName,
		quorum.Namespace,
		quorum.Spec.NodePools,
		extraLabels,
	)
}
