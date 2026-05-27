#!/usr/bin/env bash
# bump-k8s.sh bumps the k8s.io core packages to a new version.
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

K8S_PKGS=('k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go' 'k8s.io/code-generator')

# Derive the Kubernetes version (major 1) from the k8s.io package version.
K8S_VER="v1.${MINOR}.${PATCH}"

# Remove any existing replace pins, set requires for the API trio, and pin all
# four packages via replace (k8s.io/code-generator is not a *dependency*,
# it's for the *tool* cmd/conversion-gen)
gomod_del_replace "${K8S_PKGS[@]}"
gomod_set_require "${NEW}" 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'
gomod_add_replace "${NEW}" "${K8S_PKGS[@]}"
gomod_tidy

# Update KUBERNETES_VERSION in e2e config files.
e2econfig_set_k8s "${K8S_VER}"

# Update --kubernetes-version references in docs.
docs_set_k8s "${K8S_VER}"
