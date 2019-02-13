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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	masterRole = "master"
	dataRole   = "data"
	ingestRole = "ingest"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Cluster is the Schema for the clusters API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	NodePools []NodePool `json:"nodePools,omitempty"`
}

func (c *ClusterSpec) Pools() (masterPools, dronePools []NodePool) {
	for _, pool := range c.NodePools {
		var eligible bool
		for _, role := range pool.Roles {
			if role == masterRole {
				eligible = true
			}
		}

		if eligible {
			masterPools = append(masterPools, pool)
		} else {
			dronePools = append(dronePools, pool)
		}
	}

	return masterPools, dronePools
}

func (c *ClusterSpec) EligibleMasters() (res int32) {
	masterPools, _ := c.Pools()
	for _, pool := range masterPools {
		res += pool.Replicas
	}
	return res
}

type NodePool struct {
	Replicas int32  `json:"replicas,omitempty"`
	Name     string `json:"name"`
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

type Persistence struct {
	Enabled      bool              `json:"enabled"`
	Size         resource.Quantity `json:"size,omitempty"`
	StorageClass string            `json:"storageClass,omitempty"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
