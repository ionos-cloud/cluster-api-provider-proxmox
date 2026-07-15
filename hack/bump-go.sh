#!/usr/bin/env bash
# bump-go.sh bumps the Go version in all places it is referenced:
#   - go.mod
#   - Dockerfile
#   - .golangci-kal.yml (run.go)
#   - .github/workflows/test.yml (pinned Go container image)
#
# Usage:   ./hack/bump-go.sh <new-version> [<digest>]
# Example: ./hack/bump-go.sh 1.26.0
# Example: ./hack/bump-go.sh 1.26.4 sha256:0dcba0d95dbfb072e9917a106b9e07d7cc298097dc83e9307056ef1889de654d

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -lt 1 || $# -gt 2 ]]; then
    echo "Usage: $0 <new-version> [<digest>]" >&2
    echo "Example: $0 1.26.0" >&2
    exit 1
fi

# Go versions don't use a 'v' prefix
NEW=$(strip_v_prefix "$1")
validate_semver "${NEW}"
DIGEST="${2:-}"
if [[ -n "${DIGEST}" ]]; then validate_sha256 "${DIGEST}"; fi

NEW_MINOR=$(echo "${NEW}" | cut -d. -f1-2)

gomod_set_go "${NEW}"
dockerfile_set_go "${NEW_MINOR}"
golangcikal_set_go "${NEW_MINOR}"
testworkflow_set_go_image "${NEW}" "${DIGEST}"

gomod_tidy
