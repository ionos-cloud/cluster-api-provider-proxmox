
# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.30.0

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	go tool controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	go tool controller-gen object paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run lint.
	go tool golangci-lint run

# Package names to test
WHAT ?= ./...

## Location for envtest binary assets
LOCALBIN ?= $(shell pwd)/bin

.PHONY: test
test: manifests generate fmt vet ## Run tests. Specify packages to test using WHAT.
	KUBEBUILDER_ASSETS="$(shell go tool setup-envtest use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $(WHAT) -coverprofile cover.out

.PHONY: mockgen
mockgen: ## Generate mocks.
	go tool mockery

.PHONY: yamlfmt
yamlfmt: ## Run yamlfmt against yaml.
	go tool yamlfmt -dry -quiet
	go tool yamlfmt

.PHONY: tidy
tidy: ## Run go mod tidy to ensure modules are up to date
	go mod tidy

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go --enable-webhooks=false --v=4

# If you wish built the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64 ). However, you must enable Docker BuildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: test ## Build Docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push Docker image with the manager.
	docker push ${IMG}

# PLATFORMS defines the target platforms for  the manager image be build to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image for your registry (i.e. if you do not inform a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To properly provided solutions that supports more than one platform you should use this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: test ## Build and push Docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- docker buildx rm project-v3-builder
	rm Dockerfile.cross

##@ verify

.PHONY: verify
verify: verify-modules verify-gen verify-versions ## verify the manifests and the code.

.PHONY: verify-modules
verify-modules: tidy ## Verify go modules are up to date
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		git diff; \
		echo "go module files are out of date"; exit 1; \
	fi
	@if (find . -name 'go.mod' | xargs -n1 grep -q -i 'k8s.io/client-go.*+incompatible'); then \
		find . -name "go.mod" -exec grep -i 'k8s.io/client-go.*+incompatible' {} \; -print; \
		echo "go module contains an incompatible client-go version"; exit 1; \
	fi

.PHONY: verify-gen
verify-gen: generate manifests mockgen ## Verify go generated files and CRDs are up to date
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate and/or make mockgen"; exit 1; \
	fi

.PHONY: verify-versions
verify-versions: ## Verify that versions of special dependencies are consistent across the repository.
	hack/verify-versions.sh


##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	go tool kustomize build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	go tool kustomize build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && go tool kustomize edit set image controller=${IMG}
	go tool kustomize build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	go tool kustomize build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Test

SHELLSPEC_ARGS ?=

.PHONY: test-sh
test-sh: ## Run ShellSpec tests for hack/ scripts with kcov coverage.
	cd hack/spec && shellspec --kcov --kcov-options='--include-path=$(CURDIR)/hack/' $(SHELLSPEC_ARGS)

.PHONY: tilt-up
tilt-up: ## Start Tilt in a kind cluster.
	hack/start-capi-tilt.sh

##@ Helpers
HELM ?= helm

CILIUM_VERSION ?= 1.14.1

.PHONY: crs-cilium
crs-cilium: ## Generates crs manifests for Cilium.
	$(HELM) repo add cilium https://helm.cilium.io/ --force-update
	$(HELM) template cilium cilium/cilium --version $(CILIUM_VERSION) --set internalTrafficPolicy=local --namespace kube-system > templates/crs/cni/cilium.yaml

CALICO_VERSION ?= v3.28.2

.PHONY: crs-calico
crs-calico: ## Generates crs manifests for Calico.
	curl -o templates/crs/cni/calico.yaml https://raw.githubusercontent.com/projectcalico/calico/$(CALICO_VERSION)/manifests/calico.yaml

METALLB_VERSION ?= 0.14.4
FRR_K8S_DIR = metallb/charts/metallb/charts/frr-k8s/templates
LB_TOLERATIONS = [{"key": "node-role.kubernetes.io/load-balancer", "operator": "Exists", "effect": "NoSchedule"}]
CP_TOLERATIONS = [{"key": "node-role.kubernetes.io/control-plane", "operator": "Exists", "effect": "NoSchedule"}]
FRR_NODESELECTOR = {"node-role.kubernetes.io/load-balancer": ""}
.PHONY: crs-metallb
crs-metallb: ## Generates crs manifests for MetalLB.
	$(HELM) repo add metallb https://metallb.github.io/metallb
	$(HELM) template metallb metallb/metallb --version $(METALLB_VERSION) \
			--set frrk8s.enabled=true,speaker.frr.enabled=false \
			--set-json 'controller.tolerations=$(CP_TOLERATIONS)' \
			--set-json 'speaker.tolerations=$(LB_TOLERATIONS)' \
			--set-json 'frr-k8s.frrk8s.tolerations=$(LB_TOLERATIONS)' \
			--set-json 'frr-k8s.frrk8s.nodeSelector=$(FRR_NODESELECTOR)' \
			--namespace=metallb-system > templates/crs/metallb.yaml


