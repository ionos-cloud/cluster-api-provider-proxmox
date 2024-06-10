# Kubernetes Cluster API Provider for Proxmox Virtual Environment - CAPMOX

[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=ionos-cloud_cluster-api-provider-proxmox&metric=alert_status&token=fb1b4c0a87d83a780c76c21be0f89dc13efc2ca0)](https://sonarcloud.io/summary/new_code?id=ionos-cloud_cluster-api-provider-proxmox)
[![Go Report Card](https://goreportcard.com/badge/github.com/ionos-cloud/cluster-api-provider-proxmox)](https://goreportcard.com/report/github.com/ionos-cloud/cluster-api-provider-proxmox)
[![End-to-End Test Status](https://github.com/ionos-cloud/cluster-api-provider-proxmox/actions/workflows/e2e.yml/badge.svg?branch=main)](https://github.com/ionos-cloud/cluster-api-provider-proxmox/actions/workflows/e2e.yml?query=branch%3Amain)

## Overview

The [Cluster API](https://github.com/kubernetes-sigs/cluster-api) brings declarative, Kubernetes-style APIs to cluster creation, configuration and management.
Cluster API Provider for Proxmox VE is a concrete implementation of Cluster API for Proxmox VE.

## Launching a Kubernetes cluster on Proxmox VE

Check out the [quickstart guide](./docs/Usage.md#quick-start) for launching a cluster on Proxmox VE.

## Compatibility with Cluster API and Kubernetes Versions
This provider's versions are compatible with the following versions of Cluster API:

|                        | Cluster API v1beta1 (v1.4) | Cluster API v1beta1 (v1.5) | Cluster API v1beta1 (v1.6) | Cluster API v1beta1 (v1.7) |
|------------------------|:--------------------------:|:--------------------------:|:--------------------------:|:--------------------------:|
| CAPMOX v1alpha1 (v0.1) |             ✓              |             ✓              |             ☓              |             ☓              |
| CAPMOX v1alpha1 (v0.2) |             ☓              |             ✓              |             ✓              |             ☓              |
| CAPMOX v1alpha1 (v0.3) |             ☓              |             ✓              |             ✓              |             ✓              |
| CAPMOX v1alpha1 (v0.4) |             ☓              |             ✓              |             ✓              |             ✓              |
| CAPMOX v1alpha1 (v0.5) |             ☓              |             ☓              |             ✓              |             ✓              |

(See [Kubernetes support matrix](https://cluster-api.sigs.k8s.io/reference/versions.html) of Cluster API versions).

## Documentation

Further documentation is available in the `/docs` directory.

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/cluster-api-proxmox)

## Security

We take security seriously.
Please read our [security policy](SECURITY.md) for information on how to report security issues.
