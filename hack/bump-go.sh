#!/usr/bin/env bash
# bump-go.sh bumps the Go version in all places it is referenced:
#   - go.mod
#   - Dockerfile
#   - docs/Development.md
#
# Usage:   ./hack/bump-go.sh <new-version>
# Example: ./hack/bump-go.sh 1.26.0

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>"
    echo "Example: $0 1.26.0"
    exit 1
fi

# Go versions don't use a 'v' prefix
NEW=$(strip_v_prefix "$1")
validate_go_version "${NEW}"

NEW_MINOR=$(echo "${NEW}" | cut -d. -f1-2)

gomod_set_go "${NEW}"
dockerfile_set_go "${NEW_MINOR}"
docs_set_go "${NEW_MINOR}"

gomod_tidy
