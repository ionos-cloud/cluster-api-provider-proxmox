#!/usr/bin/env bash
# bump-golangci-lint.sh bumps the golangci-lint version in all places it is
# referenced:
#   - go.mod (replace directive)
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

# Normalize: ensure version has 'v' prefix
INPUT_VERSION="$1"
if [[ "${INPUT_VERSION}" == v* ]]; then
    NEW_VERSION="${INPUT_VERSION}"
else
    NEW_VERSION="v${INPUT_VERSION}"
fi

REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)

# go.mod – replace directive for golangci-lint
OLD=$(grep -E '^\s+github\.com/golangci/golangci-lint/v[0-9]+ =>' "${REPO_ROOT}/go.mod" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | tail -1 || true)
sed -i -E "s|(github\.com/golangci/golangci-lint/v[0-9]+ => github\.com/golangci/golangci-lint/v[0-9]+) v[^ ]+|\1 ${NEW_VERSION}|" "${REPO_ROOT}/go.mod"
[[ -n "${OLD}" && "${OLD}" != "${NEW_VERSION}" ]] && echo "go.mod: Updated replace golangci-lint ${OLD} to ${NEW_VERSION}"

# .github/workflows/lint.yml – the version: field inside the golangci-lint-action step
# Use awk for a context-aware replacement: only update the 'version:' field that
# appears within the golangci-lint-action block.
OLD=$(grep -A5 'golangci-lint-action' "${REPO_ROOT}/.github/workflows/lint.yml" | grep 'version:' | awk '{print $2}' | head -1 || true)
awk '
    /golangci-lint-action/ { in_block=1 }
    in_block && /version:/ {
        sub(/version:.*$/, "version: '"${NEW_VERSION}"'")
        in_block=0
    }
    { print }
' "${REPO_ROOT}/.github/workflows/lint.yml" > /tmp/lint.yml.tmp && mv /tmp/lint.yml.tmp "${REPO_ROOT}/.github/workflows/lint.yml"
[[ -n "${OLD}" && "${OLD}" != "${NEW_VERSION}" ]] && echo ".github/workflows/lint.yml: Updated golangci-lint ${OLD} to ${NEW_VERSION}"

# Update module files
(cd "${REPO_ROOT}" && go mod tidy)
