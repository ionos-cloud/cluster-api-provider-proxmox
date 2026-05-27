#!/usr/bin/env bash
# verify-versions.sh checks that versions of special dependencies are consistent
# across the repository. Add this to CI via the verify-versions Makefile target.

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

ERRORS=()

fail() {
    ERRORS+=("$1")
}

# ---- Go version ----
# go.mod, Dockerfile, and docs/Development.md must all reference the same Go
# version. go.mod uses the full "major.minor.patch" form; Dockerfile and docs
# use only "major.minor".

GO_VERSION_ROOT=$(gomod_get_go)

GO_VERSION_MINOR=$(echo "${GO_VERSION_ROOT}" | cut -d. -f1-2)
DOCKERFILE_GO_VERSION=$(dockerfile_get_go)
if [[ "${DOCKERFILE_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), Dockerfile has '${DOCKERFILE_GO_VERSION}'"
fi

DOCS_GO_VERSION=$(docs_get_go)
if [[ -n "${DOCS_GO_VERSION}" && "${DOCS_GO_VERSION}" != "${GO_VERSION_MINOR}" ]]; then
    fail "Go version mismatch: go.mod has '${GO_VERSION_ROOT}' (${GO_VERSION_MINOR}), docs/Development.md lists 'Go v${DOCS_GO_VERSION}'"
fi

# ---- Report results ----

if [[ ${#ERRORS[@]} -gt 0 ]]; then
    echo "Version consistency check FAILED:"
    for err in "${ERRORS[@]}"; do
        echo "  - ${err}"
        # In GitHub Actions, emit workflow commands for annotations.
        if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
            echo "::error::${err}"
        fi
    done
    exit 1
fi
