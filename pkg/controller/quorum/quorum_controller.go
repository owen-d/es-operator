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
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
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

	res, err = r.UpdateStatus(instance)
	return res, err

}

func (r *ReconcileQuorum) UpdateStatus(quorum *elasticsearchv1beta1.Quorum) (reconcile.Result, error) {
	var err error
	deployMap := make(map[string]int32)

	for _, pool := range quorum.Spec.NodePools {

		deployName := pool.DeployName(quorum.Spec.ClusterName)
		found := &appsv1.Deployment{}
		err = r.Get(context.TODO(), types.NamespacedName{
			Name:      deployName,
			Namespace: quorum.Namespace,
		}, found)
		if err != nil && errors.IsNotFound(err) {
			deployMap[deployName] = 0
		} else if err != nil {
			return reconcile.Result{}, err
		} else {
			deployMap[deployName] = found.Status.AvailableReplicas
		}
	}

	quorum.Status.Deployments = deployMap

	err = r.Status().Update(context.TODO(), quorum)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
