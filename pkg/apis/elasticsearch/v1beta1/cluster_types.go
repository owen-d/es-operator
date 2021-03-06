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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MasterRole = "master"
	DataRole   = "data"
	IngestRole = "ingest"
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
	NodePools []PoolSpec `json:"nodePools,omitempty"`
}

func (c *ClusterSpec) Pools() (masterPools, dronePools []PoolSpec) {
	for _, pool := range c.NodePools {

		if pool.IsMasterEligible() {
			masterPools = append(masterPools, pool)
		} else {
			dronePools = append(dronePools, pool)
		}
	}

	return masterPools, dronePools
}

type Persistence struct {
	Enabled      bool              `json:"enabled"`
	Size         resource.Quantity `json:"size,omitempty"`
	StorageClass string            `json:"storageClass,omitempty"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	// Ready maps pool names to the number of alive replicas.
	// This can include replicas that aren't in the spec (i.e. if a cluster updates and drops a node pool)
	DronePools map[string]*PoolSetMetrics `json:"dronePools,omitempty"`
}

func (s *ClusterStatus) ReadyReplicas() (ct int32) {
	for _, stats := range s.DronePools {
		ct += stats.Ready
	}
	return ct
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
