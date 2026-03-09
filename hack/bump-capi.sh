#!/usr/bin/env bash
# bump-capi.sh bumps sigs.k8s.io/cluster-api to a new version, keeping all
# references in sync:
#   - go.mod: require sigs.k8s.io/cluster-api
#   - go.mod: require sigs.k8s.io/cluster-api/test
#   - go.mod: replace sigs.k8s.io/cluster-api pin
#   - test/e2e/data/shared/v1beta1/metadata.yaml: releaseSeries first entry
#
# Usage:   ./hack/bump-capi.sh <new-version> <contract>
# Example: ./hack/bump-capi.sh 1.11.0 v1beta1

set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <new-version> <contract>"
    echo "Example: $0 1.11.0 v1beta1"
    exit 1
fi

# Normalize: ensure version has 'v' prefix
INPUT_VERSION="$1"
if [[ "${INPUT_VERSION}" == v* ]]; then
    NEW_VERSION="${INPUT_VERSION}"
else
    NEW_VERSION="v${INPUT_VERSION}"
fi
CONTRACT="$2"

# Validate version format: must be major.minor.patch
if ! [[ "${NEW_VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "ERROR: invalid version format '${INPUT_VERSION}'"
    echo "Expected: major.minor.patch (e.g. 1.11.0 or v1.11.0)"
    exit 1
fi

CAPI_VERSION_NO_V=$(echo "${NEW_VERSION}" | sed 's/v//')
CAPI_MAJOR=$(echo "${CAPI_VERSION_NO_V}" | cut -d. -f1)
CAPI_MINOR=$(echo "${CAPI_VERSION_NO_V}" | cut -d. -f2)
REPO_ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
GO_MOD="${REPO_ROOT}/go.mod"
METADATA="${REPO_ROOT}/test/e2e/data/shared/v1beta1/metadata.yaml"

# ---- go.mod: require sigs.k8s.io/cluster-api ----
OLD=$(grep -E '^\s+sigs\.k8s\.io/cluster-api\s+v' "${GO_MOD}" | awk '{print $2}' | head -1)
sed -i -E "s|(^\s+sigs\.k8s\.io/cluster-api[[:space:]]+)v[^ ]+|\1${NEW_VERSION}|" "${GO_MOD}"
[[ "${OLD}" != "${NEW_VERSION}" ]] && echo "go.mod: Updated require sigs.k8s.io/cluster-api ${OLD} to ${NEW_VERSION}"

# ---- go.mod: require sigs.k8s.io/cluster-api/test ----
OLD=$(grep -E '^\s+sigs\.k8s\.io/cluster-api/test v' "${GO_MOD}" | awk '{print $2}' | head -1)
sed -i -E "s|(^\s+sigs\.k8s\.io/cluster-api/test) v[^ ]+|\1 ${NEW_VERSION}|" "${GO_MOD}"
[[ "${OLD}" != "${NEW_VERSION}" ]] && echo "go.mod: Updated require sigs.k8s.io/cluster-api/test ${OLD} to ${NEW_VERSION}"

# ---- go.mod: replace pin ----
OLD=$(grep -E '^replace sigs\.k8s\.io/cluster-api =>' "${GO_MOD}" | awk '{print $NF}' | head -1)
sed -i -E "s|(^replace sigs\.k8s\.io/cluster-api => sigs\.k8s\.io/cluster-api) v[^ ]+|\1 ${NEW_VERSION}|" "${GO_MOD}"
[[ "${OLD}" != "${NEW_VERSION}" ]] && echo "go.mod: Updated replace sigs.k8s.io/cluster-api ${OLD} to ${NEW_VERSION}"

# ---- test/e2e/data/shared/v1beta1/metadata.yaml ----
# Add new releaseSeries entry as the first element if not already present.
if ! awk '/- major: '"${CAPI_MAJOR}"'[[:space:]]*$/{found=1; next} found && /minor: '"${CAPI_MINOR}"'[[:space:]]*$/{ok=1; exit} {found=0} END{exit !ok}' "${METADATA}"; then
    python3 -c "
new_entry = '  - major: ${CAPI_MAJOR}\n    minor: ${CAPI_MINOR}\n    contract: ${CONTRACT}\n'
marker = 'releaseSeries:\n'
with open('${METADATA}') as f:
    content = f.read()
idx = content.find(marker)
if idx == -1:
    raise SystemExit('ERROR: could not find releaseSeries in ${METADATA}')
insert_pos = idx + len(marker)
content = content[:insert_pos] + new_entry + content[insert_pos:]
with open('${METADATA}', 'w') as f:
    f.write(content)
"
    echo "test/e2e/data/shared/v1beta1/metadata.yaml: Added releaseSeries entry for v${CAPI_MAJOR}.${CAPI_MINOR} (${CONTRACT})"
fi

# ---- go mod tidy ----
(cd "${REPO_ROOT}" && go mod tidy)
