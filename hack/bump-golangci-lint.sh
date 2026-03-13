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

NEW=$(ensure_v_prefix "$1")

gomod_add_replace "${NEW}" 'github.com/golangci/golangci-lint/v2'
customgcl_set_version "${NEW}"

gomod_tidy
