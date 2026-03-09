#!/usr/bin/env bash
# bump-golangci-lint.sh bumps the golangci-lint version in all places it is
# referenced:
#   - hack/tools/go.mod
#   - .github/workflows/lint.yml
#
# Usage:   ./hack/bump-golangci-lint.sh <new-version>
# Example: ./hack/bump-golangci-lint.sh v2.10.0

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>"
    echo "Example: $0 v2.10.0"
    exit 1
fi

NEW_VERSION="$1"
REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)

# Ensure version starts with 'v'
if [[ "${NEW_VERSION}" != v* ]]; then
    echo "ERROR: version must start with 'v', got '${NEW_VERSION}'"
    exit 1
fi

echo "Bumping golangci-lint to ${NEW_VERSION}..."

# hack/tools/go.mod
sed -i -E "s|(github\.com/golangci/golangci-lint/v[0-9]+) v[^ ]+|\1 ${NEW_VERSION}|" "${REPO_ROOT}/hack/tools/go.mod"
echo "  Updated hack/tools/go.mod"

# .github/workflows/lint.yml – the version: field inside the golangci-lint-action step
# Use awk for a context-aware replacement: only update the 'version:' field that
# appears within the golangci-lint-action block.
awk '
    /golangci-lint-action/ { in_block=1 }
    in_block && /version:/ {
        sub(/version:.*$/, "version: '"${NEW_VERSION}"'")
        in_block=0
    }
    { print }
' "${REPO_ROOT}/.github/workflows/lint.yml" > /tmp/lint.yml.tmp && mv /tmp/lint.yml.tmp "${REPO_ROOT}/.github/workflows/lint.yml"
echo "  Updated .github/workflows/lint.yml"

echo "Done."
echo "Next steps: run 'make tidy' to update go.sum files, then 'make verify-versions' to confirm."
