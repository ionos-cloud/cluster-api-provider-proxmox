#!/usr/bin/env bash
# bump-capi.sh bumps sigs.k8s.io/cluster-api to a new version, keeping all
# references in sync:
#   - go.mod: require sigs.k8s.io/cluster-api
#   - go.mod: require sigs.k8s.io/cluster-api/test
#   - go.mod: replace sigs.k8s.io/cluster-api pin
#   - test/e2e/config: cluster-api provider versions and download URLs
#   - test/e2e/data/shared/<contract>/metadata.yaml: releaseSeries first entry,
#     in every contract catalog
#
# Usage:   ./hack/bump-capi.sh <new-version>
# Example: ./hack/bump-capi.sh 1.11.0

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>" >&2
    echo "Example: $0 1.11.0" >&2
    exit 1
fi

validate_semver "$1"
NEW=$(ensure_v_prefix "$1")

# ---- go.mod ----
gomod_set_require "${NEW}" 'sigs.k8s.io/cluster-api' 'sigs.k8s.io/cluster-api/test'
gomod_add_replace "${NEW}" 'sigs.k8s.io/cluster-api'

# ---- test/e2e/data/shared/<contract>/metadata.yaml ----
# Register the release in every contract catalog. Idempotent: only catalogs
# missing the entry are touched.
e2emetadata_add_release "${NEW}"

# ---- test/e2e/config ----
# Update cluster-api provider version in e2e config files.
e2econfig_set_capi "${NEW}"

# ---- go mod tidy ----
gomod_tidy
