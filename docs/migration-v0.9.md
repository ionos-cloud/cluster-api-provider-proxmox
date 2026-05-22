# Migration Guide: CAPMOX v0.8 → v0.9

CAPMOX v0.9 upgrades Cluster API to v1.12, controller-runtime to v0.22, and
IPAM provider in-cluster to v1.1.0. No CAPMOX API changes — existing v1alpha2 manifests
work as-is.

See the [CAPI v1.12 release notes](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.12.0)
for upstream breaking changes.

## Prerequisites

- CAPMOX v0.8.x
- Cluster API and clusterctl v1.12
- IPAM provider in-cluster v1.1.0 (if using IPAM)

## Upgrading from earlier releases

>[!WARNING]
>You must be on the latest v0.8 release before upgrading. Direct upgrades from
>pre-v0.8 are not supported.

Upgrading from v0.7 or earlier? Follow the [v0.8 migration guide](migration-v0.8-v1alpha2.md) first.

## No manifest changes required

CAPMOX v0.9 uses the same `v1alpha2` API and `v1beta2` contract as v0.8.
