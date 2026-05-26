# Migration Guide: CAPMOX v0.8 → v0.9

This guide covers upgrading from CAPMOX v0.8 to v0.9. The main change in this release
is the bump to Cluster API v1.12 and controller-runtime v0.22. **There are no CAPMOX
API changes** — your existing v1alpha2 manifests work without modification.

## Table of Contents

- [Prerequisites](#prerequisites)
- [No Manifest Changes Required](#no-manifest-changes-required)
- [Breaking Changes from CAPI v1.12](#breaking-changes-from-capi-v112)
  - [AfterClusterUpgrade hook is now blocking](#afterclusterupgrade-hook-is-now-blocking)
  - [clusterctl move blocks paused clusters](#clusterctl-move-blocks-paused-clusters)
- [New Features in CAPI v1.12](#new-features-in-capi-v112)
  - [Additional lifecycle hooks](#additional-lifecycle-hooks)
- [Kubernetes Version Support Matrix](#kubernetes-version-support-matrix)
- [Bug Fixes Affecting Observed Behavior](#bug-fixes-affecting-observed-behavior)

---

## Prerequisites

- CAPMOX v0.8.x — direct upgrade from earlier releases is not tested.
- Cluster API v1.12
- `clusterctl` updated to v1.12
- IPAM provider in-cluster v1.1.0-rc.2+ (for CAPI v1beta2 support), if using IP address management

## No Manifest Changes Required

CAPMOX v0.9 continues to use the `v1alpha2` API and the CAPI `v1beta2` contract
introduced in v0.8. No changes to your ProxmoxCluster, ProxmoxMachine,
ProxmoxMachineTemplate, KubeadmControlPlane, MachineDeployment, or MachineHealthCheck
manifests are required as part of this upgrade.

If you are upgrading from v0.7 (v1alpha1 / CAPI v1beta1), follow the
[v0.7 → v0.8 migration guide](migration-v0.8-v1alpha2.md) first.

## Breaking Changes from CAPI v1.12

### AfterClusterUpgrade hook is now blocking

> **This is the most impactful change in v1.12. Review any lifecycle hook extensions
> you have deployed before upgrading.**

In CAPI v1.11, a failure in an `AfterClusterUpgrade` RuntimeExtension was logged
and the upgrade was allowed to complete. In CAPI v1.12, the hook is **blocking**:
a non-transient error from your extension halts the upgrade until the extension
succeeds or is removed.

**Who is affected:** Anyone who has deployed a RuntimeExtension that handles the
`AfterClusterUpgrade` hook.

**What to do:**
1. Review your extension's `AfterClusterUpgrade` handler and ensure it returns
   `Status: Success` once the post-upgrade work is done.
2. Ensure your extension is reachable from the management cluster at all times during
   an upgrade — an extension that returns connection errors will also block.
3. If you use the hook for best-effort cleanup and are comfortable with failures being
   ignored, remove the `AfterClusterUpgrade` hook registration from your
   `ExtensionConfig`, or update the handler to always return `Status: Success`.

### clusterctl move blocks paused clusters

`clusterctl move` now returns an error if the source Cluster or any of its
ClusterClasses are paused. Previously, paused resources were silently skipped during
the move, which could leave the target management cluster in a partially-migrated state.

**What to do:** Before running `clusterctl move`, ensure the cluster and its
ClusterClass (if any) are unpaused:

```sh
kubectl patch cluster <cluster-name> -n <namespace> \
  --type=merge -p '{"spec":{"paused":false}}'
```

## New Features in CAPI v1.12

### Additional lifecycle hooks

CAPI v1.12 adds four new lifecycle hooks that RuntimeExtensions can handle:

| Hook | When it fires |
|------|---------------|
| `BeforeControlPlaneUpgrade` | Before the control plane nodes are upgraded |
| `BeforeWorkersUpgrade` | Before worker nodes are upgraded |
| `AfterWorkersUpgrade` | After all worker nodes have been upgraded |
| `GenerateUpgradePlan` | Allows extensions to supply a custom upgrade sequence |

These are opt-in — existing extensions that do not register for these hooks are
unaffected. See the [CAPI runtime extensions documentation](https://cluster-api.sigs.k8s.io/tasks/experimental-features/runtime-sdk/implement-lifecycle-hooks)
for details.

## Kubernetes Version Support Matrix

| Component | Supported versions |
|-----------|-------------------|
| Management cluster Kubernetes | v1.31.x – v1.34.x |
| Workload cluster Kubernetes | v1.29.x – v1.34.x |

The minimum supported workload cluster version moves from v1.28 (CAPI v1.11) to v1.29.

## Bug Fixes Affecting Observed Behavior

### GlobalInClusterIPPool address assignment

A bug in the IPAM address lookup caused `GlobalInClusterIPPool`-backed IP addresses
to be filtered out incorrectly when multiple pool types were in use simultaneously.
Concretely: if a cluster used both `InClusterIPPool` and `GlobalInClusterIPPool`
pools, only the `InClusterIPPool` addresses were returned, leaving machines that
should have received a `GlobalInClusterIPPool` address without one.

This is fixed in v0.9. No user action is required. Machines that stalled waiting
for an IP address from a `GlobalInClusterIPPool` should reconcile successfully
after the upgrade.
