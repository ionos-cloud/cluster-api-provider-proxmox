#!/usr/bin/env bash
# bump-capi.sh bumps sigs.k8s.io/cluster-api to a new version, keeping all
# references in sync:
#   - go.mod: require sigs.k8s.io/cluster-api
#   - go.mod: require sigs.k8s.io/cluster-api/test
#   - go.mod: replace sigs.k8s.io/cluster-api pin
#   - test/e2e/config: cluster-api provider versions and download URLs
#   - test/e2e/data/shared/v1beta1/metadata.yaml: releaseSeries first entry
#
# Usage:   ./hack/bump-capi.sh <new-version> <contract>
# Example: ./hack/bump-capi.sh 1.11.0 v1beta2

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <new-version> <contract>"
    echo "Example: $0 1.11.0 v1beta2"
    exit 1
fi

validate_semver "$1"
NEW=$(ensure_v_prefix "$1")
CONTRACT="$2"

split_version "${NEW}"
CAPI_MAJOR="${MAJOR}"
CAPI_MINOR="${MINOR}"

# ---- go.mod ----
gomod_set_require "${NEW}" 'sigs.k8s.io/cluster-api' 'sigs.k8s.io/cluster-api/test'
gomod_add_replace "${NEW}" 'sigs.k8s.io/cluster-api'

# ---- test/e2e/data/shared/v1beta1/metadata.yaml ----
# Add new releaseSeries entry as the first element if not already present.
if ! metadata_has_release "${CAPI_MAJOR}" "${CAPI_MINOR}"; then
    metadata_add_release "${CAPI_MAJOR}" "${CAPI_MINOR}" "${CONTRACT}"
fi

# ---- test/e2e/config ----
# Update cluster-api provider version in e2e config files.
e2econfig_set_capi "${NEW}"

# ---- go mod tidy ----
gomod_tidy
