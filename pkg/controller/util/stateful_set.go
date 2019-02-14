package util

import (
	"context"
	logr "github.com/go-logr/logr"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func ReconcileStatefulSet(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	owner metav1.Object,
	clusterName string,
	namespace string,
	pool elasticsearchv1beta1.PoolSpec,
	extraLabels map[string]string,
) (reconcile.Result, error) {
	var err error

	// statefulsets require a headless service.
	if res, err := ReconcileHeadlessServiceForStatefulSet(
		client,
		scheme,
		log,
		owner,
		clusterName,
		namespace,
		pool,
	); err != nil {
		return res, err
	}

	name := StatefulSetName(clusterName, pool.Name)
	var storageClass *string
	if pool.StorageClass != "" {
		storageClass = &pool.StorageClass
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    extraLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &pool.Replicas,
			ServiceName: StatefulSetService(clusterName, pool.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"statefulSet": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"statefulSet": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      "nginx",
							Image:     "nginx",
							Resources: pool.Resources,
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"true"},
									},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: VolumeNameTemplate(clusterName),
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("256Mi"),
							},
						},
						StorageClassName: storageClass,
					},
				},
			},
		},
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, statefulSet, scheme); err != nil {
			return reconcile.Result{}, err
		}
	}

	found := &appsv1.StatefulSet{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
		err = client.Create(context.TODO(), statefulSet)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(statefulSet.Spec, found.Spec) {
		found.Spec = statefulSet.Spec
		log.Info("Updating StatefulSet", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
		err = client.Update(context.TODO(), found)
		if err != nil {

			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func ReconcileHeadlessServiceForStatefulSet(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	owner metav1.Object,
	clusterName string,
	namespace string,
	pool elasticsearchv1beta1.PoolSpec,
) (reconcile.Result, error) {
	var err error

	name := StatefulSetService(clusterName, pool.Name)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{Port: 9200},
			},
			Selector: map[string]string{
				"statefulSet": StatefulSetName(clusterName, pool.Name),
			},
		},
	}

	if owner != nil {
		if err := controllerutil.SetControllerReference(owner, svc, scheme); err != nil {
			return reconcile.Result{}, err
		}
	}

	found := &corev1.Service{}
	err = client.Get(context.TODO(), types.NamespacedName{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}, found)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Service", "namespace", svc.Namespace, "name", svc.Name)
		err = client.Create(context.TODO(), svc)
		return reconcile.Result{}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(svc.Spec, found.Spec) {
		found.Spec = svc.Spec
		log.Info("Updating Svc", "namespace", svc.Namespace, "name", svc.Name)

		err = client.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
