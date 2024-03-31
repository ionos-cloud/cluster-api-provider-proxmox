# Development

This document describes how to get started with developing CAPMOX.

Table of contents
=================

* [About CAPMOX](#about-capmox)
* [CAPMOX Dependencies](#capmox-dependencies)
* [Getting Started](#getting-started)
    * [Proxmox VE API Token](#promox-api-token)
* [Setting up a development environment](#how-to-setup-a-development-environment)
    * [Running Tilt](#running-tilt)
* [Make Targets](#make-targets)
    * [Modifying API Definitions](#modifying-api-definitions)
* [Manual Capmox Setup](#manual-capmox-setup)
    * [Deploying CAPMOX](#deploying-capmox-to-kind)
    * [Running CAPMOX](#running-capmox)
    * [Uninstalling CAPMOX](#uninstalling-capmox)

## About CAPMOX
CAPMOX is a Kubernetes Cluster API provider for Proxmox Virtual Environment.

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.


## CAPMOX dependencies

The following dependencies are required to setup a development environment:

- git
- make
- Go v1.21 (newer versions break testing)
- Kubebuilder (only required for making new controllers)
- Docker (required for Kind)
- Tilt
- Kubectl
- kind
- clusterctl

Our Makefile will set up the following dependencies automatically:
- [kustomize](https://cluster-api.sigs.k8s.io/tasks/using-kustomize)
- [controller-gen](https://book.kubebuilder.io/reference/controller-gen)
- [setup-envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/tools/setup-envtest)

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [kind](https://sigs.k8s.io/kind) to get a local cluster for testing or run against a remote cluster.
It is possible to substitute Docker with [Podman](https://kind.sigs.k8s.io/docs/user/rootless/), and kind with minikube but various assumptions
in cluster-api's Tiltfile do not hold. We strongly advise against this approach.

**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

If you're having trouble setting any of this up, check the [Troubleshooting](Troubleshooting.md) docs.

### Proxmox VE API Token
Cluster-api-provider-proxmox requires a running Proxmox VE instance and an API token for access. See the [Proxmox wiki](https://pve.proxmox.com/wiki/Proxmox_VE_API#API_Tokens)
for more information.

## How to setup a development environment

### Running Tilt
- Create a directory and cd into it
- Clone [cluster-api](https://github.com/kubernetes-sigs/cluster-api)
- Clone [cluster-api-ipam-provider-in-cluster](https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster)
- Clone [cluster-api-provider-proxmox](https://github.com/ionos-cloud/cluster-api-provider-proxmox)

- You should now have a directory containing the following git repositories:
```
./cluster-api
./cluster-api-ipam-provider-in-cluster
./cluster-api-provider-proxmox
```

- Change directory to cluster-api: `cd cluster-api`. This directory is the working directory for Tilt.
- Create a file called `tilt-settings.json` with the following contents:

```json
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
```
  This file instructs Tilt to use the cluster-api-provider-proxmox and ipam-provider-in-cluster repositories. `allowed_contexts` is used to add
  allowed clusters other than kind (which is always implicitly enabled).

- Change directory to cluster-api-ipam-provider-in-cluster `cd ../cluster-api-ipam-provider-in-cluster`.
- Reset the git repository to `1d4735`: `git reset --hard 1d4735`. This is the last commit that works with Cluster API v1.6 and Go v1.20.

- If you don't have a cluster, create a new kind cluster:
```
kind create cluster --name capi-test
```
- cluster-api-provider-proxmox uses environment variables to connect to Proxmox VE. These need to be set in the shell which spawns Tilt.
  Tilt will pass these to the respective Kubernetes pods created. All variables are documented in `../cluster-api-provider-proxmox/envfile.example`.
  Copy `../cluster-api-provider-proxmox/envfile.example` to `../cluster-api-provider-proxmox/envfile` and make changes pertaining to your configuration.
  For documentation on environment variables, see [usage](Usage.md#environment-variables)

- If you already had a kind cluster, add this line to `../cluster-api-provider-proxmox/envfile`:
```
CAPI_KIND_CLUSTER_NAME=<yourclustername>
```

- Start tilt with the following command (with CWD still being ./cluster-api):
```
. ../cluster-api-provider-proxmox/envfile && tilt up
```

Press the **space** key to open the Tilt web interface in your browser. Check that all resources turn green and there are no warnings.
You can click on (Tiltfile) to see all the resources.

> **Congratulations** you now have CAPMOX running via Tilt. If you make any code changes you should see that CAPMOX is automatically rebuilt.
For help deploying your first cluster with CAPMOX, see [usage](Usage.md).

## Make targets

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

### Modifying API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

## Manual CAPMOX setup

### Deploying CAPMOX to kind
1. Install CAPMOX's Custom Resources into kind:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/cluster-api-provider-proxmox:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/cluster-api-provider-proxmox:tag
```

### Running CAPMOX
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Uninstalling CAPMOX
To delete the CRDs from the cluster:

```sh
make uninstall
```

To undeploy the controller from the cluster:

```sh
make undeploy
```
