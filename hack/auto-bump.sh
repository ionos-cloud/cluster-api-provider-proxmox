#!/usr/bin/env bash
# auto-bump.sh detects version inconsistencies and runs the appropriate bump
# scripts to fix them. Designed to be called from CI on dependabot PRs to
# 'finish' a partial version bump.
#
# Each group is checked independently. If a mismatch is found, the
# authoritative version (usually the one dependabot bumps first) is used to
# re-synchronise all references.
#
# Usage: ./hack/auto-bump.sh
# Exit code 0: no mismatches found or all fixed successfully.

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

BUMPED=()

# ---- Go version ----
# Source of truth: go.mod.

GO_VERSION=$(gomod_go_version)
GO_MINOR=$(echo "${GO_VERSION}" | cut -d. -f1-2)
DOCKERFILE_GO=$(dockerfile_go_version)
DOCS_GO=$(docs_go_version)

if [[ "${DOCKERFILE_GO}" != "${GO_MINOR}" || ( -n "${DOCS_GO}" && "${DOCS_GO}" != "${GO_MINOR}" ) ]]; then
    "${REPO_ROOT}/hack/bump-go.sh" "${GO_VERSION}"
    BUMPED+=("go:${GO_VERSION}")
fi

# ---- golangci-lint ----
# Source of truth: go.mod replace directive.

GCL_GOMOD=$(gomod_replace_version 'github.com/golangci/golangci-lint/v2')
GCL_CUSTOM=$(custom_gcl_version)
if [[ -n "${GCL_GOMOD}" && -n "${GCL_CUSTOM}" && "${GCL_GOMOD}" != "${GCL_CUSTOM}" ]]; then
    "${REPO_ROOT}/hack/bump-golangci-lint.sh" "${GCL_GOMOD}"
    BUMPED+=("golangci-lint:${GCL_GOMOD}")
fi

# ---- cluster-api ----
# Source of truth: require directive in go.mod. Dependabot bumps require but
# leaves the replace pin stale.

CAPI_REQ=$(gomod_require_version 'sigs.k8s.io/cluster-api')
CAPI_REP=$(gomod_replace_version 'sigs.k8s.io/cluster-api')
CAPI_TEST=$(gomod_require_version 'sigs.k8s.io/cluster-api/test')

CAPI_NEEDS_BUMP=false
if [[ -n "${CAPI_REQ}" && -n "${CAPI_REP}" && "${CAPI_REQ}" != "${CAPI_REP}" ]]; then
    CAPI_NEEDS_BUMP=true
fi
if [[ -n "${CAPI_REQ}" && -n "${CAPI_TEST}" && "${CAPI_REQ}" != "${CAPI_TEST}" ]]; then
    CAPI_NEEDS_BUMP=true
fi

if [[ "${CAPI_NEEDS_BUMP}" == true ]]; then
    # Determine contract from existing metadata — use the contract of the
    # latest entry as a default since auto-bump can't make contract decisions.
    CONTRACT=$(yq '.releaseSeries[0].contract' "${REPO_ROOT}/test/e2e/data/shared/v1beta1/metadata.yaml")
    "${REPO_ROOT}/hack/bump-capi.sh" "${CAPI_REQ}" "${CONTRACT}"
    BUMPED+=("cluster-api:${CAPI_REQ}")
fi

# ---- k8s.io packages ----
# Source of truth: whichever k8s.io package has the highest version (i.e. the
# one dependabot bumped). All three must match.

K8S_LATEST=""
for pkg in 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'; do
    ver=$(effective_version "${pkg}")
    if [[ -n "${ver}" ]]; then
        if [[ -z "${K8S_LATEST}" || "$(printf '%s\n%s' "${K8S_LATEST}" "${ver}" | sort -V | tail -1)" == "${ver}" ]]; then
            K8S_LATEST="${ver}"
        fi
    fi
done

if [[ -n "${K8S_LATEST}" ]]; then
    NEEDS_BUMP=false
    for pkg in 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'; do
        ver=$(effective_version "${pkg}")
        if [[ -n "${ver}" && "${ver}" != "${K8S_LATEST}" ]]; then
            NEEDS_BUMP=true
            break
        fi
    done

    # Also check ENVTEST alignment.
    if [[ "${NEEDS_BUMP}" != true ]]; then
        split_version "${K8S_LATEST}"
        EXPECTED_ENVTEST="1.${MINOR}.${PATCH}"
        ENVTEST_CUR=$(makefile_envtest_version)
        if [[ -n "${ENVTEST_CUR}" && "${ENVTEST_CUR}" != "${EXPECTED_ENVTEST}" ]]; then
            NEEDS_BUMP=true
        fi
    fi

    if [[ "${NEEDS_BUMP}" == true ]]; then
        "${REPO_ROOT}/hack/bump-k8s.sh" "${K8S_LATEST}"
        BUMPED+=("k8s.io:${K8S_LATEST}")
    fi
fi

# ---- Summary ----

if [[ ${#BUMPED[@]} -gt 0 ]]; then
    echo "Auto-bump applied:"
    for b in "${BUMPED[@]}"; do
        echo "  - ${b}"
    done
else
    echo "All versions are consistent, no bumps needed."
fi
