# E2E Test

This document is to help developers understand how to run e2e tests for CAPMOX.

## Requirements

In order to run the e2e tests the following requirements must be met:

* [Ginkgo](https://github.com/onsi/ginkgo)
* [Docker](https://www.docker.com/)
* Proxmox VE Cluster
* A Kubernetes cluster running on Proxmox VE

### Environment variables

The first step to running the e2e tests is setting up the required environment variables:

| Environment variable              | Description                              |
| --------------------------------- |------------------------------------------|
| `PROXMOX_URL`       | The Proxmox host                         |
| `PROXMOX_TOKEN`  | The Proxmox token                        |
| `PROXMOX_SECRET`           | The secret assocaited with the token     |
| `PROXMOX_SOURCENODE`           | The Proxmox node that host the temnplate |
| `TEMPLATE_VMID`           | The VM ID for the temnplate              |

### Running e2e test

In the root project directory run:

```
make test-e2e
```

### Running Conformance test

In the root project directory run:

```
make test-conformance
```
