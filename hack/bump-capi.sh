#!/usr/bin/env bash
# bump-capi.sh bumps sigs.k8s.io/cluster-api to a new version, keeping all
# references in sync:
#   - go.mod: require sigs.k8s.io/cluster-api
#   - go.mod: require sigs.k8s.io/cluster-api/test
#   - go.mod: replace sigs.k8s.io/cluster-api pin
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

METADATA="${REPO_ROOT}/test/e2e/data/shared/v1beta1/metadata.yaml"

# ---- go.mod ----
gomod_set_require_version 'sigs.k8s.io/cluster-api' "${NEW}"
gomod_set_require_version 'sigs.k8s.io/cluster-api/test' "${NEW}"
gomod_set_replace_version 'sigs.k8s.io/cluster-api' "${NEW}"

# ---- test/e2e/data/shared/v1beta1/metadata.yaml ----
# Add new releaseSeries entry as the first element if not already present.
if ! yq -e '.releaseSeries[] | select(.major == '"${CAPI_MAJOR}"' and .minor == '"${CAPI_MINOR}"')' "${METADATA}" > /dev/null 2>&1; then
    yq -i '.releaseSeries = [{"major": '"${CAPI_MAJOR}"', "minor": '"${CAPI_MINOR}"', "contract": "'"${CONTRACT}"'"}] + .releaseSeries' "${METADATA}"
    echo "test/e2e/data/shared/v1beta1/metadata.yaml: Added releaseSeries entry for v${CAPI_MAJOR}.${CAPI_MINOR} (${CONTRACT})"
fi

# ---- go mod tidy ----
run_mod_tidy
