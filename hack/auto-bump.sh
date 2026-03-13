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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# shellcheck source=hack/helpers.sh
source "${SCRIPT_DIR}/helpers.sh"

# ---- Go version ----
# Source of truth: go.mod.

GO_VERSION=$(gomod_get_go)
GO_MINOR=$(echo "${GO_VERSION}" | cut -d. -f1-2)
DOCKERFILE_GO=$(dockerfile_get_go)
DOCS_GO=$(docs_get_go)

if [[ "${DOCKERFILE_GO}" != "${GO_MINOR}" || ( -n "${DOCS_GO}" && "${DOCS_GO}" != "${GO_MINOR}" ) ]]; then
    echo "Auto-bump: go ${GO_VERSION}"
    "${SCRIPT_DIR}/bump-go.sh" "${GO_VERSION}"
fi

# ---- golangci-lint ----
# Source of truth: require directive in go.mod. Dependabot bumps require but
# leaves the replace pin stale.

GCL_REQ=$(gomod_get_require 'github.com/golangci/golangci-lint/v2')
GCL_REP=$(gomod_get_replace 'github.com/golangci/golangci-lint/v2')
GCL_CUSTOM=$(customgcl_get_version)

GCL_NEEDS_BUMP=false
if versions_differ "${GCL_REQ}" "${GCL_REP}"; then
    GCL_NEEDS_BUMP=true
fi
if versions_differ "${GCL_REQ}" "${GCL_CUSTOM}"; then
    GCL_NEEDS_BUMP=true
fi

if [[ "${GCL_NEEDS_BUMP}" == true && -n "${GCL_REQ}" ]]; then
    echo "Auto-bump: golangci-lint ${GCL_REQ}"
    "${SCRIPT_DIR}/bump-golangci-lint.sh" "${GCL_REQ}"
fi

# ---- cluster-api ----
# Source of truth: require directive in go.mod. Dependabot bumps require but
# leaves the replace pin stale.

CAPI_REQ=$(gomod_get_require 'sigs.k8s.io/cluster-api')
CAPI_REP=$(gomod_get_replace 'sigs.k8s.io/cluster-api')
CAPI_TEST=$(gomod_get_require 'sigs.k8s.io/cluster-api/test')

CAPI_NEEDS_BUMP=false
if versions_differ "${CAPI_REQ}" "${CAPI_REP}"; then
    CAPI_NEEDS_BUMP=true
fi
if versions_differ "${CAPI_REQ}" "${CAPI_TEST}"; then
    CAPI_NEEDS_BUMP=true
fi

if [[ "${CAPI_NEEDS_BUMP}" == true ]]; then
    # Determine contract from the metadata file — use the contract of the
    # entry with the highest major.minor as a default since auto-bump can't
    # make contract decisions.
    CONTRACT=$(metadata_latest_contract)
    echo "Auto-bump: cluster-api ${CAPI_REQ}"
    "${SCRIPT_DIR}/bump-capi.sh" "${CAPI_REQ}" "${CONTRACT}"
fi

# ---- k8s.io packages ----
# Source of truth: require directives in go.mod. Dependabot bumps the require
# but leaves replace directives stale. Find the highest require version (i.e.
# the one dependabot bumped) and synchronise everything to that.

K8S_LATEST=""
for pkg in 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'; do
    ver=$(gomod_get_require "${pkg}")
    if [[ -n "${ver}" ]]; then
        if [[ -z "${K8S_LATEST}" || "$(printf '%s\n%s' "${K8S_LATEST}" "${ver}" | sort -V | tail -1)" == "${ver}" ]]; then
            K8S_LATEST="${ver}"
        fi
    fi
done

if [[ -n "${K8S_LATEST}" ]]; then
    NEEDS_BUMP=false
    for pkg in 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'; do
        req=$(gomod_get_require "${pkg}")
        if versions_differ "${req}" "${K8S_LATEST}"; then
            NEEDS_BUMP=true
            break
        fi
        rep=$(gomod_get_replace "${pkg}")
        if [[ -n "${rep}" ]] && versions_differ "${rep}" "${K8S_LATEST}"; then
            NEEDS_BUMP=true
            break
        fi
    done

    # Also check ENVTEST alignment.
    if [[ "${NEEDS_BUMP}" != true ]]; then
        split_version "${K8S_LATEST}"
        ENVTEST_CUR=$(makefile_get_envtest)
        ENVTEST_MINOR=$(echo "${ENVTEST_CUR}" | cut -d. -f2)
        if [[ -n "${ENVTEST_CUR}" && "${ENVTEST_MINOR}" != "${MINOR}" ]]; then
            NEEDS_BUMP=true
        fi
    fi

    if [[ "${NEEDS_BUMP}" == true ]]; then
        echo "Auto-bump: k8s.io ${K8S_LATEST}"
        "${SCRIPT_DIR}/bump-k8s.sh" "${K8S_LATEST}"
    fi
fi
