#!/usr/bin/env bash
# bump-k8s.sh bumps the k8s.io core packages to a new version.
#
# ENVTEST_K8S_VERSION is derived dynamically via hack/envtest-ver.sh
# and does not need to be updated by this script.
#
# In addition to go.mod, this updates:
#   - test/e2e/config: KUBERNETES_VERSION defaults (v1.MINOR.PATCH)
#   - docs: --kubernetes-version references (v1.MINOR.PATCH)
#
# Usage:   ./hack/bump-k8s.sh <new-version>
# Example: ./hack/bump-k8s.sh 0.33.0

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>"
    echo "Example: $0 0.33.0"
    exit 1
fi

validate_semver "$1"
NEW=$(ensure_v_prefix "$1")

split_version "${NEW}"
if [[ "${MAJOR}" -ne 0 ]]; then
    echo "ERROR: k8s.io packages use major version 0, got '${MAJOR}'"
    echo "Provide a v0.MINOR.PATCH version (e.g. 0.33.0 or v0.33.0)"
    exit 1
fi

K8S_PKGS=('k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go')

# Derive the Kubernetes version (major 1) from the k8s.io package version.
K8S_VER="v1.${MINOR}.${PATCH}"

# Step 1: Remove any existing replace pins for k8s.io packages.
gomod_del_replace "${K8S_PKGS[@]}"

# Step 2: Bump the require directives.
gomod_set_require "${NEW}" "${K8S_PKGS[@]}"

# Step 3: go mod tidy.
gomod_tidy

# Step 4: Verify effective versions — add replace overrides where needed.
OVERRIDE_PKGS=()
for pkg in "${K8S_PKGS[@]}"; do
    eff=$(gomod_get_version "${pkg}")
    if [[ -n "${eff}" && "${eff}" != "${NEW}" ]]; then
        echo "WARNING: ${pkg} resolved to ${eff} instead of ${NEW} after go mod tidy; adding replace override"
        OVERRIDE_PKGS+=("${pkg}")
    fi
done

if [[ ${#OVERRIDE_PKGS[@]} -gt 0 ]]; then
    gomod_add_replace "${NEW}" "${OVERRIDE_PKGS[@]}"
    gomod_tidy

    # Final check.
    for pkg in "${OVERRIDE_PKGS[@]}"; do
        eff=$(gomod_get_version "${pkg}")
        if [[ -n "${eff}" && "${eff}" != "${NEW}" ]]; then
            echo "ERROR: ${pkg} is still at ${eff} instead of ${NEW} after adding replace override"
            exit 1
        fi
    done
fi

# Step 5: Update KUBERNETES_VERSION in e2e config files.
e2econfig_set_k8s "${K8S_VER}"

# Step 6: Update --kubernetes-version references in docs.
docs_set_k8s "${K8S_VER}"
