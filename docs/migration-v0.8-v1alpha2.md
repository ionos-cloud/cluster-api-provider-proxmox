# Migration Guide: CAPMOX v0.7 → v0.8 (v1alpha2)

This guide will help you upgrade existing clusters from CAPMOX v0.7 (API v1alpha1) to
CAPMOX v0.8 (API v1alpha2).

## Table of Contents

- [Prerequisites](#prerequisites)
- [What's New in v0.8 (v1alpha2)](#whats-new-in-v08-v1alpha2)
- [Breaking Changes](#breaking-changes)
- [Automatic Conversion](#automatic-conversion)
- [Converting Manifest Files](#converting-manifest-files)
- [Migrating Your Manifests](#migrating-your-manifests)
  - [API Version and Cluster API References](#api-version-and-cluster-api-references)
  - [Integer Types](#integer-types)
  - [ProxmoxCluster](#proxmoxcluster)
  - [ProxmoxMachineTemplate](#proxmoxmachinetemplate)
  - [KubeadmControlPlane and KubeadmConfigTemplate](#kubeadmcontrolplane-and-kubeadmconfigtemplate)
- [ClusterClass Support](#clusterclass-support)
- [Full Before/After Example](#full-beforeafter-example)

---

## Prerequisites

- CAPMOX v0.7.x — we have not tested automated conversion from earlier releases,
  so upgrading to v0.7 before proceeding is recommended.
- Cluster API v1.11
- `clusterctl` updated to v1.11
- IPAM provider in-cluster v1.1.0-rc.1+ (earliest release with CAPI v1beta2 support), if using IP address management

## What's New in v0.8 (v1alpha2)

**Cluster API v1beta2 contract.** CAPMOX v0.8 supports the Cluster API v1.11's v1beta2
contract, bringing native Kubernetes conditions and the new initialization status
pattern. All Cluster API resource references switch from `apiVersion` to `apiGroup`.

**Networking refactor.** The separate `default` + `additionalDevices` network
configuration is replaced by a single `networkDevices` type. The primary device
is simply the one named `net0`. IP pool references are unified into a single `ipPoolRef`
list instead of the separate `ipv4PoolRef`/`ipv6PoolRef` fields.

**Zone-aware deployments.** CAPMOX v0.8 introduces first-class support for deploying
across multiple Proxmox zones. You can define per-zone IP pools, DNS servers, and
network configuration through the new `zoneConfigs` field on ProxmoxCluster.

**ClusterClass and ClusterTopology.** Full support for Cluster API's ClusterClass
pattern, allowing you to define reusable cluster blueprints with variable-driven
customization.

**Template selection improvements.** `sourceNode` is now optional when using a `templateSelector`,
and a new `matchPolicy` field supports three modes: `exact` (default, same as before —
requires an exact 1:1 tag match), `uniqueSubset` (template tags must contain all
specified tags), and `bestSubset` (selects the template with the fewest extra tags
beyond the specified set).
The `Target` field has been removed — use `allowedNodes` instead. 

**v1alpha1 deprecation.** The v1alpha1 API is deprecated and only supported through the
conversion webhook. All new manifests should target v1alpha2.

## Breaking Changes

The following changes affect how you write CAPMOX manifests. Existing resources stored
in the cluster are converted automatically (see [Automatic Conversion](#automatic-conversion)),
but any manifests, Helm charts, or GitOps templates you maintain **must be updated manually**.

### Summary

| Change | Impact |
|--------|--------|
| API version `v1alpha1` → `v1alpha2` | All CAPMOX resources |
| Cluster API `v1beta1` → `v1beta2` | All Cluster, KubeadmControlPlane, MachineDeployment, KubeadmConfigTemplate resources |
| Unsigned integer fields → signed integers | All CAPMOX resources with numeric fields |
| Network: `default` + `additionalDevices` → `networkDevices` list | ProxmoxMachineTemplate |
| IP pool refs: `ipv4PoolRef` / `ipv6PoolRef` → `ipPoolRef` list | ProxmoxMachineTemplate |
| `kubeletExtraArgs` format changed from map to name/value list | KubeadmControlPlane, KubeadmConfigTemplate |
| `CloneSpec` removed from ProxmoxCluster | ProxmoxCluster |
| `machineTemplate.infrastructureRef` moved into `machineTemplate.spec` | KubeadmControlPlane |

## Automatic Conversion

CAPMOX v0.8 serves both `v1alpha1` and `v1alpha2` through a conversion webhook.
When you upgrade the provider, **existing resources already stored in etcd are
converted automatically** — you do not need to manually edit live cluster resources.

However, be aware of a few things:

- **v1alpha2 is the storage version.** After upgrade, all resources are stored as
  v1alpha2 internally. You can still read them via the v1alpha1 API, but writes
  should target v1alpha2.
- **Lossy field conversions.** Some v1alpha2-only fields (like `zoneConfigs`, per-device
  `defaultIPv4`/`defaultIPv6`) have no v1alpha1 equivalent. If you read a resource
  through the v1alpha1 API, these fields will be absent. The provider preserves them
  through annotations during round-trip conversion.
- **Deprecated fields dropped.** `FailureReason` and `FailureMessage` on status
  objects are removed in v1alpha2, in line with the Cluster API v1beta2 contract
  which replaces these free-form fields with structured conditions. Conditions
  provide machine-readable status that is easier to monitor and aggregate.
- **Your YAML manifests, Helm values, and GitOps templates are NOT auto-converted.**
  You must update them to v1alpha2 as described below.

## Converting Manifest Files

The `convert` CLI tool automates the mechanical manifest changes listed in the
[Breaking Changes](#breaking-changes) summary. Run it on your manifest files before
making any manual edits.

### What it converts

- API versions: `v1alpha1` → `v1alpha2`, CAPI `v1beta1` → `v1beta2`
- Resource references: `apiVersion` → `apiGroup` (infrastructureRef, controlPlaneRef, configRef)
- `machineTemplate.infrastructureRef` → `machineTemplate.spec.infrastructureRef`
- `kubeletExtraArgs` map → name/value list
- `network.default` / `additionalDevices` → `network.networkDevices` list
- `ipv4PoolRef` / `ipv6PoolRef` → `ipPoolRef` list
- YAML comments and `${VARIABLE}` substitution expressions are preserved

### What requires manual work

- **`cloneSpec` decomposition** — if your ProxmoxCluster uses `cloneSpec`, the tool
  cannot split it into separate ProxmoxMachineTemplate and KubeadmConfigSpec resources.
  See [ProxmoxCluster → Removed: cloneSpec](#proxmoxcluster) below.

### Installation

```sh
make build-convert
```

This produces `bin/convert` and a `bin/clusterctl-capmox-convert` symlink
for use as a [clusterctl plugin](https://cluster-api.sigs.k8s.io/clusterctl/plugins).

### Usage

**Stdin/stdout** — convert a single file:

```sh
convert <cluster-template.yaml >cluster-template-v1alpha2.yaml
```

**In-place with backup** — overwrites the file, saving the original as `.bak`:

```sh
convert -f cluster-template.yaml -i.bak
```

**Multiple files:**

```sh
convert -f file1.yaml -f file2.yaml -i.bak
```

**As a clusterctl plugin** (requires `clusterctl-capmox-convert` in `$PATH`):

```sh
clusterctl capmox-convert -f cluster-template.yaml -i.bak
```

### Post-conversion review

- Manifest files should not contain `status` fields. The tool strips zero-value
  `status` blocks. If a `status` block with non-zero values is found, the tool
  emits a warning — this likely means the input was exported from a live
  resource rather than being a proper manifest template.
  You are strongly advised to remove any `status` fields from your templates.
- If you had `cloneSpec`, follow the manual migration in
  [ProxmoxCluster → Removed: cloneSpec](#proxmoxcluster).
- Run a diff against the original to verify that the output looks correct.

## Migrating Your Manifests

> **Tip:** If you already ran `convert`, the changes in this section are
> already applied. You can skip to
> [ProxmoxCluster → Removed: cloneSpec](#proxmoxcluster) if that applies to you.

### API Version and Cluster API References

Update all CAPMOX resource API versions and Cluster API references:

| Before (CAPMOX v0.7 v1alpha1/CAPI v1beta1) | After (CAPMOX v0.8 v1alpha2/CAPI v1beta2) |
|--------------------------------------------|-------------------------------------------|
| `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1` | `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2` |
| `apiVersion: cluster.x-k8s.io/v1beta1` | `apiVersion: cluster.x-k8s.io/v1beta2` |
| `apiVersion: controlplane.cluster.x-k8s.io/v1beta1` | `apiVersion: controlplane.cluster.x-k8s.io/v1beta2` |
| `apiVersion: bootstrap.cluster.x-k8s.io/v1beta1` | `apiVersion: bootstrap.cluster.x-k8s.io/v1beta2` | 

### Integer Types

All unsigned integer fields (`uint32`, `uint16`) in CAPMOX types have been changed to
their signed equivalents (`int32`, `int16`). This follows Kubernetes API best practices
— signed types produce cleaner OpenAPI schemas and behave more predictably during JSON
round-tripping and webhook conversion. This applies to fields like VRF `table`,
routing policy types, and other numeric configuration. YAML values do not need to
change, but be aware that the API now validates `table >= 1` for VRF configuration.

### ProxmoxCluster

The ProxmoxCluster spec is largely unchanged. The `controlPlaneEndpoint`, `ipv4Config`,
`ipv6Config`, `dnsServers`, and `allowedNodes` fields all remain the same.

**Removed: `cloneSpec`.** If you had a `cloneSpec` on your ProxmoxCluster, remove it
and move the machine configuration into your ProxmoxMachineTemplate resources instead.
Each entry in the old `cloneSpec.machineSpec` map becomes its own
ProxmoxMachineTemplate. The `sshAuthorizedKeys` and `virtualIPNetworkInterface` fields
move to `kubeadmConfigSpec` — SSH keys go under `users`, and the VIP interface is
set in the kube-vip manifest under `files`.

<table>
<tr>
<th>Before (v0.7) — <code>cloneSpec</code> on <code>ProxmoxCluster</code></th>
<th>After (v0.8) — split across separate resources</th>
</tr>
<tr>
<td>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: my-cluster
spec:
  controlPlaneEndpoint:
    host: 10.10.10.5
    port: 6443
  ipv4Config:
    addresses: [10.10.10.10-10.10.10.20]
    prefix: 24
    gateway: 10.10.10.1
  dnsServers: [8.8.8.8]
  allowedNodes: [pve1, pve2]
  cloneSpec:

    machineSpec:
      controlPlane:





        sourceNode: pve1
        templateID: 100
        numSockets: 2
        numCores: 4
        memoryMiB: 16384
        network:

          default:
            bridge: vmbr0







    sshAuthorizedKeys:
      - ssh-ed25519 AAAA...




    virtualIPNetworkInterface: eth1



```

</td>
<td>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxCluster
metadata:
  name: my-cluster
spec:
  controlPlaneEndpoint:
    host: 10.10.10.5
    port: 6443
  ipv4Config:
    addresses: [10.10.10.10-10.10.10.20]
    prefix: 24
    gateway: 10.10.10.1
  dnsServers: [8.8.8.8]
  allowedNodes: [pve1, pve2]
  # cloneSpec removed
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxMachineTemplate
metadata:
  name: my-cluster-control-plane
spec:
  template:
    spec:
      sourceNode: pve1
      templateID: 100
      numSockets: 2
      numCores: 4
      memoryMiB: 16384
      network:
        networkDevices:
        - name: net0
          bridge: vmbr0

---
kind: KubeadmControlPlane
spec:
  kubeadmConfigSpec:
    users:
    - name: root
      sshAuthorizedKeys:
        - ssh-ed25519 AAAA...
    files:
    - content: |
        # kube-vip manifest (abbreviated)
        env:
        - name: vip_interface
          value: "eth1"
      path: /etc/kubernetes/manifests/kube-vip.yaml
```

</td>
</tr>
</table>

### ProxmoxMachineTemplate

This is where the most significant changes are.

#### Network Configuration

The `network.default` and `network.additionalDevices` fields are replaced by a single
`network.networkDevices` list. Your primary network device becomes an entry named
`net0`. The separate `ipv4PoolRef` and `ipv6PoolRef` fields are replaced by a single
`ipPoolRef` list that holds both.

<table>
<tr>
<th>Before (v0.7)</th>
<th>After (v0.8)</th>
</tr>
<tr>
<td>

```yaml
spec:
  template:
    spec:
      network:
        default:
          bridge: vmbr0
        additionalDevices:
        - name: net1
          bridge: vmbr1
          ipv4PoolRef:
            apiGroup: ipam.cluster.x-k8s.io
            kind: GlobalInClusterIPPool
            name: ipv4-pool
          ipv6PoolRef:
            apiGroup: ipam.cluster.x-k8s.io
            kind: GlobalInClusterIPPool
            name: ipv6-pool
          dnsServers: [8.8.8.8]
```

</td>
<td>

```yaml
spec:
  template:
    spec:
      network:
        networkDevices:
        - name: net0
          bridge: vmbr0
        - name: net1
          bridge: vmbr1
          ipPoolRef:
          - apiGroup: ipam.cluster.x-k8s.io
            kind: GlobalInClusterIPPool
            name: ipv4-pool

          - apiGroup: ipam.cluster.x-k8s.io
            kind: GlobalInClusterIPPool
            name: ipv6-pool
          dnsServers: [8.8.8.8]
```

</td>
</tr>
</table>

### KubeadmControlPlane and KubeadmConfigTemplate

These are Cluster API (CAPI) resources, not CAPMOX-specific. The changes below come
from the CAPI v1beta1 → v1beta2 contract upgrade and apply to all providers, not just
CAPMOX.

#### kubeletExtraArgs format (CAPI v1beta2)

The `kubeletExtraArgs` field changed from a flat map to a list of name/value pairs:

<table>
<tr>
<th>Before (CAPI v1beta1)</th>
<th>After (CAPI v1beta2)</th>
</tr>
<tr>
<td>

```yaml
initConfiguration:
  nodeRegistration:
    kubeletExtraArgs:

      provider-id: "proxmox://'{{ ds.meta_data.instance_id }}'"
```

</td>
<td>

```yaml
initConfiguration:
  nodeRegistration:
    kubeletExtraArgs:
    - name: provider-id
      value: "proxmox://'{{ ds.meta_data.instance_id }}'"
```

</td>
</tr>
</table>

Apply this change to `initConfiguration`, `joinConfiguration`, and anywhere else
`kubeletExtraArgs` appears.

#### infrastructureRef location in KubeadmControlPlane (CAPI v1beta2)

The `infrastructureRef` moved one level deeper, under `machineTemplate.spec`.
Additionally, `apiVersion` is replaced by `apiGroup` in all resource references
throughout CAPI v1beta2:

<table>
<tr>
<th>Before (CAPI v1beta1)</th>
<th>After (CAPI v1beta2)</th>
</tr>
<tr>
<td>

```yaml
spec:
  machineTemplate:

    infrastructureRef:
      kind: ProxmoxMachineTemplate
      apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
      name: my-cluster-control-plane
```

</td>
<td>

```yaml
spec:
  machineTemplate:
    spec:
      infrastructureRef:
        kind: ProxmoxMachineTemplate
        apiGroup: infrastructure.cluster.x-k8s.io
        name: my-cluster-control-plane
```

</td>
</tr>
</table>

## ClusterClass Support

CAPMOX v0.8 adds full ClusterClass support. Instead of defining every resource
individually, you can reference a shared ClusterClass and override values through
topology variables.

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  topology:
    classRef:
      name: proxmox-clusterclass-v0.2.1
    version: 1.33.3
    controlPlane:
      replicas: 3
    workers:
      machineDeployments:
      - class: proxmox-worker
        name: worker-pool
        replicas: 3
      - class: proxmox-loadbalancer
        name: loadbalancer-pool
        replicas: 0
    variables:
    - name: controlPlaneEndpoint
      value:
        host: 10.10.10.9
        port: 6443
    - name: ipv4Config
      value:
        addresses: [10.10.10.10-10.10.10.20]
        gateway: 10.10.10.1
        prefix: 24
    - name: dnsServers
      value: [8.8.8.8, 8.8.4.4]
    - name: cloneSpec
      value:
        sshAuthorizedKeys: [ssh-ed25519 AAAA...]
        virtualIPNetworkInterface: ""
        machineSpec:
        - machineType: controlPlane
          sourceNode: pve1
          templateID: 100
          network:
            networkDevices:
            - name: net0
              bridge: vmbr0
        - machineType: worker
          sourceNode: pve1
          templateID: 100
          network:
            networkDevices:
            - name: net0
              bridge: vmbr0
        - machineType: loadBalancer
          sourceNode: pve1
          templateID: 100
          network:
            networkDevices:
            - name: net0
              bridge: vmbr0
```

Pre-built ClusterClass templates are included in the release:
- `cluster-class.yaml` — default (no CNI)
- `cluster-class-calico.yaml` — with Calico
- `cluster-class-cilium.yaml` — with Cilium

## Full Before/After Example

<table>
<tr>
<th>v0.7 (v1alpha1 / CAPI v1beta1)</th>
<th>v0.8 (v1alpha2 / CAPI v1beta2)</th>
</tr>
<tr>
<td>

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: my-cluster
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
    kind: ProxmoxCluster
    name: my-cluster
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1beta1
    kind: KubeadmControlPlane
    name: my-cluster-control-plane
```

</td>
<td>

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: ProxmoxCluster
    name: my-cluster
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: KubeadmControlPlane
    name: my-cluster-control-plane
```

</td>
</tr>
<tr>
<td>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: my-cluster
spec:
  controlPlaneEndpoint:
    host: 10.10.10.5
    port: 6443
  ipv4Config:
    addresses: [10.10.10.10-10.10.10.20]
    prefix: 24
    gateway: 10.10.10.1
  dnsServers: [8.8.8.8]
  allowedNodes: [pve1, pve2]
```

</td>
<td>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxCluster
metadata:
  name: my-cluster
spec:
  controlPlaneEndpoint:
    host: 10.10.10.5
    port: 6443
  ipv4Config:
    addresses: [10.10.10.10-10.10.10.20]
    prefix: 24
    gateway: 10.10.10.1
  dnsServers: [8.8.8.8]
  allowedNodes: [pve1, pve2]
```

</td>
</tr>
<tr>
<td>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxMachineTemplate
metadata:
  name: my-cluster-control-plane
spec:
  template:
    spec:
      sourceNode: pve1
      templateID: 100
      format: qcow2
      numSockets: 2
      numCores: 4
      memoryMiB: 16384
      disks:
        bootVolume:
          disk: scsi0
          sizeGb: 100
      network:
        default:

          bridge: vmbr0
```

</td>
<td>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: ProxmoxMachineTemplate
metadata:
  name: my-cluster-control-plane
spec:
  template:
    spec:
      sourceNode: pve1
      templateID: 100
      format: qcow2
      numSockets: 2
      numCores: 4
      memoryMiB: 16384
      disks:
        bootVolume:
          disk: scsi0
          sizeGb: 100
      network:
        networkDevices:
        - name: net0
          bridge: vmbr0
```

</td>
</tr>
<tr>
<td>

```yaml
kind: KubeadmControlPlane
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
metadata:
  name: my-cluster-control-plane
spec:
  replicas: 3
  machineTemplate:

    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
      kind: ProxmoxMachineTemplate
      name: my-cluster-control-plane
  kubeadmConfigSpec:
    initConfiguration:
      nodeRegistration:
        kubeletExtraArgs:
          provider-id: "proxmox://..."

    joinConfiguration:
      nodeRegistration:
        kubeletExtraArgs:
          provider-id: "proxmox://..."

  version: v1.33.3
```

</td>
<td>

```yaml
kind: KubeadmControlPlane
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
metadata:
  name: my-cluster-control-plane
spec:
  replicas: 3
  machineTemplate:
    spec:
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: ProxmoxMachineTemplate
        name: my-cluster-control-plane
  kubeadmConfigSpec:
    initConfiguration:
      nodeRegistration:
        kubeletExtraArgs:
        - name: provider-id
          value: "proxmox://..."
    joinConfiguration:
      nodeRegistration:
        kubeletExtraArgs:
        - name: provider-id
          value: "proxmox://..."
  version: v1.33.3
```

</td>
</tr>
</table>

### Change-by-Change Summary

| v0.7 (v1alpha1 / CAPI v1beta1) | v0.8 (v1alpha2 / CAPI v1beta2) | Affected Resources |
|---|---|---|
| `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1` | `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2` | All CAPMOX resources |
| `apiVersion: cluster.x-k8s.io/v1beta1` | `apiVersion: cluster.x-k8s.io/v1beta2` | Cluster, KubeadmControlPlane, KubeadmConfigTemplate, MachineDeployment |
| `apiVersion: controlplane.cluster.x-k8s.io/v1beta1` | `apiVersion: controlplane.cluster.x-k8s.io/v1beta2` | KubeadmControlPlane |
| `apiVersion: bootstrap.cluster.x-k8s.io/v1beta1` | `apiVersion: bootstrap.cluster.x-k8s.io/v1beta2` | KubeadmConfigTemplate |
| `infrastructureRef.apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1` | `infrastructureRef.apiGroup: infrastructure.cluster.x-k8s.io` | Cluster, KubeadmControlPlane |
| `controlPlaneRef.apiVersion: controlplane.cluster.x-k8s.io/v1beta1` | `controlPlaneRef.apiGroup: controlplane.cluster.x-k8s.io` | Cluster |
| `machineTemplate.infrastructureRef: ...` | `machineTemplate.spec.infrastructureRef: ...` | KubeadmControlPlane |
| `network.default: {bridge, model, ...}` | `network.networkDevices: [{name: net0, bridge, model, ...}]` | ProxmoxMachineTemplate |
| `network.additionalDevices: [{name: net1, ...}]` | additional entries in `network.networkDevices` | ProxmoxMachineTemplate |
| `ipv4PoolRef: {apiGroup, kind, name}` | `ipPoolRef: [{apiGroup, kind, name}, ...]` | ProxmoxMachineTemplate |
| `ipv6PoolRef: {apiGroup, kind, name}` | (merged into `ipPoolRef` list above) | ProxmoxMachineTemplate |
| `kubeletExtraArgs: {key: value}` | `kubeletExtraArgs: [{name: key, value: value}]` | KubeadmControlPlane, KubeadmConfigTemplate |
| `cloneSpec` on ProxmoxCluster | removed (use ProxmoxMachineTemplate / ProxmoxClusterTemplate) | ProxmoxCluster |

### Real-World Diffs

The following commits show the actual migration applied to this repository's own
templates and end-to-end tests:

- **Cluster templates and examples** — [`e655f26`](https://github.com/ionos-cloud/cluster-api-provider-proxmox/commit/e655f261e66064958c2f5f88212459dd5ea02521): ports all ClusterClass definitions, deployment templates, and example manifests to v1alpha2 / CAPI v1beta2.
- **E2E test templates** — [`016dad1`](https://github.com/ionos-cloud/cluster-api-provider-proxmox/commit/016dad18ed1a3f709c5f0930a2aa44e89a3054f0): ports the end-to-end test cluster templates (CI, Flatcar, upgrades) to v1alpha2 / CAPI v1beta2. This is a working, tested configuration.
