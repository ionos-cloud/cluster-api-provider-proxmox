#!/usr/bin/env bash
# bump-k8s.sh bumps the k8s.io core packages and ENVTEST_K8S_VERSION to a new
# version, keeping all references in sync:
#   - go.mod: require k8s.io/{api,apimachinery,client-go}
#   - go.mod: replace k8s.io/{api,apimachinery,client-go} (if present)
#   - Makefile: ENVTEST_K8S_VERSION
#
# The k8s.io packages use major version 0 (v0.MINOR.PATCH) while
# ENVTEST_K8S_VERSION uses major version 1 (1.MINOR.PATCH). The script accepts
# the k8s.io version (e.g. 0.33.0 or v0.33.0) and derives the ENVTEST version.
#
# Usage:   ./hack/bump-k8s.sh <new-version>
# Example: ./hack/bump-k8s.sh 0.33.0

set -euo pipefail

# shellcheck source=hack/helpers.sh
source "$(dirname "$0")/helpers.sh"

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <new-version>"
    echo "Example: $0 0.33.0"
    exit 1
fi

validate_semver "$1"
NEW=$(ensure_v_prefix "$1")

split_version "${NEW}"
if [[ "${MAJOR}" -ne 0 ]]; then
    echo "ERROR: k8s.io packages use major version 0, got '${MAJOR}'"
    echo "Provide a v0.MINOR.PATCH version (e.g. 0.33.0 or v0.33.0)"
    exit 1
fi

ENVTEST_NEW="1.${MINOR}.${PATCH}"

# ---- go.mod ----
for pkg in 'k8s.io/api' 'k8s.io/apimachinery' 'k8s.io/client-go'; do
    gomod_set_require_version "${pkg}" "${NEW}"
    # Update replace directive if present for this package.
    if [[ -n "$(gomod_replace_version "${pkg}")" ]]; then
        gomod_set_replace_version "${pkg}" "${NEW}"
    fi
done

# ---- Makefile ----
makefile_set_envtest_version "${ENVTEST_NEW}"

# ---- go mod tidy ----
run_mod_tidy
