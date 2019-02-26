package util

import (
	"context"
	logr "github.com/go-logr/logr"
	policy "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func EnsurePDB(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	owner metav1.Object,
	namespace string,
	name string,
	spec policy.PodDisruptionBudgetSpec,
	labels map[string]string,
) (err error) {
	pdb := &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: spec,
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, pdb, scheme); err != nil {
			return err
		}
	}

	found := &policy.PodDisruptionBudget{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      pdb.Name,
		Namespace: pdb.Namespace,
	}, found)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating PodDisruptionBudget", "namespace", pdb.Namespace, "name", pdb.Name)
		return client.Create(context.TODO(), pdb)
	} else if err != nil {
		return err
	}

	if !reflect.DeepEqual(pdb.Spec, found.Spec) {
		log.Info(
			"Found existing PodDisruptionBudget, attempting to delete/recreate",
			"namespace",
			pdb.Namespace,
			"name",
			pdb.Name,
		)

		if err = client.Delete(context.TODO(), found); err != nil {
			return err
		}
		return client.Create(context.TODO(), pdb)
	}

	// TODO: replace this with an update when/if it's implemented. Gated by
	// https://github.com/kubernetes/kubernetes/issues/45398

	return nil

}

func MkPdbMinAvailable(
	maxUnavailable int,
	labels map[string]string,
) policy.PodDisruptionBudgetSpec {
	min := intstr.FromInt(maxUnavailable)

	return policy.PodDisruptionBudgetSpec{
		MinAvailable: &min,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
	}
}
