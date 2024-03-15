#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail
if [[ "${TRACE-0}" == "1" ]]; then
    set -o xtrace
fi

if [[ ! -d ../cluster-api ]]; then
    echo "Cloning missing CAPI provider"
    git clone --depth=1 --branch=main https://github.com/kubernetes-sigs/cluster-api ../cluster-api
fi

if [[ ! -d ../cluster-api-ipam-provider-in-cluster ]]; then
    echo "Cloning missing IPAM provider"
    git clone --depth=1 --branch=main https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster ../cluster-api-ipam-provider-in-cluster
fi

if [[ ! -f ../cluster-api/tilt-settings.json ]]; then
    cat > ../cluster-api/tilt-settings.json <<EOM
{
  "default_registry": "ghcr.io/ionos-cloud",
  "provider_repos": ["../cluster-api-provider-proxmox/", "../cluster-api-ipam-provider-in-cluster/"],
  "enable_providers": ["ipam-in-cluster", "proxmox", "kubeadm-bootstrap", "kubeadm-control-plane"],
  "allowed_contexts": ["minikube"],
  "kustomize_substitutions": {},
  "extra_args": {
    "proxmox": ["--v=4"]
  }
}
EOM
fi

if ! kind get clusters | grep -q capi-test; then
    kind create cluster --name capi-test
fi

# check that required environment variables to start capmox are set
[[ -n ${PROXMOX_URL-} ]] || { echo "PROXMOX_URL is not set" >&2; exit 1; }
[[ -n ${PROXMOX_TOKEN-} ]] || { echo "PROXMOX_TOKEN is not set" >&2; exit 1; }
[[ -n ${PROXMOX_USERNAME} ]] || { echo "PROXMOX_USERNAME is not set" >&2; exit 1; }

export EXP_CLUSTER_RESOURCE_SET=true

cd ../cluster-api
tilt up
