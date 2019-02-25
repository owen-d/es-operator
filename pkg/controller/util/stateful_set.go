package util

import (
	"context"
	"fmt"
	logr "github.com/go-logr/logr"
	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"path/filepath"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	dataVolumeMountPath       = "/usr/share/elasticsearch/data"
	configVolumeMountPath     = "/usr/share/elasticsearch/config/elasticsearch.yml"
	schedulingVolumeMountPath = "/usr/share/elasticsearch/scheduling"
	elasticConfigFile         = "elasticsearch.yml"
	esImage                   = "docker.elastic.co/elasticsearch/elasticsearch"
	esTag                     = "6.6.1"
	reloaderImage             = "owend/es-sidecar"
	reloaderTag               = "latest"
	maxMapCount               = 262144
)

// GroupID for the elasticsearch user. The official elastic docker images always have the id of 1000
var esFsGroup int64 = 1000

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

	statefulLabels := map[string]string{StatefulSetKey: name}
	podLabels := MergeMaps(statefulLabels, extraLabels)

	podEnv := mkEnv(clusterName, namespace, pool)

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
				MatchLabels: statefulLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: &esFsGroup,
					},
					InitContainers: mkInitContainers(CatImage(esImage, esTag), maxMapCount),
					Containers: []corev1.Container{
						{
							Name:      "elasticsearch",
							Image:     CatImage(esImage, esTag),
							Resources: pool.Resources,
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"true"},
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 3,
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"true"},
									},
								},
							},
							Env: podEnv,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      DataVolumeNameTemplate(clusterName, pool.Name),
									MountPath: dataVolumeMountPath,
									ReadOnly:  false,
								},
								{
									Name:      QuorumConfigMapName(clusterName),
									MountPath: configVolumeMountPath,
									SubPath:   elasticConfigFile,
									ReadOnly:  true,
								},
								{
									Name:      PoolSchedulableConfigMapName(clusterName, pool.Name),
									MountPath: schedulingVolumeMountPath,
									ReadOnly:  true,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										// lock memory
										"IPC_LOCK",
										// override resource limits
										"SYS_RESOURCE",
									},
								},
							},
						},
						mkReloader(
							QuorumConfigMapName(clusterName),
							// TODO: confirm
							// mount entire configmap as there's unusual behavior (not notifying)
							// when mounting just single files
							filepath.Dir(configVolumeMountPath),
							configVolumeMountPath,
						),
					},
					Volumes: []corev1.Volume{
						{
							Name: QuorumConfigMapName(clusterName),
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: QuorumConfigMapName(clusterName),
									},
								},
							},
						},
						{
							Name: PoolSchedulableConfigMapName(clusterName, pool.Name),
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: PoolSchedulableConfigMapName(clusterName, pool.Name),
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
						Name: DataVolumeNameTemplate(clusterName, pool.Name),
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("512Mi"),
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
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{Port: 9200},
			},
			Selector: map[string]string{
				StatefulSetKey: StatefulSetName(clusterName, pool.Name),
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

func mkEnv(clusterName string, namespace string, pool elasticsearchv1beta1.PoolSpec) []corev1.EnvVar {
	podEnv := []corev1.EnvVar{
		corev1.EnvVar{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		corev1.EnvVar{
			Name:  "NODE_MASTER",
			Value: fmt.Sprintf("%t", StringIn(elasticsearchv1beta1.MasterRole, pool.Roles...)),
		},
		corev1.EnvVar{
			Name:  "NODE_DATA",
			Value: fmt.Sprintf("%t", StringIn(elasticsearchv1beta1.DataRole, pool.Roles...)),
		},
		corev1.EnvVar{
			Name:  "NODE_INGEST",
			Value: fmt.Sprintf("%t", StringIn(elasticsearchv1beta1.IngestRole, pool.Roles...)),
		},
		corev1.EnvVar{
			Name:  "DISCOVERY_URL",
			Value: DiscoveryServiceDNS(clusterName, namespace),
		},
	}
	return podEnv
}

func mkInitContainers(image string, maxMapCount int) []corev1.Container {
	var user int64 = 0
	privileged := true

	reqs := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("25m"),
		corev1.ResourceMemory: resource.MustParse("128Mi"),
	}

	return []corev1.Container{
		{
			Name: "sysctl-conf",
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  &user,
				Privileged: &privileged,
			},
			Image:   image,
			Command: []string{"sysctl", "-w", fmt.Sprintf("vm.max_map_count=%d", maxMapCount)},
			Resources: corev1.ResourceRequirements{
				Requests: reqs,
				Limits:   reqs,
			},
		},
	}
}

func mkReloader(configMapName, mountPath, configFile string) corev1.Container {
	reqs := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("25m"),
		corev1.ResourceMemory: resource.MustParse("64Mi"),
	}

	return corev1.Container{
		Name:  "reloader",
		Image: CatImage(reloaderImage, reloaderTag),
		Args:  []string{"-v", "-c", configFile},
		Resources: corev1.ResourceRequirements{
			Requests: reqs,
			Limits:   reqs,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      configMapName,
				MountPath: mountPath,
				ReadOnly:  true,
			},
		},
	}
}