##@ Release
## --------------------------------------
## Release
## --------------------------------------

REPOSITORY ?= ghcr.io/ionos-cloud/cluster-api-provider-proxmox
RELEASE_DIR ?= out
RELEASE_VERSION ?= v0.0.1

.PHONY: release-manifests
RELEASE_MANIFEST_SOURCE_BASE ?= config/default
release-manifests: ## Create kustomized release manifest in $RELEASE_DIR (defaults to out).
	@mkdir -p $(RELEASE_DIR)
	cp metadata.yaml $(RELEASE_DIR)/metadata.yaml
	## change the image tag to the release version
	cd $(RELEASE_MANIFEST_SOURCE_BASE) && go tool kustomize edit set image $(REPOSITORY):$(RELEASE_VERSION)
	## generate the release manifest
	go tool kustomize build $(RELEASE_MANIFEST_SOURCE_BASE) > $(RELEASE_DIR)/infrastructure-components.yaml

.PHONY: release-templates
release-templates: ## Generate release templates
	@mkdir -p $(RELEASE_DIR)
	cp templates/cluster-template*.yaml $(RELEASE_DIR)/


##@ e2e
## --------------------------------------
## E2e tests
## --------------------------------------
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
E2E_CONF_FILE ?= $(ROOT_DIR)/test/e2e/config/proxmox-ci.yaml
E2E_CONF_FILE_ENVSUBST := $(ROOT_DIR)/test/e2e/config/proxmox-ci-envsubst.yaml
E2E_DATA_DIR ?= $(ROOT_DIR)/test/e2e/data
KUBETEST_CONF_PATH ?= $(abspath $(E2E_DATA_DIR)/kubetest/conformance.yaml)


# Allow overriding the e2e configurations
GINKGO_FOCUS ?= Workload cluster creation
GINKGO_SKIP ?= Generic|Flatcar
GINKGO_NODES ?= 1
GINKGO_NOCOLOR ?= false
GINKGO_ARGS ?=
GINKGO_TIMEOUT ?= 2h
GINKGO_POLL_PROGRESS_AFTER ?= 10m
GINKGO_POLL_PROGRESS_INTERVAL ?= 1m
ARTIFACTS ?= $(ROOT_DIR)/_artifacts
SKIP_CLEANUP ?= false
SKIP_CREATE_MGMT_CLUSTER ?= false

.PHONY: e2e-image
e2e-image:
	docker build --tag="$(REPOSITORY):e2e" .

.PHONY: test-e2e
test-e2e: e2e-image ## Run the end-to-end tests
	go tool envsubst < $(E2E_CONF_FILE) > $(E2E_CONF_FILE_ENVSUBST) && \
	time go tool ginkgo -v --trace -poll-progress-after=$(GINKGO_POLL_PROGRESS_AFTER) -poll-progress-interval=$(GINKGO_POLL_PROGRESS_INTERVAL) \
	--tags=e2e --focus="$(GINKGO_FOCUS)" -skip="$(GINKGO_SKIP)" --nodes=$(GINKGO_NODES) --no-color=$(GINKGO_NOCOLOR) \
	--timeout=$(GINKGO_TIMEOUT) --output-dir="$(ARTIFACTS)" --junit-report="junit.e2e_suite.1.xml" --fail-fast  $(GINKGO_ARGS) ./test/e2e -- \
		-e2e.artifacts-folder="$(ARTIFACTS)" \
		-e2e.config="$(E2E_CONF_FILE_ENVSUBST)" \
		-e2e.skip-resource-cleanup=$(SKIP_CLEANUP) \
		-e2e.use-existing-cluster=$(SKIP_CREATE_MGMT_CLUSTER) $(E2E_ARGS)

CONFORMANCE_E2E_ARGS ?= -kubetest.config-file=$(KUBETEST_CONF_PATH)
CONFORMANCE_E2E_ARGS += $(E2E_ARGS)
.PHONY: test-conformance
test-conformance: ## Run conformance test on workload cluster.
	$(MAKE) test-e2e GINKGO_FOCUS="Conformance Tests" E2E_ARGS='$(CONFORMANCE_E2E_ARGS)' GINKGO_ARGS='$(LOCAL_GINKGO_ARGS)'
