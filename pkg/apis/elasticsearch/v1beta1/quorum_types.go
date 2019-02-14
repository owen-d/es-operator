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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// QuorumSpec defines the desired state of Quorum
type QuorumSpec struct {
	NodePools []PoolSpec `json:"nodePools,omitempty"`
	// Important: Run "make" to regenerate code after modifying this file
}

func (s *QuorumSpec) DesiredReplicas() (ct int32) {
	for _, pool := range s.NodePools {
		ct += pool.Replicas
	}
	return ct
}

// QuorumStatus defines the observed state of Quorum
type QuorumStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Ready maps pool names to the number of alive replicas.
	// This can include replicas that aren't in the spec (i.e. if a cluster updates and drops a node pool)
	ReadyPools map[string]int32
}

func (s *QuorumStatus) Ready() (ct int32) {
	for _, ready := range s.ReadyPools {
		ct += ready
	}
	return ct
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Quorum is the Schema for the quorums API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Quorum struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   QuorumSpec   `json:"spec,omitempty"`
	Status QuorumStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// QuorumList contains a list of Quorum
type QuorumList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Quorum `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Quorum{}, &QuorumList{})
}
