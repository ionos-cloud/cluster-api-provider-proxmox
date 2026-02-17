# Copilot Instructions for CAPMOX

This repository contains the Cluster API Provider for Proxmox Virtual Environment (CAPMOX).

## Project Overview

CAPMOX is a Kubernetes Cluster API provider that enables declarative management of Kubernetes clusters on Proxmox VE infrastructure. It follows the Kubernetes Operator pattern and uses controllers to reconcile resources.

- **Language**: Go 1.25.0
- **Framework**: Kubebuilder v4
- **Key Dependencies**: 
  - Cluster API v1.11.4
  - controller-runtime v0.21.0
  - go-proxmox v0.3.2 (Proxmox API client)

## Repository Structure

- `api/v1alpha1/` - Custom Resource Definitions (ProxmoxCluster, ProxmoxMachine, ProxmoxMachineTemplate, ProxmoxClusterTemplate)
- `cmd/main.go` - Controller manager entry point
- `internal/controller/` - Reconciliation logic for controllers
- `internal/webhook/` - Webhook handlers for validation and defaulting
- `pkg/` - Shared packages (ignition, cloudinit, proxmox client, scope, ipam)
- `config/` - Kustomize configuration for CRDs, RBAC, webhooks
- `docs/` - Documentation (Development.md, Usage.md, Troubleshooting.md)
- `test/e2e/` - End-to-end tests
- `hack/` - Helper scripts and tools

**Note:** This branch (v1alpha2/wip) is in active development. The v1alpha2 API version is not yet available in this branch.

## Build and Development Workflow

### Prerequisites
- Go 1.25.0
- Docker (for building images and running kind)
- kubectl
- kind or minikube
- make

### Key Commands

**Building:**
```bash
make build              # Build the manager binary
make docker-build       # Build Docker image
```

**Testing:**
```bash
make test              # Run unit tests (includes manifests, generate, fmt, vet)
make test WHAT=./pkg/... # Run tests for specific packages
```

**Code Generation and Validation:**
```bash
make manifests         # Generate CRDs and RBAC manifests
make generate          # Generate DeepCopy methods
make mockgen           # Generate mocks for testing
make verify            # Verify modules and generated files are up to date
make verify-modules    # Check if go.mod and go.sum are up to date
make verify-gen        # Check if generated files are up to date
```

**Linting:**
```bash
make lint              # Run golangci-lint
make yamlfmt           # Format YAML files
```

**Code Formatting:**
```bash
make fmt               # Run go fmt
make vet               # Run go vet
```

**Module Management:**
```bash
make tidy              # Run go mod tidy (including hack/tools)
```

### Development Environment Setup

**Tilt (Recommended):**
Use the helper script to set up a development environment with Tilt:
```bash
./hack/start-capi-tilt.sh
```

This will:
1. Clone cluster-api and cluster-api-ipam-provider-in-cluster if not present
2. Create tilt-settings.json
3. Start Tilt with hot-reload enabled

**Manual Setup:**
See `docs/Development.md` for detailed manual setup instructions.

### Important Development Notes

1. **Always run `make verify` before committing** - This ensures generated files and modules are up to date
2. **After modifying API types**, run `make manifests generate` to update generated code and CRDs
3. **After adding new dependencies**, run `make tidy` to update go.mod and go.sum
4. **When adding mocks**, update `.mockery.yaml` and run `make mockgen`

## CI/CD Pipeline

The repository uses GitHub Actions with the following workflows:

- **test.yml**: Runs `make verify` and `make test`, includes SonarQube scanning
- **lint.yml**: Runs golangci-lint, yamllint, and actionlint
- **e2e.yml**: End-to-end testing
- **codespell.yml**: Spell checking

All PRs must pass these checks before merging.

## Common Issues and Workarounds

1. **Test Failures**: Run `make test` to see detailed error messages. Unit tests use envtest with Kubernetes 1.30.0.
2. **Build Failures**: Ensure Go version matches go.mod (1.25.0). Run `make tidy` to fix module issues.
3. **Generated Files Out of Date**: Run `make verify-gen` to check, then `make manifests generate mockgen` to fix.
4. **Module Issues**: Run `make verify-modules` to check, then `make tidy` to fix.

## Testing Strategy

- Unit tests are located alongside source files (e.g., `pkg/scope/machine_test.go`)
- Use testify for assertions and gomega for BDD-style tests
- Mock interfaces are generated using mockery (see `.mockery.yaml`)
- E2E tests are in `test/e2e/` and require a Proxmox VE instance

## Key Conventions

- Controllers follow the Cluster API contract for infrastructure providers
- Use structured logging with klog/logr
- Error handling uses pkg/errors for wrapping
- Network configuration supports both cloud-init and Ignition
- Support for IPAM provider for IP address management

## Environment Variables

Development environment variables are documented in `envfile.example`. Key variables:
- `PROXMOX_URL` - Proxmox API endpoint
- `PROXMOX_TOKEN` - API token ID
- `PROXMOX_SECRET` - API token secret
- `CAPMOX_LOGLEVEL` - Log level (default: 4)

## Additional Resources

- [Development Guide](docs/Development.md)
- [Usage Guide](docs/Usage.md)
- [Contributing Guide](CONTRIBUTING.md)
- [Troubleshooting](docs/Troubleshooting.md)
