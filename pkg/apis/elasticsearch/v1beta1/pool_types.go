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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PoolSpec defines the desired state of Pool
type PoolSpec struct {
	ClusterName string `json:"clusterName,omitempty"`
	Replicas    int32  `json:"replicas,omitempty"`
	Name        string `json:"name"`
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
	// TODO: allow choosing of own image/versions for es
}

// PoolStatus defines the observed state of Pool
type PoolStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Pool is the Schema for the pools API
// +k8s:openapi-gen=true
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PoolSpec   `json:"spec,omitempty"`
	Status PoolStatus `json:"status,omitempty"`
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
