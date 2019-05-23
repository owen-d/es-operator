# es-operator
Elasticsearch operator for Kubernetes

### Warning
This is a proof of concept shown at [DC Hack & Tell](https://dc.hackandtell.org/). It is not ready for production use.

### What
This is a [Kubernetes Operator](https://coreos.com/operators/) for Elasticsearch. It provides a declarative interface for managing Elasticsearch clusters and takes care of handling state transitions for you.
### Why
[StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) are a nice addition to Kubernetes and lay the groundwork for building stateful applications on top of Kubernetes. Unfortunately, they only provide a _top down_ view of stateful applications and provide tools for migrating volumes alongside containers. In the case of many distributed systems, this is only one failure condition.
#### Elasticsearch
Scaling Elasticsearch can be tricky. At a high level you need to:
- Ensure that `minimum_master_nodes` is always set to the appropriate size for your cluster's quorum
- Ensure that data loss (even temporary via relocating a shard with no replicas) is avoided

To this end, we attempt to fully drain a node of it's shards before de-registering it.
Configurations are proxied to pods via [ConfigMaps](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/), removing the need for pods to have read privileges to the kubernetes api.


### Design
#### Custom Resource Definitions
##### Cluster
A cluster with various Node pools, each given a name and a selection of [roles](https://www.elastic.co/guide/en/elasticsearch/reference/current/modules-node.html).
This is the only type which should be specified. All others are created/updated/removed programmatically via the controller manager.
##### Quorum
This is not to be handled by the end-user, but specifies a generated quorum definition based on master-eligible node pools in a cluster spec.
##### Node Pools
This is not to be handled by the end-user outside of subfields in the Cluster spec, but specifies a pool of elasticsearch nodes.

### Example
```yaml
apiVersion: elasticsearch.k8s.io/v1beta1
kind: Cluster
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: mycluster
spec:
  nodePools:
    - name: master
      roles:
        - master
      replicas: 3
      storageClass: "standard"
      resources:
        requests:
          memory: "512Mi"
          cpu: "200m"
    - name: data
      roles:
        - data
      replicas: 5
      storageClass: "standard"
      resources:
        requests:
          memory: "512Mi"
          cpu: "200m"
```


### Regular operations
Most actions are handled via `make`
- `make build` - runs code generation
- `make install` - generates [custom resource definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) and installs them into current cluster via `kubectl`
- `make manager` - builds the controller manager
- `make sidecar` - build executable which monitors and proxies `minimum_master_nodes` settings to a cluster
- `make handler` - builds the executable which monitors schedulability of an elasticsearch node and deregisters (terminates) it when applicable
- `make seed` - build elasticsearch seeding tool (for seeding dummy data)
- `make run` - runs the controller manager against current k8s installation (most likely [minikube](https://kubernetes.io/docs/setup/minikube/))
