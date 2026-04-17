# AGENTS.md

CAPMOX (Cluster API Provider for Proxmox VE) enables declarative management of Kubernetes clusters on Proxmox VE using the Cluster API provider contract. See `go.mod` for dependency versions.

The storage API version is **v1alpha2** (imported as `infrav1`). v1alpha1 exists only for backward compatibility with automatic conversion to/from v1alpha2.

## Repository Structure

- `api/v1alpha1/` — deprecated CRDs with conversion to v1alpha2
- `api/v1alpha2/` — current storage version CRDs (ProxmoxCluster, ProxmoxMachine, ProxmoxMachineTemplate, ProxmoxClusterTemplate)
- `cmd/main.go` — controller manager entry point
- `internal/controller/` — reconciliation logic
- `internal/webhook/` — validation and defaulting webhooks
- `internal/service/` — VM, scheduler, and task services
- `pkg/` — shared packages (proxmox client, scope, cloudinit, ignition, ipam)
- `config/` — Kustomize configuration for CRDs, RBAC, webhooks
- `hack/` — helper scripts
- `test/e2e/` — end-to-end tests

## Commands

### Build & Test

```bash
make build                      # Build manager binary
make test                       # Run unit tests
make test WHAT=./pkg/scope/...  # Run tests for specific packages
make lint                       # Run golangci-lint + kube-api-linter
make lint-fix                   # Lint with auto-fix
make yamlfmt                    # Format YAML files
make tidy                       # go mod tidy
make tilt-up                    # Start Tilt dev environment in a kind cluster
```

### Code Generation

```bash
make manifests    # Regenerate CRDs, RBAC, webhook manifests
make generate     # Regenerate DeepCopy methods and conversion functions
make mockgen      # Regenerate mocks (configured in .mockery.yaml)
```

### Verification

```bash
make verify           # Verify modules and generated files are up to date
```

## Architecture

### Reconciliation Flow

Controllers (`internal/controller/`) reconcile two custom resources:

- **ProxmoxCluster** — manages cluster-level infrastructure (control plane endpoint, allowed nodes)
- **ProxmoxMachine** — manages individual VM lifecycle on Proxmox VE

Each controller creates a **Scope** (`pkg/scope/`) bundling the CAPI owner objects, infrastructure CR, Proxmox client, and IPAM helper into a single context object passed through the reconciliation pipeline.

The ProxmoxMachine controller delegates VM operations to services under `internal/service/`:
- `vmservice/` — VM clone, configure, bootstrap (cloud-init or Ignition), IP assignment, power management, deletion
- `scheduler/` — selects which Proxmox node to place a new VM on
- `taskservice/` — tracks async Proxmox task completion and error handling

### Proxmox Client Abstraction

`pkg/proxmox/client.go` defines the `Client` interface for all Proxmox API operations. The production implementation lives in `pkg/proxmox/goproxmox/` (wrapping `go-proxmox`). Tests use a mock at `pkg/proxmox/proxmoxtest/`.

### API Versions and Conversion

- `api/v1alpha2/` — current storage version
- `api/v1alpha1/` — deprecated; conversion implemented in `*_conversion.go` and `zz_generated.conversion.go`

Key v1alpha2 change: unified `NetworkDevices` array replacing v1alpha1's split of `Default` + `AdditionalDevices`.

### Bootstrap & IPAM

`pkg/cloudinit/` and `pkg/ignition/` handle cloud-init and Flatcar Ignition bootstrap data. IP management is via the Cluster API IPAM contract (`pkg/kubernetes/ipam/`).

## Testing

- Unit tests colocated with source files, using envtest; see `Makefile` for `ENVTEST_K8S_VERSION`
- First `make test` run builds envtest binaries — setup output is not a test failure
- `testify` for assertions, `gomega`/`ginkgo` for BDD-style tests
- Proxmox client mocks generated via `make mockgen` (`.mockery.yaml`)
- E2E tests in `test/e2e/` require a live Proxmox VE instance; controlled by PR labels

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines, paying particular attention to the [AI-Assisted Contributions](CONTRIBUTING.md#ai-assisted-contributions) section which covers authorship, transparency, and quality expectations for AI-assisted work.

## Rules

✅ **Always:**
- Run `make manifests generate mockgen` after modifying API types
- If conversion behavior changes, update `api/v1alpha1/*_conversion.go`
- Run `make lint verify test` before committing

⚠️ **Ask before:**
- Changing v1alpha1 conversion functions (affects backward compatibility)
- Removing or renaming fields in v1alpha2 API types

🚫 **Never:**
- Edit files with a `Code generated … DO NOT EDIT` header
- Edit the unmarked outputs of `make manifests` (`config/crd/bases/*.yaml`, `config/rbac/role.yaml`, `config/webhook/manifests.yaml`) or `make crs-*` (`templates/crs/cni/*.yaml`) — regenerate instead

## Environment

See `envfile.example` for development config. Key vars: `PROXMOX_URL`, `PROXMOX_TOKEN`, `PROXMOX_SECRET`, `CAPMOX_LOGLEVEL`.
