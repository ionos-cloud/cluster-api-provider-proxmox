#!/usr/bin/env bash
# verify-versions.sh checks that versions of special dependencies are consistent
# across the repository. Add this to CI via the verify-versions Makefile target.

set -euo pipefail

REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
ERRORS=()

fail() {
    ERRORS+=("$1")
}

# ---- Go version ----
# go.mod, Dockerfile, and docs/Development.md must all reference the same Go
# version. go.mod uses the full "major.minor.patch" form; Dockerfile and docs
# use only "major.minor".

GO_VERSION_ROOT=$(grep '^go ' "${REPO_ROOT}/go.mod" | awk '{print $2}')

GO_VERSION_MINOR=$(echo "${GO_VERSION_ROOT}" | cut -d. -f1-2)
DOCKERFILE_GO_VERSION=$(grep -E '^FROM golang:[0-9]+\.[0-9]+' "${REPO_ROOT}/Dockerfile" | sed -E 's/FROM golang:([0-9]+\.[0-9]+).*/\1/' | head -1)
if [[ "${DOCKERFILE_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), Dockerfile has '${DOCKERFILE_GO_VERSION}'"
fi

DOCS_GO_VERSION=$(grep -E '^\s*- Go v[0-9]+\.[0-9]+' "${REPO_ROOT}/docs/Development.md" | sed -E 's/.*Go v([0-9]+\.[0-9]+).*/\1/' | head -1)
if [[ -n "${DOCS_GO_VERSION}" && "${DOCS_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), docs/Development.md lists 'Go v${DOCS_GO_VERSION}'"
fi

# ---- golangci-lint version ----
# The golangci-lint replace directive in go.mod and the version in
# .github/workflows/lint.yml must use the same version.

GOLANGCI_VERSION_GOMOD=$(grep -E '^\s+github\.com/golangci/golangci-lint/v[0-9]+ =>' "${REPO_ROOT}/go.mod" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | tail -1 || true)
GOLANGCI_VERSION_ACTION=$(grep -A5 'golangci-lint-action' "${REPO_ROOT}/.github/workflows/lint.yml" | grep 'version:' | awk '{print $2}' | head -1 || true)
if [[ -n "${GOLANGCI_VERSION_GOMOD}" && -n "${GOLANGCI_VERSION_ACTION}" && "${GOLANGCI_VERSION_GOMOD}" != "${GOLANGCI_VERSION_ACTION}" ]]; then
    fail "golangci-lint version mismatch: go.mod replace has '${GOLANGCI_VERSION_GOMOD}', .github/workflows/lint.yml has '${GOLANGCI_VERSION_ACTION}'"
fi

# ---- cluster-api: require and replace ----
# The replace directive in go.mod must pin the same version as the require directive.

CAPI_REQUIRE=$(grep -E '^\s+sigs\.k8s\.io/cluster-api\s+v' "${REPO_ROOT}/go.mod" | awk '{print $2}' | head -1 || true)
CAPI_REPLACE=$(grep -E 'sigs\.k8s\.io/cluster-api =>' "${REPO_ROOT}/go.mod" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | tail -1 || true)
if [[ -n "${CAPI_REQUIRE}" && -n "${CAPI_REPLACE}" && "${CAPI_REQUIRE}" != "${CAPI_REPLACE}" ]]; then
    fail "cluster-api version mismatch: require directive has '${CAPI_REQUIRE}', replace directive has '${CAPI_REPLACE}'"
fi

# ---- cluster-api and cluster-api/test ----
# sigs.k8s.io/cluster-api and sigs.k8s.io/cluster-api/test must be the same version.

CAPI_TEST=$(grep -E '^\s+sigs\.k8s\.io/cluster-api/test v' "${REPO_ROOT}/go.mod" | awk '{print $2}' | head -1 || true)
if [[ -n "${CAPI_REQUIRE}" && -n "${CAPI_TEST}" && "${CAPI_REQUIRE}" != "${CAPI_TEST}" ]]; then
    fail "cluster-api version mismatch: sigs.k8s.io/cluster-api is '${CAPI_REQUIRE}', sigs.k8s.io/cluster-api/test is '${CAPI_TEST}'"
fi

# ---- cluster-api version in test/e2e metadata ----
# The cluster-api major.minor from go.mod must be listed in the e2e metadata file.

if [[ -n "${CAPI_REQUIRE}" ]]; then
    CAPI_VERSION_NO_V=$(echo "${CAPI_REQUIRE}" | sed 's/v//')
    CAPI_MAJOR=$(echo "${CAPI_VERSION_NO_V}" | cut -d. -f1)
    CAPI_MINOR=$(echo "${CAPI_VERSION_NO_V}" | cut -d. -f2)
    METADATA_FILE="${REPO_ROOT}/test/e2e/data/shared/v1beta1/metadata.yaml"
    if ! awk '/- major: '"${CAPI_MAJOR}"'[[:space:]]*$/{found=1; next} found && /minor: '"${CAPI_MINOR}"'[[:space:]]*$/{ok=1; exit} {found=0} END{exit !ok}' "${METADATA_FILE}"; then
        fail "cluster-api v${CAPI_MAJOR}.${CAPI_MINOR} is not listed in test/e2e/data/shared/v1beta1/metadata.yaml"
    fi
fi

# ---- k8s.io core package versions ----
# k8s.io/api, k8s.io/apimachinery, and k8s.io/client-go follow the same release
# cycle and must all be at the same effective version. When a replace directive
# overrides a package, the replace version is the effective one.

effective_version() {
    local pkg="$1"
    local replace_ver
    replace_ver=$(grep -E "^\s+${pkg} =>" "${REPO_ROOT}/go.mod" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | tail -1 || true)
    if [[ -n "${replace_ver}" ]]; then
        echo "${replace_ver}"
        return
    fi
    grep -E "^\s+${pkg} v" "${REPO_ROOT}/go.mod" | awk '{print $2}' | head -1 || true
}

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

# ---- Report results ----

if [[ ${#ERRORS[@]} -gt 0 ]]; then
    echo "Version consistency check FAILED:"
    for err in "${ERRORS[@]}"; do
        echo "  - ${err}"
    done
    exit 1
fi
