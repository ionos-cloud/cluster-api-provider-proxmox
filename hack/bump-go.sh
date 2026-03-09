#!/usr/bin/env bash
# bump-go.sh bumps the Go version in all places it is referenced:
#   - go.mod
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

# Normalize: strip leading 'v' if present (Go versions don't use a 'v' prefix)
INPUT_VERSION="$1"
NEW_VERSION="${INPUT_VERSION#v}"

# Validate: must be major.minor or major.minor.patch
if ! [[ "${NEW_VERSION}" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
    echo "ERROR: invalid version format '${INPUT_VERSION}'"
    echo "Expected: major.minor (e.g. 1.26) or major.minor.patch (e.g. 1.26.0)"
    exit 1
fi

NEW_VERSION_MINOR=$(echo "${NEW_VERSION}" | cut -d. -f1-2)
REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)

# go.mod
OLD=$(grep '^go ' "${REPO_ROOT}/go.mod" | awk '{print $2}')
sed -i -E "s/^go [0-9]+\.[0-9]+(\.[0-9]+)?/go ${NEW_VERSION}/" "${REPO_ROOT}/go.mod"
[[ "${OLD}" != "${NEW_VERSION}" ]] && echo "go.mod: Updated go ${OLD} to ${NEW_VERSION}"

# Dockerfile – uses only major.minor in the base image tag
OLD=$(grep -E '^FROM golang:[0-9]+\.[0-9]+' "${REPO_ROOT}/Dockerfile" | sed -E 's/FROM golang:([0-9]+\.[0-9]+).*/\1/' | head -1)
sed -i -E "s/^(FROM golang:)[0-9]+\.[0-9]+(.*)/\1${NEW_VERSION_MINOR}\2/" "${REPO_ROOT}/Dockerfile"
[[ "${OLD}" != "${NEW_VERSION_MINOR}" ]] && echo "Dockerfile: Updated golang:${OLD} to golang:${NEW_VERSION_MINOR}"

# docs/Development.md – lists the required Go version for developers
OLD=$(grep -E '^\s*- Go v[0-9]+\.[0-9]+' "${REPO_ROOT}/docs/Development.md" | sed -E 's/.*Go v([0-9]+\.[0-9]+).*/\1/' | head -1)
sed -i -E "s/(- Go v)[0-9]+\.[0-9]+/\1${NEW_VERSION_MINOR}/" "${REPO_ROOT}/docs/Development.md"
[[ "${OLD}" != "${NEW_VERSION_MINOR}" ]] && echo "docs/Development.md: Updated Go v${OLD} to Go v${NEW_VERSION_MINOR}"

# Update module files
(cd "${REPO_ROOT}" && go mod tidy)
