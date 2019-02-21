
# Image URL to use all building/pushing image targets
IMG ?= owend/es-controller:latest
SIDECAR_IMG ?= owend/es-sidecar:latest

all: test manager

ping:
	curl $$(minikube ip):$$(kubectl get svc mycluster -o jsonpath='{.spec.ports[0].nodePort}')/_cluster/health | jq '.'

clean-pvc:
	kubectl get pvc | tail -n +2 | awk '{print $$1}' | xargs -n 1 kubectl delete pvc

# Compile but don't run tests
build: generate fmt vet

# Run tests
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager github.com/owen-d/es-operator/cmd/manager

sidecar:
	go build -o bin/reloader github.com/owen-d/es-operator/cmd/reloader

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	go generate ./pkg/... ./cmd/...

# Build the docker image
docker-build:
	docker build . -t ${IMG} -f controller.Dockerfile
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml
	docker build . -t ${SIDECAR_IMG} -f sidecar.Dockerfile

# Push the docker image
docker-push:
	docker push ${IMG}
	docker push ${SIDECAR_IMG}
