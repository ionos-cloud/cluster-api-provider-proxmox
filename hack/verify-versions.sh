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

GO_VERSION_ROOT=$(gomod_go_version)

GO_VERSION_MINOR=$(echo "${GO_VERSION_ROOT}" | cut -d. -f1-2)
DOCKERFILE_GO_VERSION=$(dockerfile_go_version)
if [[ "${DOCKERFILE_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), Dockerfile has '${DOCKERFILE_GO_VERSION}'"
fi

DOCS_GO_VERSION=$(docs_go_version)
if [[ -n "${DOCS_GO_VERSION}" && "${DOCS_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), docs/Development.md lists 'Go v${DOCS_GO_VERSION}'"
fi

# ---- golangci-lint version ----
# The golangci-lint replace directive in go.mod and the version in
# .custom-gcl.yaml must use the same version.

GOLANGCI_VERSION_GOMOD=$(gomod_replace_version 'github.com/golangci/golangci-lint/v2')
GOLANGCI_VERSION_CUSTOM=$(custom_gcl_version)
if [[ -n "${GOLANGCI_VERSION_GOMOD}" && -n "${GOLANGCI_VERSION_CUSTOM}" && "${GOLANGCI_VERSION_GOMOD}" != "${GOLANGCI_VERSION_CUSTOM}" ]]; then
    fail "golangci-lint version mismatch: go.mod replace has '${GOLANGCI_VERSION_GOMOD}', .custom-gcl.yaml has '${GOLANGCI_VERSION_CUSTOM}'"
fi

# ---- cluster-api: require and replace ----
# The replace directive in go.mod must pin the same version as the require directive.

CAPI_REQUIRE=$(gomod_require_version 'sigs.k8s.io/cluster-api')
CAPI_REPLACE=$(gomod_replace_version 'sigs.k8s.io/cluster-api')
if [[ -n "${CAPI_REQUIRE}" && -n "${CAPI_REPLACE}" && "${CAPI_REQUIRE}" != "${CAPI_REPLACE}" ]]; then
    fail "cluster-api version mismatch: require directive has '${CAPI_REQUIRE}', replace directive has '${CAPI_REPLACE}'"
fi

# ---- cluster-api and cluster-api/test ----
# sigs.k8s.io/cluster-api and sigs.k8s.io/cluster-api/test must be the same version.

CAPI_TEST=$(gomod_require_version 'sigs.k8s.io/cluster-api/test')
if [[ -n "${CAPI_REQUIRE}" && -n "${CAPI_TEST}" && "${CAPI_REQUIRE}" != "${CAPI_TEST}" ]]; then
    fail "cluster-api version mismatch: sigs.k8s.io/cluster-api is '${CAPI_REQUIRE}', sigs.k8s.io/cluster-api/test is '${CAPI_TEST}'"
fi

# ---- cluster-api version in test/e2e metadata ----
# The cluster-api major.minor from go.mod must be listed in the e2e metadata file.

if [[ -n "${CAPI_REQUIRE}" ]]; then
    split_version "${CAPI_REQUIRE}"
    CAPI_MAJOR="${MAJOR}"
    CAPI_MINOR="${MINOR}"
    METADATA_FILE="${REPO_ROOT}/test/e2e/data/shared/v1beta1/metadata.yaml"
    if ! yq -e '.releaseSeries[] | select(.major == '"${CAPI_MAJOR}"' and .minor == '"${CAPI_MINOR}"')' "${METADATA_FILE}" > /dev/null 2>&1; then
        fail "cluster-api v${CAPI_MAJOR}.${CAPI_MINOR} is not listed in test/e2e/data/shared/v1beta1/metadata.yaml"
    fi
fi

# ---- k8s.io core package versions ----
# k8s.io/api, k8s.io/apimachinery, and k8s.io/client-go follow the same release
# cycle and must all be at the same effective version. When a replace directive
# overrides a package, the replace version is the effective one; otherwise the
# version from the require block (direct or indirect) is used.

declare -A K8S_VERSIONS
for pkg in "k8s.io/api" "k8s.io/apimachinery" "k8s.io/client-go"; do
    VERSION=$(effective_version "${pkg}")
    if [[ -n "${VERSION}" ]]; then
        K8S_VERSIONS["${pkg}"]="${VERSION}"
    fi
done

FIRST_K8S_PKG=""
FIRST_K8S_VERSION=""
for pkg in "k8s.io/api" "k8s.io/apimachinery" "k8s.io/client-go"; do
    if [[ -n "${K8S_VERSIONS[${pkg}]+x}" ]]; then
        if [[ -z "${FIRST_K8S_VERSION}" ]]; then
            FIRST_K8S_PKG="${pkg}"
            FIRST_K8S_VERSION="${K8S_VERSIONS[${pkg}]}"
        elif [[ "${K8S_VERSIONS[${pkg}]}" != "${FIRST_K8S_VERSION}" ]]; then
            fail "k8s.io package version mismatch: ${pkg} is '${K8S_VERSIONS[${pkg}]}', but ${FIRST_K8S_PKG} is '${FIRST_K8S_VERSION}'"
        fi
    fi
done

# ---- k8s.io and ENVTEST_K8S_VERSION ----
# The k8s.io packages use major version 0 (v0.MINOR.PATCH) while
# ENVTEST_K8S_VERSION uses major version 1 (1.MINOR.PATCH). The minor.patch
# portions must match.

if [[ -n "${FIRST_K8S_VERSION}" ]]; then
    split_version "${FIRST_K8S_VERSION}"
    K8S_MAJOR="${MAJOR}"
    K8S_MINOR="${MINOR}"
    K8S_PATCH="${PATCH}"
    if [[ "${K8S_MAJOR}" -ne 0 ]]; then
        fail "k8s.io packages should have major version 0, but ${FIRST_K8S_PKG} is '${FIRST_K8S_VERSION}'"
    fi
    ENVTEST_VERSION=$(makefile_envtest_version)
    if [[ -n "${ENVTEST_VERSION}" ]]; then
        EXPECTED_ENVTEST="1.${K8S_MINOR}.${K8S_PATCH}"
        if [[ "${ENVTEST_VERSION}" != "${EXPECTED_ENVTEST}" ]]; then
            fail "ENVTEST_K8S_VERSION mismatch: k8s.io packages are '${FIRST_K8S_VERSION}' (expect ENVTEST ${EXPECTED_ENVTEST}), but Makefile has '${ENVTEST_VERSION}'"
        fi
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
