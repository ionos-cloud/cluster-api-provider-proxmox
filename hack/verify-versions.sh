#!/usr/bin/env bash
# verify-versions.sh checks that versions of special dependencies are consistent
# across the repository. Add this to CI via the verify-versions Makefile target.

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

HAD_ERRORS=false

fail() {
    local msg="$1"
    echo "ERROR: ${msg}" >&2
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        echo "::error::${msg}" # NOSONAR: workflow commands must go to stdout for GitHub Actions to parse them
    fi
    HAD_ERRORS=true
}

# ---- Go version ----
# go.mod and Dockerfile must reference the same Go version. go.mod uses the
# full "major.minor.patch" form; Dockerfile uses only "major.minor".

GO_VERSION_ROOT=$(gomod_get_go)
GO_VERSION_MINOR=$(echo "${GO_VERSION_ROOT}" | cut -d. -f1-2)

DOCKERFILE_GO_VERSION=$(dockerfile_get_go)
if [[ "${DOCKERFILE_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), Dockerfile has '${DOCKERFILE_GO_VERSION}'"
fi

# ---- golangci-lint version ----
# The golangci-lint replace directive in go.mod and the version in
# .custom-gcl.yaml must use the same version.

GOLANGCI_VERSION_GOMOD=$(gomod_get_replace 'github.com/golangci/golangci-lint/v2')
GOLANGCI_VERSION_CUSTOM=$(customgcl_get_version)
if [[ "${GOLANGCI_VERSION_GOMOD}" != "${GOLANGCI_VERSION_CUSTOM}" ]]; then
    fail "golangci-lint version mismatch: go.mod replace has '${GOLANGCI_VERSION_GOMOD}', .custom-gcl.yaml has '${GOLANGCI_VERSION_CUSTOM}'"
fi

if [[ "${HAD_ERRORS}" == true ]]; then
    exit 1
fi
