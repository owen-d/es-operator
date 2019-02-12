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
	"github.com/owen-d/es-operator/pkg/controller/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"math"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
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

	if res, err = r.ReconcileStatus(instance); err != nil {
		return res, err
	}

	if res, err = r.ReconcilePDB(instance); err != nil {
		return res, err
	}

	/* scale downs are done stepwise (one at a time) to avoid two problems:
	1) dropping new min masters much lower and then seeing a network partition could cause a split brain
	2) master eligible nodes may also be data nodes. Therefore we don't want to carelessly delete shards
	   and bring the cluster to a red state
	*/
	if alive := instance.Status.Alive(); instance.Spec.DesiredReplicas() < alive {
		newQuorum := util.ComputeQuorum(alive - 1)

		if res, err = r.ReconcileMinMasters(instance, newQuorum); err != nil {
			return res, err
		}

		// TODO(owen): this could be improved to be increased in a more granular stepwise
		// fashion, similar to the scale downs above.
		// It's a little more logic due to no maxAvailable/minUnavailable fields,
		// so I'm leaving this for later
	} else {
		newQuorum := util.ComputeQuorum(
			int32(math.Min(
				float64(alive),
				float64(instance.Spec.DesiredReplicas()),
			)),
		)

		if res, err = r.ReconcileMinMasters(instance, newQuorum); err != nil {
			return res, err
		}

	}

	// both paths update deployments
	if res, err = r.ReconcileDeployments(instance); err != nil {
		return res, err
	}

	return reconcile.Result{}, nil

}

func (r *ReconcileQuorum) ReconcilePDB(
	quorum *elasticsearchv1beta1.Quorum,
) (reconcile.Result, error) {
	pdbName := strings.Join([]string{quorum.Spec.ClusterName, "master", "pdb"}, "-")
	var err error
	var matchingDeploys []string
	for _, pool := range quorum.Spec.NodePools {
		matchingDeploys = append(matchingDeploys, pool.DeployName(quorum.Spec.ClusterName))
	}

	maxUnavailable := intstr.FromInt(1)
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: quorum.Namespace,
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "deployment",
						Operator: metav1.LabelSelectorOpIn,
						Values:   matchingDeploys,
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(quorum, pdb, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &policyv1beta1.PodDisruptionBudget{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: pdb.Name, Namespace: pdb.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating PodDisruptionBudget", "namespace", pdb.Namespace, "name", pdb.Name)
		err = r.Create(context.TODO(), pdb)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(pdb.Spec, found.Spec) {
		found.Spec = pdb.Spec
		log.Info("Updating PodDisruptionBudget", "namespace", pdb.Namespace, "name", pdb.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
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

func (r *ReconcileQuorum) ReconcileDeployments(quorum *elasticsearchv1beta1.Quorum) (reconcile.Result, error) {
	var err error
	for _, pool := range quorum.Spec.NodePools {

		deployName := pool.DeployName(quorum.Spec.ClusterName)

		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployName,
				Namespace: quorum.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &pool.Replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"deployment": deployName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"deployment": deployName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:      "nginx",
								Image:     "nginx",
								Resources: pool.Resources,
							},
						},
					},
				},
			},
		}

		if err := controllerutil.SetControllerReference(quorum, deploy, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		found := &appsv1.Deployment{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
			err = r.Create(context.TODO(), deploy)
			return reconcile.Result{}, err
		} else if err != nil {
			return reconcile.Result{}, err
		}

		if !reflect.DeepEqual(deploy.Spec, found.Spec) {
			found.Spec = deploy.Spec
			log.Info("Updating Deployment", "namespace", deploy.Namespace, "name", deploy.Name)
			err = r.Update(context.TODO(), found)
			if err != nil {

				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}
