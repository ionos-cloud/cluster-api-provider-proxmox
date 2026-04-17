#!/usr/bin/env bash
# verify-versions.sh checks that versions of special dependencies are consistent
# across the repository. Add this to CI via the verify-versions Makefile target.

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

ERRORS=()

fail() {
    ERRORS+=("$1")
}

# ---- Go version ----
# go.mod, Dockerfile, and docs/Development.md must all reference the same Go
# version. go.mod uses the full "major.minor.patch" form; Dockerfile and docs
# use only "major.minor".

GO_VERSION_ROOT=$(gomod_get_go)

GO_VERSION_MINOR=$(echo "${GO_VERSION_ROOT}" | cut -d. -f1-2)
DOCKERFILE_GO_VERSION=$(dockerfile_get_go)
if [[ "${DOCKERFILE_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), Dockerfile has '${DOCKERFILE_GO_VERSION}'"
fi

DOCS_GO_VERSION=$(docs_get_go)
if [[ -n "${DOCS_GO_VERSION}" && "${DOCS_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), docs/Development.md lists 'Go v${DOCS_GO_VERSION}'"
fi

GOLANGCI_KAL_GO_VERSION=$(golangcikal_get_go)
if [[ -n "${GOLANGCI_KAL_GO_VERSION}" && "${GOLANGCI_KAL_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), .golangci-kal.yml run.go has '${GOLANGCI_KAL_GO_VERSION}'"
fi

# ---- golangci-lint version ----
# The golangci-lint replace directive in go.mod and the version in
# .custom-gcl.yaml must use the same version.

GOLANGCI_VERSION_GOMOD=$(gomod_get_replace 'github.com/golangci/golangci-lint/v2')
GOLANGCI_VERSION_CUSTOM=$(customgcl_get_version)
if versions_differ "${GOLANGCI_VERSION_GOMOD}" "${GOLANGCI_VERSION_CUSTOM}"; then
    fail "golangci-lint version mismatch: go.mod replace has '${GOLANGCI_VERSION_GOMOD}', .custom-gcl.yaml has '${GOLANGCI_VERSION_CUSTOM}'"
fi

# ---- cluster-api: require and replace ----
# The replace directive in go.mod must pin the same version as the require directive.

CAPI_REQUIRE=$(gomod_get_require 'sigs.k8s.io/cluster-api')
CAPI_REPLACE=$(gomod_get_replace 'sigs.k8s.io/cluster-api')
if versions_differ "${CAPI_REQUIRE}" "${CAPI_REPLACE}"; then
    fail "cluster-api version mismatch: require directive has '${CAPI_REQUIRE}', replace directive has '${CAPI_REPLACE}'"
fi

# ---- cluster-api and cluster-api/test ----
# sigs.k8s.io/cluster-api and sigs.k8s.io/cluster-api/test must be the same version.

CAPI_TEST=$(gomod_get_require 'sigs.k8s.io/cluster-api/test')
if versions_differ "${CAPI_REQUIRE}" "${CAPI_TEST}"; then
    fail "cluster-api version mismatch: sigs.k8s.io/cluster-api is '${CAPI_REQUIRE}', sigs.k8s.io/cluster-api/test is '${CAPI_TEST}'"
fi

# ---- cluster-api version in test/e2e metadata ----
# The cluster-api major.minor from go.mod must be listed in the e2e metadata file.

if [[ -n "${CAPI_REQUIRE}" ]]; then
    split_version "${CAPI_REQUIRE}"
    CAPI_MAJOR="${MAJOR}"
    CAPI_MINOR="${MINOR}"
    if ! e2emetadata_has_release "${CAPI_MAJOR}" "${CAPI_MINOR}"; then
        fail "cluster-api v${CAPI_MAJOR}.${CAPI_MINOR} is not listed in test/e2e/data/shared/v1beta1/metadata.yaml"
    fi
fi

# ---- k8s.io core package versions ----
# k8s.io/api, k8s.io/apimachinery, and k8s.io/client-go follow the same release
# cycle and must all be at the same effective version. When a replace directive
# overrides a package, the replace version is the effective one; otherwise the
# version from the require block (direct or indirect) is used.

K8S_PKGS=('k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go')

if ! gomod_has_version_match "${K8S_PKGS[@]}"; then
    version_details=""
    for pkg in "${K8S_PKGS[@]}"; do
        version_details+=" ${pkg}=$(gomod_get_version "${pkg}")"
    done
    fail "k8s.io package version mismatch:${version_details}"
fi

# ---- k8s.io/code-generator ----
# k8s.io/code-generator hosts the conversion-gen tool. It's pinned via
# `replace` only. Its target version must match the effective k8s.io
# package version.

CODE_GEN_VER=$(gomod_get_replace 'k8s.io/code-generator')
K8S_EFFECTIVE_VER=$(gomod_get_version 'k8s.io/api')
if [[ -n "${CODE_GEN_VER}" ]] && versions_differ "${CODE_GEN_VER}" "${K8S_EFFECTIVE_VER}"; then
    fail "k8s.io/code-generator version mismatch: k8s.io/api is '${K8S_EFFECTIVE_VER}', replace for k8s.io/code-generator is '${CODE_GEN_VER}'"
fi

# ---- k8s.io and ENVTEST_K8S_VERSION ----
# The Makefile is meant to call gomod_make_envtest to derive ENVTEST_K8S_VERSION.
# This check verifies that it still does.

EXPECTED_ENVTEST=$(gomod_make_envtest)
ENVTEST_VERSION=$(makefile_get_envtest)
if versions_differ "${EXPECTED_ENVTEST}" "${ENVTEST_VERSION}"; then
    fail "ENVTEST_K8S_VERSION mismatch: k8s.io/api is '$(gomod_get_version 'k8s.io/api')' but ENVTEST_K8S_VERSION is '${ENVTEST_VERSION}'"
fi

# ---- KUBERNETES_VERSION in e2e config ----
# test/e2e/config files should reference the matching Kubernetes version
# (v1.MINOR.PATCH).
K8S_VERSION=$(gomod_get_version 'k8s.io/api')
if [[ -n "${K8S_VERSION}" ]]; then
    split_version "${K8S_VERSION}"
    K8S_MINOR="${MINOR}"
    K8S_PATCH="${PATCH}"
    EXPECTED_K8S_VER="v1.${K8S_MINOR}.${K8S_PATCH}"
    E2E_K8S_VER=$(e2econfig_get_k8s)
    if versions_differ "${E2E_K8S_VER}" "${EXPECTED_K8S_VER}"; then
        fail "KUBERNETES_VERSION mismatch: k8s.io/api is '${K8S_VERSION}', expected KUBERNETES_VERSION '${EXPECTED_K8S_VER}' but e2e config has '${E2E_K8S_VER}'"
    fi

    # ---- --kubernetes-version in docs ----
    # Docs should reference the same Kubernetes version as k8s.io packages.
    DOCS_K8S_VER=$(docs_get_k8s)
    if [[ -n "${DOCS_K8S_VER}" ]]; then
        if versions_differ "${DOCS_K8S_VER}" "${EXPECTED_K8S_VER}"; then
            fail "docs --kubernetes-version mismatch: k8s.io/api is '${K8S_VERSION}', expected '${EXPECTED_K8S_VER}' but docs has '${DOCS_K8S_VER}'"
        fi
    fi
fi

# ---- cluster-api version in e2e config ----
# The cluster-api provider version in e2e config files should match go.mod.
E2E_CAPI_VER=$(e2econfig_get_capi)
if [[ -n "${CAPI_REQUIRE}" && -n "${E2E_CAPI_VER}" ]]; then
    if versions_differ "${E2E_CAPI_VER}" "${CAPI_REQUIRE}"; then
        fail "cluster-api version mismatch: go.mod require is '${CAPI_REQUIRE}', but e2e config has '${E2E_CAPI_VER}'"
    fi
fi

# ---- Report results ----

if [[ ${#ERRORS[@]} -gt 0 ]]; then
    echo "Version consistency check FAILED:"
    for err in "${ERRORS[@]}"; do
        echo "  - ${err}"
        # In GitHub Actions, emit workflow commands for annotations.
        if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
            echo "::error::${err}"
        fi
    done
    exit 1
fi
