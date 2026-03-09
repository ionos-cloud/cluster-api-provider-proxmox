#!/usr/bin/env bash
# bump-go.sh bumps the Go version in all places it is referenced:
#   - go.mod
#   - hack/tools/go.mod
#   - Dockerfile
#   - docs/Development.md
#
# Usage:   ./hack/bump-go.sh <new-version>
# Example: ./hack/bump-go.sh 1.26.0

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>"
    echo "Example: $0 1.26.0"
    exit 1
fi

NEW_VERSION="$1"
REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)

# Validate: must be major.minor or major.minor.patch
if ! [[ "${NEW_VERSION}" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
    echo "ERROR: invalid version format '${NEW_VERSION}'"
    echo "Expected: major.minor (e.g. 1.26) or major.minor.patch (e.g. 1.26.0)"
    exit 1
fi

NEW_VERSION_MINOR=$(echo "${NEW_VERSION}" | cut -d. -f1-2)

echo "Bumping Go version to ${NEW_VERSION} (image tag: ${NEW_VERSION_MINOR})..."

# go.mod
sed -i -E "s/^go [0-9]+\.[0-9]+(\.[0-9]+)?/go ${NEW_VERSION}/" "${REPO_ROOT}/go.mod"
echo "  Updated go.mod"

# hack/tools/go.mod
sed -i -E "s/^go [0-9]+\.[0-9]+(\.[0-9]+)?/go ${NEW_VERSION}/" "${REPO_ROOT}/hack/tools/go.mod"
echo "  Updated hack/tools/go.mod"

# Dockerfile – uses only major.minor in the base image tag
sed -i -E "s/^(FROM golang:)[0-9]+\.[0-9]+(.*)/\1${NEW_VERSION_MINOR}\2/" "${REPO_ROOT}/Dockerfile"
echo "  Updated Dockerfile"

# docs/Development.md – lists the required Go version for developers
sed -i -E "s/(- Go v)[0-9]+\.[0-9]+/\1${NEW_VERSION_MINOR}/" "${REPO_ROOT}/docs/Development.md"
echo "  Updated docs/Development.md"

echo "Done."
echo "Next steps: run 'make tidy' to update go.sum files, then 'make verify-versions' to confirm."
