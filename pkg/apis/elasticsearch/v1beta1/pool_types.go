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

package v1beta1

import (
	"fmt"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PoolSpec defines the desired state of Pool
type PoolSpec struct {
	Replicas      int32  `json:"replicas"`
	Unschedulable []int  `json:"unschedulableIndices,omitempty"`
	Name          string `json:"name"`
	// +kubebuilder:validation:Enum=master,data,ingest
	Roles     []string                `json:"roles,omitempty"`
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Persistence  Persistence             `json:"persistence,omitempty"`
	NodeSelector v1.NodeSelector `json:"nodeSelector,omitempty"`
	StorageClass string          `json:"storageClass,omitempty"`
	// TODO: add secret mounts
	// TODO: add es configs
	// TODO: add configMap mounts
	// TODO: affinity/antiaffinity
	// TODO: ensure spreading across AZs
}

func (s *PoolSpec) IsMasterEligible() bool {
	for _, role := range s.Roles {
		if role == MasterRole {
			return true
		}
	}
	return false
}

func (s *PoolSpec) ContainsUnschedulable(i int) bool {
	for _, x := range s.Unschedulable {
		if x == i {
			return true
		}
	}
	return false
}

// PoolStatus defines the observed state of Pool
type PoolStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ReadyReplicas maps statefulset names to the number of alive replicas.
	// This can include statefulsets that aren't in the spec (i.e. if a cluster updates and drops a node pool)
	StatefulSets map[string]*PoolSetMetrics `json:"statefulSets,omitempty"`

	// keeps string repr of pools for kubectl formatting.
	// TODO(owen): figure out how to format instead of adding a field that needs to be updated
	KubectlReplicasAnnotation string `json:"kubectlReplicasAnnotation,omitempty"`
}

type PoolSetMetrics struct {
	ResolvedName string `json:"resolvedName,omitempty"`
	Replicas     int32  `json:"replicas,omitempty"`
	Ready        int32  `json:"ready,omitempty"`
}

func (s *PoolStatus) ReadyReplicas() (ct int32) {
	for _, stats := range s.StatefulSets {
		ct += stats.Ready
	}
	return ct
}

func (s *PoolStatus) Replicas() (ct int32) {
	for _, stats := range s.StatefulSets {
		ct += stats.Replicas
	}
	return ct
}

func (s *PoolStatus) FormatReplicasAnnotation() string {
	return fmt.Sprintf("%d/%d", s.ReadyReplicas(), s.Replicas())
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Pool is the Schema for the pools API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ready",type="string",JSONPath=".status.kubectlReplicasAnnotation",description="nodes status for pool",format="byte",priority=0
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PoolSpec   `json:"spec,omitempty"`
	Status PoolStatus `json:"status,omitempty"`
}

func DesiredReplicas(specs []PoolSpec) (ct int32) {
	for _, pool := range specs {
		ct += pool.Replicas
	}
	return ct
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PoolList contains a list of Pool
type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pool{}, &PoolList{})
}
