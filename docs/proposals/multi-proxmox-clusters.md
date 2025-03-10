---
title: Multi-Proxmox Clusters
authors:
  - "@mcbenjemaa"
reviewers:
  - "@mcbenjemaa"
  - "@wikkyk"
  - "@65278"
creation-date: 2024-12-19
last-updated: 2024-12-19
status: experimental
---

# Enable Multi-Proxmox Clusters

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals/Future Work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
    - [Story 3](#story-3)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
      - [ProxmoxCluster CRD](#proxmoxcluster-crd)
      - [ProxmoxMachine CRD](#proxmoxmachine-crd)
      - [Proxmox Client](#proxmox-client)
        - [The client interface](#the-client-interface)
        - [The client implementation](#the-client-implementation)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

- **CAPMOX**: Cluster API Provider for Proxmox
- **Proxmox**: Proxmox Virtual Environment

## Summary

Today, with Cluster API provider for proxmox it can provision Kubernetes clusters on a single proxmox cluster.
However, CAPMOX allows [setting a proxmox credentials for each cluster](https://github.com/ionos-cloud/cluster-api-provider-proxmox/pull/215),
but it does not allow provisioning Kubernetes clusters on multiple proxmox clusters.

With the feature of provisioning a single Kubernetes cluster on multiple proxmox clusters,
which will enable better resource utilization and high availability.

This proposal aims to enable provisioning Kubernetes clusters on multiple proxmox clusters.

## Motivation

Proxmox is a great virtualization platform for running Kubernetes clusters, as it provides a simple and easy way to manage virtual machines.
However, it is not possible to provision a single Kubernetes cluster on multiple proxmox clusters.

For the following reasons, it is important to enable provisioning Kubernetes clusters on multiple proxmox clusters:

- **Resource Utilization**: By provisioning a single Kubernetes cluster on multiple proxmox clusters, we can better utilize the resources of the proxmox clusters.
- **High Availability**: By provisioning a single Kubernetes cluster on multiple proxmox clusters, we can achieve high availability.

### Goals

- To design a solution for provisioning Kubernetes Clusters on multiple proxmox clusters.
- To distribute machines to various proxmox clusters.
- To enable high availability of Kubernetes clusters by provisioning them on multiple proxmox clusters.

### Non-Goals/Future Work

- To manage the lifecycle of the Proxmox clusters instances.
- To manage the security of the Proxmox clusters instances.
- To maintain the state of the Proxmox clusters instances.
- To provide the credentials for the Proxmox clusters instances.

## Proposal

This document introduces the concept of a Multi Proxmox Clusters provisioning, a new way to provision HA clusters.
This feature require the user to set a specific credentials and configuration in the Secret and ProxmoxCluster.


### User Stories

#### Story 1

As a developer or Cluster operator, I would like to Provision K8s Cluster on multiple Proxmox Clusters, 
so that I can better utilize the resources of the proxmox clusters.

#### Story 2

As a developer or Cluster operator, I would like CAPMOX to expose configurations to enable provisioning Kubernetes clusters on multiple proxmox clusters.

#### Story 3

As a developer or Cluster operator, I would like CAPMOX to use a specific schema for Proxmox credentials that is used to provision the Kubernetes cluster on multiple proxmox clusters.

#### Implementation Details/Notes/Constraints

The proposed solution is designed to make the provision of workload clusters as easy as possible while keeping backward compatible with the existing CAPMOX.

##### ProxmoxCluster CRD

We propose to introduce a new field `spec.settings` in ProxmoxCluster CRD.
The goal of this field is to determine which mode to use for provisioning the Kubernetes cluster.


```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: test
spec:
  settings:
    mode: Default
    instances:
      - name: pmox-txl
        nodes: [pmox-txl-1, pmox-txl-2]
        template:
          sourceNode: pmox-txl-1
          templateID: 1000
        credentialsRef:
          name: "pmox-txl-proxmox-credentials"
      - name: pmox-fra
        nodes: [pmox-fra-1, pmox-fra-2, pmox-fra-3]
        template:
          templateSelector:
            matchTags:
              - capi
              - v1.30.5
        credentialsRef:
          name: "pmox-fra-proxmox-credentials"
  controlPlaneEndpoint:
    host: 10.0.0.10
    port: 6443
  ipv4Config:
    addresses: [10.0.0.11-10.0.0.50]
    prefix: 24
    gateway: 10.0.0.1
  dnsServers: [10.0.0.1]
  allowedNodes: []
  credentialsRef:
    name: "test-proxmox-credentials"
```

The `spec.settings.mode` field will have the following values:
* `Default`: This is the default mode. It will provision the Kubernetes cluster on a single proxmox cluster.
* `SingleInstance`: This mode will provision the Kubernetes cluster on a single proxmox cluster.
* `MultiInstance`: This mode will provision the Kubernetes cluster on multiple proxmox clusters.

The `spec.settings.instances` field will have the list of proxmox clusters to provision the Kubernetes cluster on.

* `spec.settings.mode=Default`: if this mode is set and the `spec.settings.instances` field is not set, the Kubernetes cluster will be provisioned on the default proxmox cluster,
  and when instances is set it will provision the Kubernetes cluster on the first proxmox instance.

* `spec.settings.mode=SingleInstance`: if this mode is set, the Kubernetes cluster will be provisioned on a single proxmox cluster chosen randomly from the list of proxmox clusters in the `spec.settings.instances` field.

* `spec.settings.mode=MultiInstance`: if this mode is set, the Kubernetes cluster will be provisioned on multiple proxmox clusters from the list of proxmox clusters in the `spec.settings.instances` field.

Notes:

* `spec.allowedNodes` can be deprecated and have no effect.
* `spec.credentialsRef` will be used only for the default proxmox cluster (can be deprecated in future). 
* If no `mode` is set the Cluster will be provisioned on the default proxmox cluster.

This will make sure to distribute the machines to the different proxmox clusters.

##### ProxmoxMachine CRD

First, in order to enable multi cluster deployment, we need the feature of selecting the template by tags https://github.com/ionos-cloud/cluster-api-provider-proxmox/pull/343
The tags, should match tags set on VM templates in Proxmox for all instances.

In order to have a way of manually setting an instance in machine level for example, when using a MachineDeployment.
we can add the field `instance` to the ProxmoxMachine CRD.

```yaml
kind: ProxmoxMachineTemplate
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
metadata:
  name: "test-worker-1"
spec:
  template:
    spec:
      templateSelector:
        matchTags:
          - capi
          - v1.30.5
      instance: pmox-txl
```

##### Proxmox Client

The new model of having multiple proxmox clusters will require changes in the Proxmox client to support multiple proxmox clusters.

###### The client interface

The client interface will stay the same, any required changes will be adapted.

###### The client implementation

The api client struct will now have a map of clients instead of a single client.
The map will have the name of the proxmox cluster as the key and the client as the value.

```go
// APIClient Proxmox API client object.
type APIClient struct {
	clients map[string]*proxmox.Client
	logger logr.Logger
}
```

* In default mode, the client will use the default client key.

The function `NewAPIClient()` will be initialized in the controller.

The controller will determine which instance to use and provide the name to the APIClient.
The APIClient functions should have a param that specify which instance to use, based on instance name.

```go

// selectNextInstance selects a client based on the request.
func (c *APIClient) CloneVM(, templateID int, clone capmox.VMCloneRequest, instance string) (capmox.VMCloneResponse, error) {

}
```


**State**
The proxmox instance will be saved in ProxmoxMachine as `spec.instance`.
The state of distributing the machines will be saved in the ProxmoxCluster as `status.instances`.


### Security Model

This proposal will add a new fields to ProxmoxCluster and ProxmoxMachine CRDs.
The permissions will stay the same.
This feature will also may require the user to manage multiple secrets.

### Risks and Mitigations

* The new feature introduce new secrets for proxmox credentials.

## Alternatives

The alternative is to use the existing CAPMOX and provision the Kubernetes cluster on a single proxmox cluster.

## Upgrade Strategy

since there are changes within the API and/or deprecations, we need to create a new version. 
`v1alpha2` or `v1alpha3` will be the new version of the ProxmoxCluster and ProxmoxMachine CRDs.

CAPMOX will support multiple versions at the same time.
only when the end of life of the the new version must be communicated with the users.

## Additional Details

### Test Plan 

- Unit test coverage for the Proxmox instance selection and distributions.
- E2e tests if possible, to test the distribution of the machines to the different proxmox clusters.

### Graduation Criteria

This feature will be released in a new version CRD `v1alpha3` to CAPMOX.

## Implementation History

- [X] 2024-12-19: Open proposal PR
- [ ] MM/DD/YYYY: Present proposal at a community meeting
