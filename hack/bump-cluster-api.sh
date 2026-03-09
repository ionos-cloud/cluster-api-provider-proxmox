#!/usr/bin/env bash
# bump-cluster-api.sh bumps the sigs.k8s.io/cluster-api version in go.mod,
# keeping the require directive, the replace pin, and cluster-api/test in sync.
#
# Usage:   ./hack/bump-cluster-api.sh <new-version>
# Example: ./hack/bump-cluster-api.sh v1.11.0

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>"
    echo "Example: $0 v1.11.0"
    exit 1
fi

NEW_VERSION="$1"
REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)

# Ensure version starts with 'v'
if [[ "${NEW_VERSION}" != v* ]]; then
    echo "ERROR: version must start with 'v', got '${NEW_VERSION}'"
    exit 1
fi

echo "Bumping sigs.k8s.io/cluster-api to ${NEW_VERSION}..."

# require sigs.k8s.io/cluster-api (not /test or -ipam-provider-in-cluster)
sed -i -E "s|(^\s+sigs\.k8s\.io/cluster-api[[:space:]]+)v[^ ]+|\1${NEW_VERSION}|" "${REPO_ROOT}/go.mod"
echo "  Updated require sigs.k8s.io/cluster-api"

# require sigs.k8s.io/cluster-api/test
sed -i -E "s|(^\s+sigs\.k8s\.io/cluster-api/test) v[^ ]+|\1 ${NEW_VERSION}|" "${REPO_ROOT}/go.mod"
echo "  Updated require sigs.k8s.io/cluster-api/test"

# replace sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api <version>
sed -i -E "s|(^replace sigs\.k8s\.io/cluster-api => sigs\.k8s\.io/cluster-api) v[^ ]+|\1 ${NEW_VERSION}|" "${REPO_ROOT}/go.mod"
echo "  Updated replace sigs.k8s.io/cluster-api"

echo "Done."
echo "Next steps:"
echo "  1. Run 'go mod tidy' to resolve transitive dependencies."
echo "  2. Run 'make verify-versions' to confirm consistency."
echo "  Note: cluster-api upgrades often require code changes – check the release notes."
