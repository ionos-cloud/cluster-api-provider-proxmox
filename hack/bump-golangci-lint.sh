#!/usr/bin/env bash
# bump-golangci-lint.sh bumps the golangci-lint version in all places it is
# referenced:
#   - go.mod (replace directive)
#   - .custom-gcl.yaml
#
# Usage:   ./hack/bump-golangci-lint.sh <new-version>
# Example: ./hack/bump-golangci-lint.sh v2.10.0

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>"
    echo "Example: $0 v2.10.0"
    exit 1
fi

NEW_VERSION=$(ensure_v_prefix "$1")

# go.mod – replace directive for golangci-lint
OLD=$(grep -E '^\s+github\.com/golangci/golangci-lint/v[0-9]+ =>' "${REPO_ROOT}/go.mod" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | tail -1 || true)
sed -i -E "s|(github\.com/golangci/golangci-lint/v[0-9]+ => github\.com/golangci/golangci-lint/v[0-9]+) v[^ ]+|\1 ${NEW_VERSION}|" "${REPO_ROOT}/go.mod"
[[ -n "${OLD}" && "${OLD}" != "${NEW_VERSION}" ]] && echo "go.mod: Updated replace golangci-lint ${OLD} to ${NEW_VERSION}"

# .custom-gcl.yaml – the version: field
CUSTOM_GCL="${REPO_ROOT}/.custom-gcl.yaml"
if [[ -f "${CUSTOM_GCL}" ]]; then
    OLD=$(grep -E '^version:' "${CUSTOM_GCL}" | awk '{print $2}' || true)
    sed -i -E "s/^(version:) .+/\1 ${NEW_VERSION}/" "${CUSTOM_GCL}"
    [[ -n "${OLD}" && "${OLD}" != "${NEW_VERSION}" ]] && echo ".custom-gcl.yaml: Updated golangci-lint ${OLD} to ${NEW_VERSION}"
fi

# Update module files
run_mod_tidy
