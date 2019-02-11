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

// import (
// 	elasticsearchv1beta1 "github.com/owen-d/es-operator/pkg/apis/elasticsearch/v1beta1"
// 	"k8s.io/api/core/v1"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// )

// // see these for reference:
// // https://www.elastic.co/guide/en/elasticsearch/reference/current/docker.html
// // https://www.elastic.co/guide/en/elasticsearch/reference/current/important-settings.html
// type ElasticConfig struct {
// 	ClusterName  string
// 	MinMasters   int32
// 	nodeSettings map[string]string
// 	// --ulimit nofile=65536:65536
// 	// -e "bootstrap.memory_lock=true" --ulimit memlock=-1:-1
// }

// func NewElasticConfig(cluster elasticsearchv1beta1.Cluster, nodePool elasticsearchv1beta1.NodePool) ElasticConfig {
// 	conf := ElasticConfig{
// 		nodeSettings: map[string]string{
// 			"bootstrap.memory_lock": "true",
// 		},
// 	}
// }
