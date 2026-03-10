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

# Validate: must be major.minor or major.minor.patch
if ! [[ "${NEW}" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
    echo "ERROR: invalid version format '$1'"
    echo "Expected: major.minor (e.g. 1.26) or major.minor.patch (e.g. 1.26.0)"
    exit 1
fi

NEW_MINOR=$(echo "${NEW}" | cut -d. -f1-2)

gomod_set_go_version "${NEW}"
dockerfile_set_go_version "${NEW_MINOR}"
docs_set_go_version "${NEW_MINOR}"

run_mod_tidy
