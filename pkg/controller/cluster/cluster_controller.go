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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	// watch a Deployment created by Cluster
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1beta1.Cluster{},
	})
	if err != nil {
		return err
	}

	// watch for changes to pod disruption budget created by cluster
	err = c.Watch(&source.Kind{Type: &policyv1beta1.PodDisruptionBudget{}}, &handler.EnqueueRequestForOwner{
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
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.io,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
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

	if !instance.Status.Exists {
		return r.InitialCreate(instance)
	}

	res, err = r.ReconcilePDB(instance)
	if err != nil {
		return res, err
	}

	res, err = r.ReconcileDeployments(instance)
	if err != nil {
		return res, err
	}
	return res, nil

}

func (r *ReconcileCluster) InitialCreate(cluster *elasticsearchv1beta1.Cluster) (
	res reconcile.Result,
	err error,
) {
	res, err = r.ReconcilePDB(cluster)
	if err != nil {
		return res, err
	}

	res, err = r.ReconcileDeployments(cluster)
	if err != nil {
		return res, err
	}

	// cluster creates are the first time where we don't care about minimum_masters changes
	// causing downtime. After setting Exists to true, this will no longer fire
	// TODO(owen): replace this with a computable form as per k8s best practices:
	// compute whether the cluster exists by presence of it's constituent parts.
	cluster.Status.Exists = true
	r.Status().Update(context.TODO(), cluster)

	return res, nil
}

func (r *ReconcileCluster) ReconcilePDB(cluster *elasticsearchv1beta1.Cluster) (res reconcile.Result, err error) {
	pdbName := strings.Join([]string{cluster.Name, "quorum", "pdb"}, "-")
	var matchingDeploys []string
	for _, pool := range cluster.Spec.MasterPools() {
		matchingDeploys = append(matchingDeploys, cluster.DeployName(pool))
	}

	minAvailable := intstr.FromInt(int(cluster.Spec.Quorum()))
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: cluster.Namespace,
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
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

	if err := controllerutil.SetControllerReference(cluster, pdb, r.scheme); err != nil {
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

func (r *ReconcileCluster) ReconcileDeployments(cluster *elasticsearchv1beta1.Cluster) (reconcile.Result, error) {
	var err error
	for _, pool := range cluster.Spec.NodePools {

		deployName := cluster.DeployName(pool)

		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployName,
				Namespace: cluster.Namespace,
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

		if err := controllerutil.SetControllerReference(cluster, deploy, r.scheme); err != nil {
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
